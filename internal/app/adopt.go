package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/scan"
	"github.com/Maybe404/skilldots/internal/skillmd"
)

// Scan inventories every agent directory on the machine.
func (s *Session) Scan() (scan.Report, error) { return scan.Run(s.Lib, s.Reg) }

// Proposal is one adoption the user can accept. Its JSON form is part of
// `skd adopt --json`.
type Proposal struct {
	Item scan.Item
	// Kind is pack | app | own | enable | drop | skip.
	Kind   string
	Label  string
	Repo   string
	Skill  string
	Reason string
	// Via is the kind of the proposal an "enable" extends (same skill
	// found in another agent); Entry is the qualified library entry an
	// "enable" of something already in the library refers to.
	Via   string `json:",omitempty"`
	Entry string `json:",omitempty"`
}

// Actionable reports whether accepting p does anything.
func (p Proposal) Actionable() bool { return p.Kind != "skip" }

// Proposals works out how to take over each unmanaged entry of rep:
// copies installed by "npx skills" become subscriptions to their upstream
// repository, links into application bundles become [[app]] entries, and
// anything else becomes a skill of the user's own under skills/.
func (s *Session) Proposals(rep scan.Report) []Proposal {
	entries, _ := s.Resolve()
	seen := map[string]bool{}
	var out []Proposal
	for _, it := range rep.Unmanaged() {
		if seen[it.Name] {
			continue
		}
		e, err := engine.Lookup(entries, it.Name)
		if err != nil {
			seen[it.Name] = true
			out = append(out, Proposal{Item: it, Kind: "skip", Label: err.Error() + "; enable one with skd enable <name> --agent " + it.Target})
			continue
		}
		if e != nil {
			// Already in the library: this agent just is not a target yet.
			enabled := false
			for _, t := range e.Targets {
				enabled = enabled || t.Name == it.Target
			}
			if enabled {
				continue
			}
			out = append(out, Proposal{Item: it, Kind: "enable", Label: "enable for " + it.Target + " (already in library)", Entry: engine.Qualified(*e)})
			continue
		}
		p := Proposal{Item: it}
		switch {
		case it.State == scan.Broken:
			p.Kind, p.Label = "drop", "broken link; move to the trash"
		case manifest.CheckName(it.Name) != nil:
			// It would become a directory and a link of that name.
			p.Kind, p.Label = "skip", "not a valid skill name (letters, digits, '.', '_' and '-'); rename it first"
		case it.Duplicate:
			p.Kind, p.Label = "skip", "same name with different content in several agents; resolve by hand"
		case it.State == scan.AppLink:
			p.Kind, p.Label = "app", "app → "+it.LinkTo
		case it.Source != "":
			p.Kind, p.Repo, p.Skill = "pack", it.Source, it.Name
			p.Label = "subscribe to " + it.Source
		case it.State == scan.ExtLink:
			if repo := gitRemote(it.LinkTo); repo != "" {
				p.Kind, p.Repo, p.Skill, p.Label = "pack", repo, it.Name, "subscribe to "+repo+" (link target is a clone of it)"
			} else {
				p.Kind, p.Label = "own", "copy into skills/ as your own"
			}
		default:
			p.Kind, p.Label = "own", "copy into skills/ as your own (no known upstream)"
		}
		seen[it.Name] = true
		out = append(out, p)
	}
	// "own"/"pack" copies found in several agents: enable the extra agents too.
	for _, it := range rep.Unmanaged() {
		for _, p := range out {
			if p.Item.Name == it.Name && p.Item.Target != it.Target && (p.Kind == "own" || p.Kind == "pack" || p.Kind == "app") {
				out = append(out, Proposal{Item: it, Kind: "enable", Label: "enable for " + it.Target, Via: p.Kind, Repo: p.Repo, Skill: p.Skill})
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Item.Name < out[j].Item.Name })
	return out
}

// gitRemote returns owner/repo when dir is inside a GitHub clone.
func gitRemote(dir string) string {
	if !fsutil.Exists(dir) {
		return ""
	}
	url, err := library.Git(dir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	src, err := pack.ParseSource(url)
	if err != nil || !strings.Contains(src.URL, "github.com") {
		return ""
	}
	return src.Repo
}

// AdoptOutcome is the result of Adopt.
type AdoptOutcome struct {
	Apply *ApplyOutcome
	// Failed counts repositories that could not be fetched.
	Failed int
}

// Adopt carries out the accepted proposals. A real directory is replaced
// by a link only when the library holds exactly its content; displaced
// directories go to the trash. It returns an error after doing what it
// could when some repository could not be fetched.
func (s *Session) Adopt(props []Proposal) (*AdoptOutcome, error) {
	lib := s.Lib
	m := lib.Manifest
	packs := map[string]*manifest.Pack{}
	var order []string
	// renamed maps an agent entry name to the skill name it was adopted as.
	renamed := map[string]string{}
	adoptedAs := func(it scan.Item) string {
		if n := renamed[it.Name]; n != "" {
			return n
		}
		return it.Name
	}
	for _, p := range props {
		switch p.Kind {
		case "pack":
			pk := m.FindPack(p.Repo)
			if pk == nil {
				pk = &manifest.Pack{Repo: p.Repo}
				m.Packs = append(m.Packs, pk)
			}
			if !pk.UsesAll() {
				pk.Use = dedupe(append(pk.Use, p.Skill))
			}
			if packs[p.Repo] == nil {
				order = append(order, p.Repo)
			}
			packs[p.Repo] = pk
		case "app":
			dup := false
			for _, a := range m.Apps {
				if a.Name == p.Item.Name {
					dup = true
					if a.Path != p.Item.LinkTo {
						s.logf(Warn, "[[app]] %s already points at %s; not changing it to %s", a.Name, a.Path, p.Item.LinkTo)
					}
				}
			}
			if !dup {
				m.Apps = append(m.Apps, &manifest.App{Name: p.Item.Name, Path: p.Item.LinkTo})
			}
		case "own":
			src := p.Item.Path
			if p.Item.LinkTo != "" {
				src = p.Item.LinkTo
			}
			// The library knows a skill by its frontmatter name, which may
			// differ from the directory name it had in the agent dir.
			name := p.Item.Name
			if meta, err := skillmd.Read(src); err == nil && meta.Name != name && manifest.CheckName(meta.Name) == nil {
				name = meta.Name
				renamed[p.Item.Name] = name
			}
			dest := filepath.Join(lib.SkillsDir(), name)
			if err := s.writableInLibrary(dest); err != nil {
				return nil, err
			}
			if !fsutil.Lexists(dest) {
				if err := fsutil.CopyTree(src, dest); err != nil {
					return nil, err
				}
				s.logf(Done, "copied %s → %s", p.Item.Name, lib.Rel(dest))
			}
		case "drop":
			// A dangling link holds only where it pointed. The path may have changed since
			// the scan (a dialog can stay open for a while): anything that is
			// not a dangling link any more is left alone, and so is one of
			// a target that no longer leads where it did.
			if t, err := s.Reg.Get(p.Item.Target); err != nil || filepath.Dir(p.Item.Path) != t.Path || t.Intact() != nil {
				s.logf(Warn, "%s: not removed; it is no longer an entry of the %s target's directory", p.Item.Path, p.Item.Target)
				continue
			}
			if fi, err := os.Lstat(p.Item.Path); err != nil {
				if !os.IsNotExist(err) {
					s.logf(Warn, "%s: %v; not removing it", p.Item.Path, err)
				}
				continue
			} else if fi.Mode()&os.ModeSymlink == 0 {
				s.logf(Warn, "%s is no longer a broken link (something was put there since the scan); left in place", p.Item.Path)
				continue
			} else if _, err := os.Stat(p.Item.Path); err == nil {
				s.logf(Warn, "%s is no longer a broken link (it points at something now); left in place", p.Item.Path)
				continue
			}
			// Not made by skd (it is unmanaged): moved to the trash, link
			// and all, like anything else skd takes out of the way.
			to, err := engine.Trash(lib, p.Item.Path)
			if err != nil {
				return nil, err
			}
			s.logf(Done, "moved broken link %s to %s", p.Item.Path, lib.Rel(to))
		}
	}
	out := &AdoptOutcome{}
	for _, repo := range order {
		pk := packs[repo]
		rep, err := engine.Sync(lib, pk)
		if err != nil {
			s.logf(Fail, "%s: %v", repo, err)
			out.Failed++
			continue
		}
		s.logf(Done, "%s @ %s", repo, short(rep.NewCommit))
		// Names the upstream no longer has stay as the user's own copy.
		for _, n := range pk.Use {
			if pack.Find(rep.Upstream, n) != nil {
				continue
			}
			s.logf(Warn, "%s is not in %s any more; keeping your copy under skills/", n, repo)
			for _, p := range props {
				if p.Kind != "pack" || p.Skill != n {
					continue
				}
				src := p.Item.Path
				if p.Item.LinkTo != "" {
					src = p.Item.LinkTo
				}
				dest := filepath.Join(lib.SkillsDir(), n)
				if err := s.writableInLibrary(dest); err != nil {
					return nil, err
				}
				if fsutil.Lexists(dest) {
					s.logf(Warn, "%s already exists; leaving it as is", lib.Rel(dest))
					break
				}
				if err := fsutil.CopyTree(src, dest); err != nil {
					return nil, err
				}
				break
			}
			var use []string
			for _, u := range pk.Use {
				if u != n {
					use = append(use, u)
				}
			}
			pk.Use = use
		}
	}
	if err := lib.Save(); err != nil {
		return nil, err
	}
	// Targets: every adopted entry is enabled for each agent it was found in.
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	// entryFor is the one library entry a proposal adopts into.
	entryFor := func(p Proposal) *engine.Entry {
		kind := p.Kind
		if kind == "enable" && p.Via != "" {
			kind = p.Via
		}
		if kind == "enable" {
			e, _ := engine.Lookup(entries, p.Entry)
			return e
		}
		for i := range entries {
			e := &entries[i]
			switch {
			case kind == "own" && e.Kind == engine.KindSkill && e.Dir == filepath.Join(lib.SkillsDir(), adoptedAs(p.Item)),
				kind == "pack" && e.Kind == engine.KindVendor && e.Origin == p.Repo && e.Name == p.Skill,
				kind == "app" && e.Kind == engine.KindApp && e.Name == p.Item.Name:
				return e
			}
		}
		return nil
	}
	// A real directory is only replaced by a link when the library holds
	// exactly its content; otherwise it may be the user's only copy of
	// something, so it stays where it is.
	keep := map[string]bool{}
	for _, p := range props {
		if p.Kind == "skip" || p.Kind == "drop" || p.Item.LinkTo != "" {
			continue
		}
		e := entryFor(p)
		if e == nil {
			continue
		}
		h, err := fsutil.HashTree(e.Dir)
		if err == nil && fsutil.Pristine(p.Item.Path, h) {
			continue
		}
		keep[p.Item.Target+"/"+p.Item.Name] = true
		s.logf(Warn, "%s differs from the library's %s (%s); left in place — compare them and remove one by hand",
			p.Item.Path, adoptedAs(p.Item), e.Dir)
	}
	want := map[*engine.Entry][]string{}
	var eorder []*engine.Entry
	for _, p := range props {
		if p.Kind == "skip" || p.Kind == "drop" || keep[p.Item.Target+"/"+p.Item.Name] {
			continue
		}
		e := entryFor(p)
		if e == nil {
			continue
		}
		if _, ok := want[e]; !ok {
			eorder = append(eorder, e)
		}
		want[e] = append(want[e], p.Item.Target)
	}
	for _, e := range eorder {
		next := dedupe(append(append([]string{}, e.TargetNames...), want[e]...))
		if len(next) != len(e.TargetNames) {
			setEntryTargets(m, e, next)
		}
	}
	if err := lib.Save(); err != nil {
		return nil, err
	}
	// Force only the agent entries this adoption takes over; anything else
	// that is modified or unmanaged keeps being skipped.
	force := map[string]bool{}
	for _, p := range props {
		if p.Kind != "skip" && p.Kind != "drop" && !keep[p.Item.Target+"/"+p.Item.Name] {
			force[p.Item.Target+"/"+p.Item.Name] = true
		}
	}
	ap, err := s.Apply(ApplyOptions{ForceKeys: force})
	if err != nil {
		return nil, err
	}
	out.Apply = ap
	// An entry adopted under another name now has a link of that name next
	// to it; move the original aside so the agent does not see the skill twice.
	for _, p := range props {
		n := adoptedAs(p.Item)
		if p.Kind == "skip" || p.Kind == "drop" || n == p.Item.Name || keep[p.Item.Target+"/"+p.Item.Name] {
			continue
		}
		t, err := s.Reg.Get(p.Item.Target)
		if err != nil || lib.Lock.Links[t.Name+"/"+n] == nil || !fsutil.Lexists(filepath.Join(t.Path, n)) || !fsutil.Lexists(p.Item.Path) {
			continue
		}
		// Only an entry of the target itself, while it leads where it did.
		if filepath.Dir(p.Item.Path) != t.Path || t.Intact() != nil {
			s.logf(Warn, "%s: not moved aside; it is no longer an entry of the %s target's directory", p.Item.Path, t.Name)
			continue
		}
		to, err := engine.Trash(lib, p.Item.Path)
		if err != nil {
			return nil, err
		}
		// Reported with the apply's own backups (printed once, after the
		// link changes), not logged here as well.
		ap.Backups = append(ap.Backups, p.Item.Path+" → "+lib.Rel(to))
	}
	if out.Failed > 0 {
		return out, fmt.Errorf("%d repositories could not be fetched", out.Failed)
	}
	return out, nil
}

func short(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
