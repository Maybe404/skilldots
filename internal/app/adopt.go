package app

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
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
	// Upstream is the repository the entry came from, as another
	// installer recorded it or as the clone it links into says, whatever
	// Kind is. Differs is set when its content is not what that
	// upstream has now (changed here, or an older version): it is then
	// proposed as a skill of your own ("own"), never replaced by
	// upstream's, and subscribing instead is the user's explicit choice
	// (skd add <Upstream> --skill <name>).
	Upstream string `json:",omitempty"`
	Differs  bool   `json:",omitempty"`
	// As is the name an "own" proposal is copied into skills/ under: its
	// SKILL.md name, or its directory name when that one is taken in the
	// library (the copy's SKILL.md then says so). The dry run and the
	// adoption use the same.
	As string `json:",omitempty"`
}

// Actionable reports whether accepting p does anything.
func (p Proposal) Actionable() bool { return p.Kind != "skip" }

// Proposals works out how to take over each unmanaged entry of rep:
// copies installed by "npx skills" become subscriptions to their upstream
// repository, links into application bundles become [[app]] entries, and
// anything else becomes a skill of the user's own under skills/.
func (s *Session) Proposals(rep scan.Report) []Proposal { return s.proposals(rep, true) }

// ScanAdvice is Proposals without fetching anything: what `skd scan` says
// about each unmanaged entry, a "skip" one (the user has to act) in
// particular. A subscription it proposes is not compared with upstream.
func (s *Session) ScanAdvice(rep scan.Report) []Proposal { return s.proposals(rep, false) }

func (s *Session) proposals(rep scan.Report, fetch bool) []Proposal {
	entries, _ := s.Resolve()
	hashes := map[int]string{}
	seen := map[string]bool{}
	var out []Proposal
	for _, it := range rep.Unmanaged() {
		if seen[it.Name] {
			continue
		}
		e, err := engine.Lookup(entries, it.Name)
		if err != nil {
			seen[it.Name] = true
			out = append(out, Proposal{Item: it, Kind: "skip", Label: i18n.Sprintf("%v; enable one with skd enable <name> --agent %s", err, it.Target)})
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
			if t, err := s.Reg.Get(it.Target); err == nil && engine.ReachesViaShared(s.Lib, e, t) {
				// The agent reads the library's copy from ~/.agents/skills
				// already: this one is a second copy it loads.
				if sameContent(it, e.Dir) {
					out = append(out, Proposal{Item: it, Kind: "enable", Entry: engine.Qualified(*e),
						Label: i18n.Sprintf("%s already reads the library's copy from ~/.agents/skills; this second copy goes to the trash", it.Target)})
				} else {
					out = append(out, Proposal{Item: it, Kind: "skip",
						Label: i18n.Sprintf("differs from the library's %s, which %s already reads from ~/.agents/skills; compare them and remove one by hand", e.LinkName, it.Target)})
				}
				continue
			}
			out = append(out, Proposal{Item: it, Kind: "enable", Label: i18n.Sprintf("enable for %s (already in library)", it.Target), Entry: engine.Qualified(*e)})
			continue
		}
		// The same content as a skill the library has, under another name
		// (a copy someone renamed): that skill, not a new one.
		if it.State != scan.Broken {
			if m := s.sameAsLibrary(it, entries, hashes); m != nil {
				seen[it.Name] = true
				out = append(out, s.alreadyIn(it, m))
				continue
			}
		}
		p := Proposal{Item: it}
		switch {
		case it.State == scan.Broken:
			p.Kind, p.Label = "drop", i18n.T("broken link; move to the trash")
		case manifest.CheckName(it.Name) != nil:
			// It would become a directory and a link of that name.
			p.Kind, p.Label = "skip", i18n.T("not a valid skill name (letters, digits, '.', '_' and '-'); rename it first")
		case it.Duplicate:
			p.Kind, p.Label = "skip", i18n.T("same name with different content in several agents; resolve by hand")
		case it.State == scan.AppLink:
			p.Kind, p.Label = "app", i18n.Sprintf("app → %s", it.LinkTo)
		case it.Source != "":
			p.Kind, p.Repo, p.Skill = "pack", it.Source, it.Name
			p.Label = i18n.Sprintf("subscribe to %s", it.Source)
		case it.State == scan.ExtLink:
			if repo := gitRemote(it.LinkTo); repo != "" {
				p.Kind, p.Repo, p.Skill, p.Label = "pack", repo, it.Name, i18n.Sprintf("subscribe to %s (link target is a clone of it)", repo)
			} else {
				p.Kind, p.Label = "own", i18n.T("copy into skills/ as a local skill")
			}
		default:
			p.Kind, p.Label = "own", i18n.T("copy into skills/ as a local skill (no known upstream)")
		}
		seen[it.Name] = true
		out = append(out, p)
	}
	if fetch {
		s.checkUpstream(out)
	}
	s.nameOwn(out, entries)
	// "own"/"pack" copies found in several agents: enable the extra agents too.
	for _, it := range rep.Unmanaged() {
		for _, p := range out {
			if p.Item.Name == it.Name && p.Item.Target != it.Target && (p.Kind == "own" || p.Kind == "pack" || p.Kind == "app") {
				out = append(out, Proposal{Item: it, Kind: "enable", Label: i18n.Sprintf("enable for %s", it.Target), Via: p.Kind, Repo: p.Repo, Skill: p.Skill})
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Item.Name < out[j].Item.Name })
	return out
}

// sameAsLibrary is the library entry whose files are exactly what it
// holds, or nil. hashes caches the entries' hashes by index.
func (s *Session) sameAsLibrary(it scan.Item, entries []engine.Entry, hashes map[int]string) *engine.Entry {
	src := it.Path
	if it.LinkTo != "" {
		src = it.LinkTo
	}
	h := it.Hash
	if h == "" || it.LinkTo != "" {
		var err error
		if h, err = fsutil.HashTree(src); err != nil {
			return nil
		}
	}
	for i := range entries {
		e := &entries[i]
		if e.Missing || e.Refused != "" || e.LinkedVia != "" {
			continue
		}
		eh, ok := hashes[i]
		if !ok {
			eh, _ = fsutil.HashTree(e.Dir)
			hashes[i] = eh
		}
		if eh == h && sameContent(it, e.Dir) {
			return e
		}
	}
	return nil
}

// alreadyIn proposes, for an agent entry holding exactly what library
// entry e holds, to install e for the agent instead: the copy found goes
// to the trash once e reaches the agent.
func (s *Session) alreadyIn(it scan.Item, e *engine.Entry) Proposal {
	p := Proposal{Item: it, Kind: "enable", Entry: engine.Qualified(*e)}
	from := Origin(*e)
	if t, err := s.Reg.Get(it.Target); err == nil && engine.ReachesViaShared(s.Lib, e, t) {
		p.Label = i18n.Sprintf("already in the library as %s (%s), which %s reads from ~/.agents/skills; this copy goes to the trash", e.LinkName, from, it.Target)
		return p
	}
	p.Label = i18n.Sprintf("already in the library as %s (%s): install it for %s; this copy goes to the trash", e.LinkName, from, it.Target)
	return p
}

// nameOwn gives every "own" proposal the name it is copied into skills/
// under (As): its SKILL.md name, else its directory name when the first is
// taken in the library (or by another proposal); with both taken it is
// not proposed, rather than adopted into a name clash.
func (s *Session) nameOwn(props []Proposal, entries []engine.Entry) {
	taken := map[string]string{}
	for _, e := range entries {
		taken[strings.ToLower(e.LinkName)] = e.LinkName + " (" + Origin(e) + ")"
	}
	for i := range props {
		p := &props[i]
		if p.Kind != "own" {
			continue
		}
		src := p.Item.Path
		if p.Item.LinkTo != "" {
			src = p.Item.LinkTo
		}
		names := []string{}
		if meta, err := skillmd.Read(src); err == nil && manifest.CheckName(meta.Name) == nil {
			names = append(names, meta.Name)
		}
		if !slices.Contains(names, p.Item.Name) {
			names = append(names, p.Item.Name)
		}
		free := func(n string) bool {
			return taken[strings.ToLower(n)] == "" && !fsutil.Lexists(filepath.Join(s.Lib.SkillsDir(), n))
		}
		for _, n := range names {
			if free(n) {
				p.As = n
				break
			}
		}
		if p.As == "" {
			p.Kind, p.Label = "skip", i18n.Sprintf("named %s, which %s already uses in the library; rename it (its directory or the name in its SKILL.md) first", names[0], taken[strings.ToLower(names[0])])
			continue
		}
		switch {
		case p.As != names[0]:
			p.Label += " · " + i18n.Sprintf("as %s: its SKILL.md name %s is taken by %s", p.As, names[0], taken[strings.ToLower(names[0])])
		case p.As != p.Item.Name:
			p.Label += " · " + i18n.Sprintf("as %s, the name in its SKILL.md", p.As)
		}
		taken[strings.ToLower(p.As)] = p.As
	}
}

// Origin says where a library entry comes from, for a person: its pack's
// repository, skills/, private/ or the application.
func Origin(e engine.Entry) string {
	switch e.Kind {
	case engine.KindVendor:
		return e.Origin
	case engine.KindSkill:
		return "skills/"
	case engine.KindPrivate:
		return "private/"
	case engine.KindApp:
		return i18n.Sprintf("app %s", e.Origin)
	}
	return string(e.Kind)
}

// checkUpstream compares every "pack" proposal with what its upstream has
// now. One whose content differs is proposed as a skill of your own
// instead: subscribing would link upstream's version in its place, and
// the user's changes would no longer reach any agent. One that cannot be
// compared (the repository cannot be fetched, or no longer has the
// skill) is not proposed: skd cannot tell which version it would put in
// its place.
func (s *Session) checkUpstream(props []Proposal) {
	type upstream struct {
		hashes map[string]string
		err    error
	}
	fetched := map[string]*upstream{}
	for i := range props {
		p := &props[i]
		if p.Kind != "pack" {
			continue
		}
		p.Upstream = p.Repo
		up := fetched[p.Repo]
		if up == nil {
			up = &upstream{hashes: map[string]string{}}
			up.err = s.upstreamHashes(p.Repo, up.hashes)
			fetched[p.Repo] = up
		}
		if up.err != nil {
			p.Kind, p.Label = "skip", i18n.Sprintf("cannot compare it with %s (%v); run skd adopt again when it can be fetched", p.Repo, firstLine(up.err.Error()))
			continue
		}
		want, ok := up.hashes[p.Skill]
		if !ok {
			p.Kind, p.Label = "skip", i18n.Sprintf("%s has no skill %s now; copy it into skills/ by hand to keep it", p.Repo, p.Skill)
			continue
		}
		have := p.Item.Hash
		if p.Item.LinkTo != "" || have == "" {
			src := p.Item.Path
			if p.Item.LinkTo != "" {
				src = p.Item.LinkTo
			}
			have, _ = fsutil.HashTree(src)
		}
		if have == want {
			continue
		}
		p.Kind, p.Repo, p.Skill, p.Differs = "own", "", "", true
		p.Label = i18n.Sprintf("differs from %s upstream (changed here, or an older version): copy into skills/ as a local skill; `skd add %s --skill %s` takes upstream's instead", p.Upstream, p.Upstream, p.Item.Name)
	}
}

// upstreamHashes fetches repo and records the content hash of each of
// its skills by name.
func (s *Session) upstreamHashes(repo string, into map[string]string) error {
	src, err := pack.ParseSource(repo)
	if err != nil {
		return err
	}
	if p := s.Lib.Manifest.FindPack(repo); p != nil {
		src.Ref = p.Ref
	}
	defer pack.Hold(src.Repo)()
	clone, _, err := pack.Fetch(src)
	if err != nil {
		return err
	}
	up, _, err := pack.Scan(clone)
	if err != nil {
		return err
	}
	for _, sk := range up {
		if h, err := fsutil.HashTree(sk.Dir); err == nil {
			into[sk.Name] = h
		}
	}
	return nil
}

// gitRemote returns owner/repo when dir is inside a GitHub clone.
func gitRemote(dir string) string {
	if !fsutil.Exists(dir) {
		return ""
	}
	url, err := library.Git(dir, "config", "--get", "remote.origin.url") // as configured, before url.insteadOf
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
	// copied is the hash of what each "own" skill was copied from, by its
	// name in the library: its SKILL.md may be renamed after the copy.
	copied := map[string]string{}
	adoptedAs := func(it scan.Item) string {
		if n := renamed[it.Name]; n != "" {
			return n
		}
		return it.Name
	}
	// A copy of a library skill found under another name: the entry it
	// is installed as is the skill's own name.
	if before, err := s.Resolve(); err == nil {
		for _, p := range props {
			if p.Kind == "enable" && p.Entry != "" {
				if e, _ := engine.Lookup(before, p.Entry); e != nil && e.LinkName != p.Item.Name {
					renamed[p.Item.Name] = e.LinkName
				}
			}
		}
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
			// differ from the directory name it had in the agent dir. The
			// proposal says which name (As); without one, as before.
			name := p.As
			meta, merr := skillmd.Read(src)
			if name == "" {
				name = p.Item.Name
				if merr == nil && meta.Name != name && manifest.CheckName(meta.Name) == nil {
					name = meta.Name
				}
			}
			if name != p.Item.Name {
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
				if h, err := fsutil.HashTree(dest); err == nil {
					copied[name] = h
				}
				// Adopted under another name than its SKILL.md says (that
				// one was taken): the copy says so too, or it would clash.
				if merr == nil && meta.Name != name {
					if err := skillmd.SetName(dest, name); err != nil {
						return nil, err
					}
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
			s.noteWarn(engine.SetTrashNote(to, &engine.TrashNote{Reason: engine.ReasonBroken, Skills: []string{p.Item.Name}}))
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
	// A link to a directory outside the library (a clone of upstream) is
	// replaced only when that directory holds exactly what the library
	// now has: it may hold the user's changes, which the library's copy
	// would take the place of (the proposals were made before, and the
	// directory may have changed since).
	keep := map[string]bool{}
	for _, p := range props {
		if p.Kind == "skip" || p.Kind == "drop" {
			continue
		}
		src := p.Item.Path
		if p.Item.LinkTo != "" {
			if kind := p.Kind; kind != "pack" && !(kind == "enable" && p.Via == "pack") {
				continue
			}
			src = p.Item.LinkTo
		}
		e := entryFor(p)
		if e == nil {
			continue
		}
		h, err := fsutil.HashTree(e.Dir)
		if c := copied[adoptedAs(p.Item)]; c != "" && (p.Kind == "own" || p.Via == "own") {
			// Compared with what was copied, before its name was changed.
			h, err = c, nil
		}
		if err == nil && fsutil.Pristine(src, h) {
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
	// An agent that reads ~/.agents/skills gets an adopted skill from there
	// (engine.Entry.ViaShared): apply makes no link in its own directory,
	// so the copy found there would be a second one it loads. It goes to
	// the trash when it is what the library holds; a copy that differs is
	// left in place with a warning.
	if after, err := s.Resolve(); err == nil {
		for _, p := range props {
			if p.Kind == "skip" || p.Kind == "drop" || keep[p.Item.Target+"/"+p.Item.Name] || !fsutil.Lexists(p.Item.Path) {
				continue
			}
			var e *engine.Entry
			for i := range after {
				if x := &after[i]; slices.Contains(x.ViaShared, p.Item.Target) && x.LinkName == adoptedAs(p.Item) {
					e = x
				}
			}
			if e == nil {
				continue
			}
			t, err := s.Reg.Get(p.Item.Target)
			if err != nil || filepath.Dir(p.Item.Path) != t.Path || t.Intact() != nil {
				continue
			}
			if !sameContent(p.Item, e.Dir) {
				s.logf(Warn, "%s differs from the library's %s, which %s reads from ~/.agents/skills; left in place — compare them and remove one by hand", p.Item.Path, e.LinkName, t.Name)
				continue
			}
			to, err := engine.Trash(lib, p.Item.Path)
			if err != nil {
				return nil, err
			}
			ap.Backups = append(ap.Backups, p.Item.Path+" → "+lib.Rel(to))
		}
	}
	if out.Failed > 0 {
		return out, i18n.Errorf("%d repositories could not be fetched", out.Failed)
	}
	return out, nil
}

// sameContent reports whether the agent entry it holds exactly what dir
// holds: a copy (followed when it is a link) with the same files and
// nothing a hash cannot see.
func sameContent(it scan.Item, dir string) bool {
	src := it.Path
	if it.LinkTo != "" {
		src = it.LinkTo
	}
	h, err := fsutil.HashTree(dir)
	return err == nil && fsutil.Pristine(src, h)
}

func short(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
