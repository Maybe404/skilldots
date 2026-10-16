package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/targets"
)

// Ops is everything the dashboard can do to a library. Each method opens
// the library afresh, makes one change, saves, re-links and returns a one
// line summary. Nothing here deletes: removed files go to the library
// trash, exactly like the CLI.
type Ops interface {
	Load() (*Snapshot, error)
	// SetTargets sets the agents for a skill (s non-nil) or a pack.
	SetTargets(s *Skill, repo string, agents []string) (string, error)
	// Browse fetches a repository and lists its skills without installing.
	Browse(src string) (*Browse, error)
	// Add subscribes to src (if needed) and vendors the named skills, or
	// follows everything when all is set.
	Add(src string, names []string, all bool) (string, error)
	Update(repos []string) ([]*engine.SyncReport, string, error)
	Fork(s Skill, private bool) (string, error)
	// Trash takes a skill out of use: a vendored skill is deselected and
	// its files go to the trash; anything else is unlinked and kept.
	Trash(s Skill) (string, error)
	Purge(repo string) (string, error)
	Apply() (string, error)
	Sync() (string, error)
}

// Browse is a fetched repository's skill list.
type Browse struct {
	Repo   string
	Commit string
	Skills []pack.Skill
	// Have marks skills already selected in the library.
	Have map[string]bool
}

// libOps implements Ops on a library directory.
type libOps struct {
	root string
	// self is the skd binary, used for `sync` so the backup path is the
	// exact one the CLI uses.
	self string
}

func (o *libOps) Load() (*Snapshot, error) { return LoadSnapshot(o.root) }

// apply re-links the library to match the manifest and saves it, like
// `skd apply` without --force.
func apply(lib *library.Library, reg *targets.Registry) (string, error) {
	entries, err := engine.Resolve(lib, reg)
	if err != nil {
		return "", err
	}
	plan := engine.MakePlan(lib, entries, false)
	var res *engine.Result
	if plan.Changes() {
		res, err = engine.Apply(lib, plan)
	}
	if saveErr := lib.Save(); saveErr != nil && err == nil {
		err = saveErr
	}
	if err != nil {
		return "", err
	}
	n := plan.Counts()
	var parts []string
	if n["create"]+n["replace"]+n["remove"] > 0 {
		parts = append(parts, fmt.Sprintf("+%d ↻%d −%d", n["create"], n["replace"], n["remove"]))
	}
	if n["skip"] > 0 {
		var why []string
		for _, op := range plan.Ops {
			if op.Action == "skip" {
				why = append(why, op.Target.Name+"/"+op.Name)
			}
		}
		parts = append(parts, fmt.Sprintf("%d skipped (%s; skd apply --force moves it to the trash)", n["skip"], strings.Join(why, ", ")))
	}
	if res != nil && len(res.Backups) > 0 {
		parts = append(parts, fmt.Sprintf("%d moved to trash", len(res.Backups)))
	}
	return strings.Join(parts, " · "), nil
}

func (o *libOps) Apply() (string, error) {
	lib, reg, err := openLibrary(o.root)
	if err != nil {
		return "", err
	}
	sum, err := apply(lib, reg)
	if err != nil {
		return "", err
	}
	if sum == "" {
		return "already up to date", nil
	}
	return "applied " + sum, nil
}

func (o *libOps) SetTargets(s *Skill, repo string, agents []string) (string, error) {
	lib, reg, err := openLibrary(o.root)
	if err != nil {
		return "", err
	}
	m := lib.Manifest
	next := append([]string{}, agents...)
	label := repo
	if s == nil {
		p := m.FindPack(repo)
		if p == nil {
			return "", fmt.Errorf("no pack %q", repo)
		}
		p.Targets = &next
	} else {
		entries, err := engine.Resolve(lib, reg)
		if err != nil {
			return "", err
		}
		e := engine.Find(entries, s.LinkName)
		if e == nil {
			return "", fmt.Errorf("no skill %q", s.LinkName)
		}
		label = e.LinkName
		setEntryTargets(m, e, next)
	}
	sum, err := apply(lib, reg)
	if err != nil {
		return "", err
	}
	msg := label + " → " + joinOr(next, "no agents (kept in library)")
	if sum != "" {
		msg += " · " + sum
	}
	return msg, nil
}

// setEntryTargets records next as the target list of e, at the same
// place in skills.toml `skd enable/disable` writes it.
func setEntryTargets(m *manifest.Manifest, e *engine.Entry, next []string) {
	switch e.Kind {
	case engine.KindVendor:
		m.FindPack(e.Origin).Override(e.Name, true).Targets = &next
	case engine.KindApp:
		for _, a := range m.Apps {
			if a.Name == e.Name {
				a.Targets = &next
			}
		}
	default:
		for _, l := range m.Locals {
			if l.Name == e.Name {
				l.Targets = &next
				return
			}
		}
		m.Locals = append(m.Locals, &manifest.Local{Name: e.Name, Targets: &next})
	}
}

func (o *libOps) Browse(arg string) (*Browse, error) {
	lib, _, err := openLibrary(o.root)
	if err != nil {
		return nil, err
	}
	if existing := lib.Manifest.FindPack(arg); existing != nil {
		arg = existing.Repo
	}
	src, err := pack.ParseSource(arg)
	if err != nil {
		return nil, err
	}
	p := lib.Manifest.FindPack(src.Repo)
	if p != nil && src.Ref == "" {
		src.Ref = p.Ref
	}
	clone, commit, err := pack.Fetch(src)
	if err != nil {
		return nil, err
	}
	up, err := pack.Scan(clone)
	if err != nil {
		return nil, err
	}
	if len(up) == 0 {
		return nil, fmt.Errorf("%s contains no SKILL.md", src.Repo)
	}
	b := &Browse{Repo: src.Repo, Commit: commit, Skills: up, Have: map[string]bool{}}
	for _, s := range up {
		if p != nil && p.Selected(s.Name, s.Category) {
			b.Have[s.Name] = true
		}
	}
	return b, nil
}

// Add mirrors `skd add <src> --skill a,b` (or --all).
func (o *libOps) Add(arg string, names []string, all bool) (string, error) {
	lib, reg, err := openLibrary(o.root)
	if err != nil {
		return "", err
	}
	m := lib.Manifest
	if existing := m.FindPack(arg); existing != nil {
		arg = existing.Repo
	}
	src, err := pack.ParseSource(arg)
	if err != nil {
		return "", err
	}
	p := m.FindPack(src.Repo)
	if p == nil {
		p = &manifest.Pack{Repo: src.Repo, Ref: src.Ref}
		m.Packs = append(m.Packs, p)
	}
	clone, _, err := pack.Fetch(pack.Source{Repo: src.Repo, URL: src.URL, Ref: p.Ref})
	if err != nil {
		return "", err
	}
	up, err := pack.Scan(clone)
	if err != nil {
		return "", err
	}
	switch {
	case all:
		p.Use = []string{"*"}
	case p.UsesAll():
		// Following everything already: un-skip the chosen ones.
		var skip []string
		for _, sk := range p.Skip {
			drop := false
			for _, n := range names {
				if s := pack.Find(up, n); s != nil && (sk == s.Name || sk == s.Category+"/"+s.Name) {
					drop = true
				}
			}
			if !drop {
				skip = append(skip, sk)
			}
		}
		p.Skip = skip
	default:
		for _, n := range names {
			if pack.Find(up, n) == nil {
				return "", fmt.Errorf("no skill %q in %s", n, src.Repo)
			}
		}
		p.Use = dedupe(append(p.Use, names...))
	}
	for _, n := range names {
		if s := pack.Find(up, n); s != nil && !p.Selected(s.Name, s.Category) {
			return "", fmt.Errorf("%s is excluded by a skip pattern in skills.toml", n)
		}
	}
	// Give new skills a free link name, as `skd add` does.
	entries, _ := engine.Resolve(lib, reg)
	var renamed []string
	for _, s := range up {
		if !p.Selected(s.Name, s.Category) {
			continue
		}
		if prev := engine.Find(entries, s.Name); prev != nil && !(prev.Kind == engine.KindVendor && prev.Origin == p.Repo) {
			if ov := p.Override(s.Name, true); ov.As == "" {
				ov.As = engine.FreeName(entries, s.Name, p.Prefix())
				renamed = append(renamed, s.Name+" as "+ov.As)
			}
		}
	}
	rep, err := engine.Sync(lib, p)
	if err != nil {
		return "", err
	}
	if err := lib.Save(); err != nil {
		return "", err
	}
	sum, err := apply(lib, reg)
	if err != nil {
		return "", err
	}
	msg := fmt.Sprintf("vendored %d from %s @ %s", len(rep.Added)+len(rep.Changed)+len(rep.Unchanged), p.Repo, short(rep.NewCommit))
	if len(renamed) > 0 {
		msg += " · linked " + strings.Join(renamed, ", ")
	}
	if rep.NoLicense {
		msg += " · no license found: check before redistributing"
	}
	if sum != "" {
		msg += " · " + sum
	}
	return msg, nil
}

// Update mirrors `skd update [pack…]`.
func (o *libOps) Update(repos []string) ([]*engine.SyncReport, string, error) {
	lib, reg, err := openLibrary(o.root)
	if err != nil {
		return nil, "", err
	}
	var packs []*manifest.Pack
	if len(repos) == 0 {
		packs = lib.Manifest.Packs
	}
	for _, r := range repos {
		p := lib.Manifest.FindPack(r)
		if p == nil {
			return nil, "", fmt.Errorf("no pack %q", r)
		}
		packs = append(packs, p)
	}
	if len(packs) == 0 {
		return nil, "no packs to update", nil
	}
	var reports []*engine.SyncReport
	var failed []string
	for _, p := range packs {
		rep, err := engine.Sync(lib, p)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", p.Repo, err))
			continue
		}
		reports = append(reports, rep)
	}
	if err := lib.Save(); err != nil {
		return reports, "", err
	}
	sum, err := apply(lib, reg)
	if err != nil {
		return reports, "", err
	}
	changed, added, fresh := 0, 0, 0
	for _, r := range reports {
		changed += len(r.Changed)
		added += len(r.Added)
		fresh += len(r.UpstreamNew)
	}
	msg := fmt.Sprintf("updated %d pack(s): %d changed · %d added · %d new upstream", len(reports), changed, added, fresh)
	if sum != "" {
		msg += " · " + sum
	}
	if len(failed) > 0 {
		return reports, msg, fmt.Errorf("%s", strings.Join(failed, "; "))
	}
	return reports, msg, nil
}

// deselect drops a vendored skill from its pack's selection.
func deselect(p *manifest.Pack, e *engine.Entry) {
	if p.UsesAll() {
		p.Skip = dedupe(append(p.Skip, e.Name))
	} else {
		var use []string
		for _, u := range p.Use {
			if u != e.Name && u != e.Category+"/"+e.Name {
				use = append(use, u)
			}
		}
		p.Use = use
	}
	delete(p.Skill, e.Name)
}

// Fork mirrors `skd fork <skill> [--private]`.
func (o *libOps) Fork(s Skill, private bool) (string, error) {
	lib, reg, err := openLibrary(o.root)
	if err != nil {
		return "", err
	}
	entries, err := engine.Resolve(lib, reg)
	if err != nil {
		return "", err
	}
	e := engine.Find(entries, s.LinkName)
	if e == nil {
		return "", fmt.Errorf("no skill %q", s.LinkName)
	}
	if e.Kind != engine.KindVendor && e.Kind != engine.KindApp {
		return "", fmt.Errorf("%s is already yours (%s)", e.LinkName, e.Dir)
	}
	base := lib.SkillsDir()
	if private {
		base = lib.PrivateDir()
	}
	dest := filepath.Join(base, e.LinkName)
	if fsutil.Lexists(dest) {
		return "", fmt.Errorf("%s already exists", dest)
	}
	if err := fsutil.CopyTree(e.Dir, dest); err != nil {
		return "", err
	}
	note := fmt.Sprintf("\n<!-- forked by skilldots from %s -->\n", engine.Describe(*e))
	if f, err := os.OpenFile(filepath.Join(dest, "SKILL.md"), os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		f.WriteString(note)
		f.Close()
	}
	if e.Kind == engine.KindVendor {
		p := lib.Manifest.FindPack(e.Origin)
		deselect(p, e)
		if _, err := engine.Sync(lib, p); err != nil {
			return "", err
		}
	} else {
		apps := lib.Manifest.Apps
		for i, a := range apps {
			if a.Name == e.Name {
				lib.Manifest.Apps = append(apps[:i], apps[i+1:]...)
				break
			}
		}
	}
	if err := lib.Save(); err != nil {
		return "", err
	}
	sum, err := apply(lib, reg)
	if err != nil {
		return "", err
	}
	msg := "forked " + e.LinkName + " → " + lib.Rel(dest)
	if sum != "" {
		msg += " · " + sum
	}
	return msg, nil
}

// Trash mirrors `skd remove` for vendored skills and `skd disable` for
// everything else (the CLI never deletes your own or an app's files).
func (o *libOps) Trash(s Skill) (string, error) {
	if s.Kind != engine.KindVendor {
		if _, err := o.SetTargets(&s, "", []string{}); err != nil {
			return "", err
		}
		return "unlinked " + s.LinkName + " from every agent; its files stay in " + tildify(s.Dir), nil
	}
	lib, reg, err := openLibrary(o.root)
	if err != nil {
		return "", err
	}
	entries, err := engine.Resolve(lib, reg)
	if err != nil {
		return "", err
	}
	e := engine.Find(entries, s.LinkName)
	if e == nil {
		return "", fmt.Errorf("no skill %q", s.LinkName)
	}
	p := lib.Manifest.FindPack(e.Origin)
	deselect(p, e)
	if len(p.Use) == 0 && !p.UsesAll() {
		msg, err := purge(lib, reg, p)
		if err != nil {
			return "", err
		}
		return p.Repo + " had no skills left · " + msg, nil
	}
	if _, err := engine.Sync(lib, p); err != nil {
		return "", err
	}
	if err := lib.Save(); err != nil {
		return "", err
	}
	sum, err := apply(lib, reg)
	if err != nil {
		return "", err
	}
	msg := "removed " + e.LinkName + "; its files are in the library trash (skd trash)"
	if sum != "" {
		msg += " · " + sum
	}
	return msg, nil
}

// Purge mirrors `skd purge <pack>` once the user has confirmed.
func (o *libOps) Purge(repo string) (string, error) {
	lib, reg, err := openLibrary(o.root)
	if err != nil {
		return "", err
	}
	p := lib.Manifest.FindPack(repo)
	if p == nil {
		return "", fmt.Errorf("no pack %q", repo)
	}
	return purge(lib, reg, p)
}

func purge(lib *library.Library, reg *targets.Registry, p *manifest.Pack) (string, error) {
	m := lib.Manifest
	for i, q := range m.Packs {
		if q == p {
			m.Packs = append(m.Packs[:i], m.Packs[i+1:]...)
			break
		}
	}
	if err := lib.Save(); err != nil {
		return "", err
	}
	sum, err := apply(lib, reg)
	if err != nil {
		return "", err
	}
	if err := engine.RemovePack(lib, p.Repo); err != nil {
		return "", err
	}
	if err := lib.Save(); err != nil {
		return "", err
	}
	msg := "purged " + p.Repo + "; vendor/" + p.Repo + " is in the library trash (skd trash)"
	if sum != "" {
		msg += " · " + sum
	}
	return msg, nil
}

// Sync runs `skd sync`, so backups go through exactly one code path.
func (o *libOps) Sync() (string, error) {
	self := o.self
	if self == "" {
		var err error
		if self, err = os.Executable(); err != nil {
			return "", err
		}
	}
	cmd := exec.Command(self, "sync", "--library", o.root)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var clean []string
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			clean = append(clean, l)
		}
	}
	if err != nil {
		if len(clean) > 0 {
			return "", fmt.Errorf("%s", strings.TrimPrefix(clean[len(clean)-1], "error: "))
		}
		return "", err
	}
	if len(clean) == 0 {
		return "backed up", nil
	}
	return strings.Join(clean, " · "), nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func short(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func joinOr(items []string, empty string) string {
	if len(items) == 0 {
		return empty
	}
	c := append([]string{}, items...)
	sort.Strings(c)
	return strings.Join(c, ", ")
}
