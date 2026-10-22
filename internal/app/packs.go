package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/skillmd"
)

// ParseAdd turns what the user typed for `add` (owner/repo, a URL, an
// alias of a subscribed pack, optionally @ref) into a source, and checks
// alias before anything is fetched.
func (s *Session) ParseAdd(arg, alias string) (pack.Source, error) {
	m := s.Lib.Manifest
	if existing := m.FindPack(arg); existing != nil {
		arg = existing.Repo
	}
	src, err := pack.ParseSource(arg)
	if err != nil {
		return pack.Source{}, err
	}
	if err := s.checkAlias(alias, src.Repo); err != nil {
		return pack.Source{}, err
	}
	return src, nil
}

func (s *Session) checkAlias(alias, repo string) error {
	if engine.ReservedQualifier(alias) {
		return i18n.Errorf("--alias %q is reserved (it names your own skills in skills:<name>); pick another", alias)
	}
	return s.Lib.Manifest.CheckAlias(alias, repo)
}

// Fetched is a repository fetched into the cache and scanned.
type Fetched struct {
	Source   pack.Source
	Commit   string
	Clone    string
	Skills   []pack.Skill
	Rejected []pack.Rejected
	// Subscribed is set when the library already follows the repository;
	// Have then marks the skills it selects.
	Subscribed bool
	Have       map[string]bool
}

// Fetch clones or fetches src into the cache and lists its skills. A
// subscribed pack is fetched at its pinned ref, as `add` will vendor it.
func (s *Session) Fetch(src pack.Source) (*Fetched, error) {
	p := s.Lib.Manifest.FindPack(src.Repo)
	ref := src.Ref
	if p != nil {
		ref = p.Ref
	}
	unlock := pack.Hold(src.Repo)
	clone, commit, err := pack.Fetch(pack.Source{Repo: src.Repo, URL: src.URL, Ref: ref})
	if err != nil {
		unlock()
		return nil, err
	}
	up, rejected, err := pack.Scan(clone)
	unlock()
	if err != nil {
		return nil, err
	}
	f := &Fetched{Source: src, Commit: commit, Clone: clone, Skills: up, Rejected: rejected, Have: map[string]bool{}}
	if p != nil {
		f.Subscribed = true
		for _, sk := range up {
			if p.Selected(sk.Name, sk.Category) {
				f.Have[sk.Name] = true
			}
		}
	}
	return f, nil
}

// Selection is what `add` vendors from a fetched repository.
type Selection struct {
	// Skills are upstream names; Categories select every skill in a
	// category (and its subcategories).
	Skills     []string
	Categories []string
	// All follows every skill, including ones added upstream later.
	All bool
	// Alias names the pack (empty keeps the current one).
	Alias string
	// Agents are the pack's targets (empty keeps the current ones, or the
	// manifest default for a new pack).
	Agents []string
	// Own marks the repository as the user's own (manifest.Pack.Own);
	// false keeps what the pack has. Their own repository may have no
	// skill yet: then it is followed whole.
	Own bool
}

// AddOutcome is the result of Add.
type AddOutcome struct {
	Repo   string
	Report *engine.SyncReport
	Apply  *ApplyOutcome
	// Linked are the link names of the pack's entries after the add.
	Linked []string
	// Nothing is set when the selection was empty; nothing was changed.
	Nothing bool
}

// Add subscribes to f's repository (if needed), adds sel to the pack's
// selection, vendors it, saves and links. A newly vendored skill whose
// name is taken is linked as <alias|repo>-<name> (see engine.Sync).
func (s *Session) Add(f *Fetched, sel Selection) (*AddOutcome, error) {
	m := s.Lib.Manifest
	src := f.Source
	if err := s.checkAlias(sel.Alias, src.Repo); err != nil {
		return nil, err
	}
	if sel.Own && len(f.Skills) == 0 {
		sel.All = true
	}
	var names []string
	for _, n := range sel.Skills {
		if pack.Find(f.Skills, n) == nil {
			return nil, i18n.Errorf("no skill %q in %s (try `skd add %s` without --skill to browse)", n, src.Repo, src.Repo)
		}
		names = append(names, n)
	}
	for _, cat := range sel.Categories {
		n := 0
		for _, sk := range f.Skills {
			if sk.Category == cat || strings.HasPrefix(sk.Category, cat+"/") {
				names = append(names, sk.Name)
				n++
			}
		}
		if n == 0 {
			return nil, i18n.Errorf("no category %q in %s (categories: %s)", cat, src.Repo, strings.Join(nonEmpty(pack.Categories(f.Skills)), ", "))
		}
	}
	names = dedupe(names)

	p := m.FindPack(src.Repo)
	isNew := p == nil
	if isNew {
		p = &manifest.Pack{Repo: src.Repo, Ref: src.Ref}
	}
	switch {
	case sel.All:
		p.Use = []string{"*"}
	case p.UsesAll():
		// Following everything already: the chosen ones are un-skipped.
		var skip []string
		for _, sk := range p.Skip {
			drop := false
			for _, n := range names {
				if u := pack.Find(f.Skills, n); u != nil && (sk == u.Name || sk == u.Category+"/"+u.Name) {
					drop = true
				}
			}
			if !drop {
				skip = append(skip, sk)
			}
		}
		p.Skip = skip
	default:
		p.Use = dedupe(append(p.Use, names...))
	}
	for _, n := range names {
		if u := pack.Find(f.Skills, n); u != nil && !p.Selected(u.Name, u.Category) {
			// Only skip entries naming the skill itself are taken out; a
			// wildcard also excludes other skills, so the user decides.
			return nil, i18n.Errorf("%s is excluded by skip = [%q] of %s in skills.toml, which excludes other skills too; "+
				"edit skills.toml: remove %q from that skip (or replace it with the names to keep out), then run skd update %s",
				n, p.SkipPattern(u.Name, u.Category), p.Repo, p.SkipPattern(u.Name, u.Category), p.Repo)
		}
	}
	if len(p.Use) == 0 {
		return &AddOutcome{Repo: src.Repo, Nothing: true}, nil
	}
	if sel.Alias != "" {
		p.Alias = sel.Alias
	}
	wasOwn := p.Own
	if sel.Own {
		p.Own = true
	}
	if len(sel.Agents) > 0 {
		t := append([]string{}, sel.Agents...)
		p.Targets = &t
	}
	if isNew {
		m.Packs = append(m.Packs, p)
	}
	rep, err := engine.Sync(s.Lib, p)
	if err != nil {
		if isNew {
			m.Packs = m.Packs[:len(m.Packs)-1]
		}
		p.Own = wasOwn
		return nil, err
	}
	s.recordBaseline(p.Repo, rep)
	ap, err := s.saveAndApply()
	if err != nil {
		return nil, err
	}
	out := &AddOutcome{Repo: p.Repo, Report: rep, Apply: ap, Linked: []string{}}
	entries, _ := s.Resolve()
	for _, e := range entries {
		if e.Kind == engine.KindVendor && e.Origin == p.Repo {
			out.Linked = append(out.Linked, e.LinkName)
		}
	}
	return out, nil
}

// Subscribe adds p to the manifest and vendors it, then saves and links.
// On a failed fetch the manifest is left as it was.
func (s *Session) Subscribe(p *manifest.Pack) (*engine.SyncReport, *ApplyOutcome, error) {
	m := s.Lib.Manifest
	m.Packs = append(m.Packs, p)
	rep, err := engine.Sync(s.Lib, p)
	if err != nil {
		m.Packs = m.Packs[:len(m.Packs)-1]
		return nil, nil, err
	}
	s.recordBaseline(p.Repo, rep)
	ap, err := s.saveAndApply()
	return rep, ap, err
}

// Failure is a pack that could not be updated.
type Failure struct {
	Repo  string `json:"repo"`
	Error string `json:"error"`
}

// UpdateOutcome is the result of Update.
type UpdateOutcome struct {
	Reports []*engine.SyncReport
	Failed  []Failure
	Apply   *ApplyOutcome
	// Changed is set when any pack's vendored content or names changed.
	Changed bool
	// History are the records written to the history file (HistoryFile),
	// one per pack that moved to a new commit or changed.
	History []UpdateRecord
}

// Update fetches the named packs (every pack when none) and re-vendors
// their selections. A pack that fails is reported and the rest are still
// saved and linked. each, when set, is told about every pack as it is done.
func (s *Session) Update(repos []string, each func(repo string, rep *engine.SyncReport, err error)) (*UpdateOutcome, error) {
	m := s.Lib.Manifest
	var packs []*manifest.Pack
	if len(repos) == 0 {
		packs = m.Packs
	}
	for _, r := range repos {
		p := m.FindPack(r)
		if p == nil {
			return nil, i18n.Errorf("no pack %q", r)
		}
		packs = append(packs, p)
	}
	out := &UpdateOutcome{Reports: []*engine.SyncReport{}, Failed: []Failure{}, History: []UpdateRecord{}}
	updated := map[string]string{}
	var recs []*UpdateRecord
	now := time.Now()
	for _, p := range packs {
		unlock := pack.Hold(p.Repo)
		known := s.knownUpstream(p.Repo)
		unlock()
		rep, err := engine.Sync(s.Lib, p)
		if err == nil {
			s.markUpstreamNew(p, rep, known)
		}
		if each != nil {
			each(p.Repo, rep, err)
		}
		if err != nil {
			out.Failed = append(out.Failed, Failure{p.Repo, err.Error()})
			continue
		}
		out.Reports = append(out.Reports, rep)
		out.Changed = out.Changed || len(rep.Changed)+len(rep.Added)+len(rep.Dropped)+len(rep.Shelved) > 0 || len(rep.Renamed) > 0
		updated[p.Repo] = rep.NewCommit
		if r := newRecord(rep, now); r != nil {
			unlock := pack.Hold(p.Repo)
			fillCommits(context.Background(), r, p.Ref, rep.Upstream)
			unlock()
			r.sanitize()
			recs = append(recs, r)
		}
	}
	// Once the lock is saved the update has happened, and it is recorded
	// even when linking fails (the record says so; skd apply finishes
	// it). When the lock cannot be saved nothing is recorded: the next
	// update starts from the old commit again and records it then.
	err := s.Lib.Save()
	saved := err == nil
	if saved {
		out.Apply, err = s.Apply(ApplyOptions{})
		if err != nil {
			for _, r := range recs {
				// The history is a file of the library: English, whatever language is in use.
				r.ApplyError = CleanText(i18n.English(err), MaxSubject)
			}
		}
		if herr := s.appendHistory(recs); herr != nil {
			s.logf(Warn, "could not write the update history (%s): %v", HistoryFile, herr)
		} else {
			for _, r := range recs {
				out.History = append(out.History, *r)
			}
		}
		// The updated packs now match upstream: `list` and the menu stop
		// counting them.
		if rerr := s.refreshRecorded(updated); rerr != nil {
			s.logf(Warn, "could not record the update for skd outdated: %v", rerr)
		}
	}
	return out, err
}

// knownUpstream is the set of skill names a pack's upstream had before
// this update: what the lock recorded at the last update or, for a lock
// written before that was recorded, what the cache clone holds now. It is
// nil when neither is known (a pack never fetched here). Callers Hold the
// repository.
func (s *Session) knownUpstream(repo string) map[string]bool {
	lp := s.Lib.Lock.Packs[repo]
	if lp == nil || lp.Commit == "" {
		return nil
	}
	known := map[string]bool{}
	if len(lp.Upstream) > 0 {
		for _, n := range lp.Upstream {
			known[n] = true
		}
		return known
	}
	dir := pack.CachePath(repo)
	if !fsutil.Exists(filepath.Join(dir, ".git")) {
		return nil
	}
	up, _, err := pack.Scan(dir)
	if err != nil {
		return nil
	}
	for _, sk := range up {
		known[sk.Name] = true
	}
	return known
}

// markUpstreamNew fills rep.UpstreamNew with the unselected skills that
// are not in known, and records the upstream's names in the lock as the
// next update's baseline. With no baseline nothing is called new: every
// unselected skill would be, on every update.
func (s *Session) markUpstreamNew(p *manifest.Pack, rep *engine.SyncReport, known map[string]bool) {
	rep.UpstreamNew = nil
	var names []string
	for _, sk := range rep.Upstream {
		names = append(names, sk.Name)
		if known != nil && !known[sk.Name] && !p.Selected(sk.Name, sk.Category) {
			rep.UpstreamNew = append(rep.UpstreamNew, sk.Name)
		}
	}
	sort.Strings(rep.UpstreamNew)
	sort.Strings(names)
	if lp := s.Lib.Lock.Packs[p.Repo]; lp != nil {
		lp.Upstream = dedupe(names)
	}
}

// recordBaseline gives a pack that has no upstream list in the lock yet
// (just subscribed, or a lock from before the list existed) the skills
// upstream has now, so the next `update` and `outdated` can tell what is
// new without the cache clone, which a check replaces. An existing list
// is kept: it is the last update's, and what appeared since is still new.
func (s *Session) recordBaseline(repo string, rep *engine.SyncReport) {
	lp := s.Lib.Lock.Packs[repo]
	if lp == nil || len(lp.Upstream) > 0 {
		return
	}
	var names []string
	for _, sk := range rep.Upstream {
		names = append(names, sk.Name)
	}
	sort.Strings(names)
	lp.Upstream = dedupe(names)
}

// deselect drops a vendored skill from its pack's selection.
func deselect(p *manifest.Pack, name string) {
	if p.UsesAll() {
		p.Skip = dedupe(append(p.Skip, name))
	} else {
		var use []string
		for _, u := range p.Use {
			if !manifest.UseNames(u, name) {
				use = append(use, u)
			}
		}
		p.Use = use
	}
	delete(p.Skill, name)
}

// RemoveOutcome is the result of Remove.
type RemoveOutcome struct {
	Entry engine.Entry
	// Purged is set when the skill was its pack's last one and the whole
	// pack was dropped.
	Purged *PurgeOutcome
	Apply  *ApplyOutcome
	// Trash is the ID of the trash entry the skill's files went to (""
	// when nothing was there to move); `skd trash restore <id>` undoes
	// the remove.
	Trash string
}

// Remove deselects a vendored skill: it is unlinked everywhere and its
// files go to the trash. Removing a pack's last skill drops the pack.
// The trash entry remembers what restoring it needs to select the skill
// again (engine.TrashNote).
func (s *Session) Remove(r Ref) (*RemoveOutcome, error) {
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	e, err := s.mustEntry(entries, r)
	if err != nil {
		return nil, err
	}
	if e.Kind == engine.KindSkill || e.Kind == engine.KindPrivate {
		if err := s.Lib.Writable(); err != nil {
			return nil, err
		}
		if err := s.writableInLibrary(e.Dir); err != nil {
			return nil, err
		}
		to, err := engine.TrashLocal(s.Lib, e.Dir)
		if err != nil {
			return nil, err
		}
		s.noteWarn(engine.SetTrashNote(to, &engine.TrashNote{Reason: engine.ReasonRemove, Skills: []string{e.Name}}))
		// Keep local overrides so restoring the directory restores its selections.
		ap, err := s.Apply(ApplyOptions{})
		return &RemoveOutcome{Entry: *e, Apply: ap, Trash: engine.TrashID(s.Lib, to)}, err
	}

	if e.Kind != engine.KindVendor {
		return nil, i18n.Errorf("%q is a %s skill; use `skd disable` to unlink it, or delete the directory to drop it", r.String(), e.Kind)
	}
	out := &RemoveOutcome{Entry: *e}
	p := s.Lib.Manifest.FindPack(e.Origin)
	was := snapshotPack(p)
	wasLock := snapshotLock(s.Lib.Lock.Packs[p.Repo])
	// Listed before the skill is deselected, while it still has links.
	links := s.PurgeLinks(p.Repo)
	deselect(p, e.Name)
	if len(p.Use) == 0 && !p.UsesAll() {
		// Removing the last skill of a pack is the whole pack: the user
		// named exactly what goes, so no further confirmation.
		out.Purged, err = s.purge(p, links, &engine.TrashNote{Reason: engine.ReasonRemove, Skills: []string{e.Name}, Repo: p.Repo, Pack: was, Lock: wasLock})
		if err != nil {
			return nil, err
		}
		out.Trash = out.Purged.Trash
		return out, nil
	}
	rep, err := engine.Sync(s.Lib, p)
	if err != nil {
		return nil, err
	}
	if to := rep.Trashed[e.Name]; to != "" {
		var lp *lock.Pack
		if wasLock != nil && wasLock.Skills[e.Name] != nil {
			lp = &lock.Pack{Commit: wasLock.Commit, Ref: wasLock.Ref, Upstream: wasLock.Upstream,
				Skills: map[string]*lock.Skill{e.Name: wasLock.Skills[e.Name]}}
		}
		s.noteWarn(engine.SetTrashNote(to, &engine.TrashNote{Reason: engine.ReasonRemove, Skills: []string{e.Name}, Repo: p.Repo, Pack: was, Lock: lp}))
		out.Trash = engine.TrashID(s.Lib, to)
	}
	out.Apply, err = s.saveAndApply()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// snapshotPack is a deep copy of p, as it is now.
func snapshotPack(p *manifest.Pack) *manifest.Pack {
	if p == nil {
		return nil
	}
	var c manifest.Pack
	if b, err := json.Marshal(p); err == nil && json.Unmarshal(b, &c) == nil {
		return &c
	}
	return nil
}

// snapshotLock is a deep copy of lp, as it is now.
func snapshotLock(lp *lock.Pack) *lock.Pack {
	if lp == nil {
		return nil
	}
	var c lock.Pack
	if b, err := json.Marshal(lp); err == nil && json.Unmarshal(b, &c) == nil {
		return &c
	}
	return nil
}

// PurgeOutcome is the result of Purge; its JSON form is what
// `skd purge --json` prints.
type PurgeOutcome struct {
	Repo     string        `json:"repo"`
	Unlinked []string      `json:"unlinked"`
	Apply    *ApplyOutcome `json:"apply"`
	// Trash is the ID of the trash entry vendor/<repo> went to ("" when
	// nothing was vendored); `skd trash restore <id>` brings the pack back.
	Trash string `json:"trash,omitempty"`
}

// PurgeLinks lists the "<target>/<link>" entries purging repo removes.
func (s *Session) PurgeLinks(repo string) []string {
	entries, _ := s.Resolve()
	links := []string{}
	for _, e := range entries {
		if e.Kind == engine.KindVendor && e.Origin == repo {
			for _, t := range e.Targets {
				links = append(links, t.Name+"/"+e.LinkName)
			}
		}
	}
	sort.Strings(links)
	return links
}

// Purge drops a pack from the manifest, the lock, vendor/ (to the trash)
// and every agent directory. Callers confirm first.
func (s *Session) Purge(repo string) (*PurgeOutcome, error) {
	p := s.Lib.Manifest.FindPack(repo)
	if p == nil {
		return nil, i18n.Errorf("no pack %q", repo)
	}
	note := &engine.TrashNote{Reason: engine.ReasonPurge, Repo: p.Repo, Pack: snapshotPack(p), Lock: snapshotLock(s.Lib.Lock.Packs[p.Repo])}
	if note.Lock != nil {
		for n := range note.Lock.Skills {
			note.Skills = append(note.Skills, n)
		}
		sort.Strings(note.Skills)
	}
	return s.purge(p, s.PurgeLinks(p.Repo), note)
}

// purge drops p; note is what the trash entry of its vendored files
// remembers, so that restoring it brings the pack back.
func (s *Session) purge(p *manifest.Pack, links []string, note *engine.TrashNote) (*PurgeOutcome, error) {
	m := s.Lib.Manifest
	for i, q := range m.Packs {
		if q == p {
			m.Packs = append(m.Packs[:i], m.Packs[i+1:]...)
			break
		}
	}
	ap, err := s.saveAndApply()
	if err != nil {
		return nil, err
	}
	to, err := engine.RemovePackTo(s.Lib, p.Repo)
	if err != nil {
		return nil, err
	}
	if err := s.Lib.Save(); err != nil {
		return nil, err
	}
	out := &PurgeOutcome{Repo: p.Repo, Unlinked: links, Apply: ap}
	if to != "" {
		s.noteWarn(engine.SetTrashNote(to, note))
		out.Trash = engine.TrashID(s.Lib, to)
	}
	return out, nil
}

// ForkOutcome is the result of Fork.
type ForkOutcome struct {
	From  engine.Entry
	Name  string
	Path  string
	Apply *ApplyOutcome
}

// Fork copies a vendored or app skill into skills/ (private/ when private)
// as name (its link name when empty), so the user can edit it. The
// original is deselected (vendored) or unregistered (app) so the names do
// not collide; the fork's frontmatter name is set to its own name.
func (s *Session) Fork(r Ref, private bool, name string) (*ForkOutcome, error) {
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	e, err := s.mustEntry(entries, r)
	if err != nil {
		return nil, err
	}
	if e.Kind != engine.KindVendor && e.Kind != engine.KindApp {
		return nil, i18n.Errorf("%q is already yours (%s)", e.LinkName, e.Dir)
	}
	if e.Refused != "" {
		return nil, i18n.Errorf("%q: %s; skd does not copy it", e.LinkName, e.Refused)
	}
	if e.LinkedVia != "" {
		return nil, i18n.Errorf("%q: its directory goes through %s, a symbolic link inside the library; skd does not copy it", e.LinkName, s.Lib.Rel(e.LinkedVia))
	}
	if name == "" {
		name = e.LinkName
	} else if err := manifest.CheckName(name); err != nil {
		return nil, i18n.Errorf("--as: %w", err)
	}
	base := s.Lib.SkillsDir()
	if private {
		base = s.Lib.PrivateDir()
	}
	dest := filepath.Join(base, name)
	if err := s.writableInLibrary(dest); err != nil {
		return nil, err
	}
	if fsutil.Lexists(dest) {
		return nil, i18n.Errorf("%s already exists", dest)
	}
	if err := fsutil.CopyTree(e.Dir, dest); err != nil {
		return nil, err
	}
	// The fork is known by its directory name; make the frontmatter
	// agree, or an aliased fork would clash with the upstream name.
	if meta, err := skillmd.Read(dest); err == nil && meta.Name != name {
		if err := skillmd.SetName(dest, name); err != nil {
			return nil, err
		}
	}
	note := fmt.Sprintf("\n<!-- forked by skilldots from %s -->\n", engine.Describe(*e))
	if f, err := os.OpenFile(filepath.Join(dest, "SKILL.md"), os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		f.WriteString(note)
		f.Close()
	}
	if e.Kind == engine.KindVendor {
		p := s.Lib.Manifest.FindPack(e.Origin)
		deselect(p, e.Name)
		if _, err := engine.Sync(s.Lib, p); err != nil {
			return nil, err
		}
	} else {
		apps := s.Lib.Manifest.Apps
		for i, a := range apps {
			if a.Name == e.Name {
				s.Lib.Manifest.Apps = append(apps[:i], apps[i+1:]...)
				break
			}
		}
	}
	ap, err := s.saveAndApply()
	if err != nil {
		return nil, err
	}
	return &ForkOutcome{From: *e, Name: name, Path: dest, Apply: ap}, nil
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
