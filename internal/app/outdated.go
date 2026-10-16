package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
)

// StaleAfter is how old a recorded upstream check may be before `list`,
// the menu and the dashboard call it stale.
const StaleAfter = 24 * time.Hour

// PackStatus is what checking one pack's upstream found, skill by skill,
// against what the library has vendored. Nothing in the library changes.
type PackStatus struct {
	Repo string `json:"repo"`
	// Commit is the vendored commit (the lock's); Latest is the upstream
	// HEAD just fetched (at the pack's ref).
	Commit string `json:"commit"`
	Latest string `json:"latest"`
	// Changed are selected, vendored skills whose upstream content (or
	// category) differs from what is vendored: `update` re-vendors them.
	Changed []string `json:"changed"`
	// New are upstream skills that appeared since the last `update` (the
	// lock's upstream list) and are not vendored: offered by `add`, or
	// vendored by `update` when the pack follows everything.
	New []string `json:"new"`
	// Gone are vendored skills that upstream had at the last `update` and
	// no longer has; `update` keeps them.
	Gone []string `json:"gone"`
	// Pending are selected skills not vendored yet: `update` vendors them.
	Pending []string  `json:"pending"`
	Checked time.Time `json:"checked"`
}

// HasUpdates reports whether `skd update` would change or report
// anything for the pack beyond a new commit.
func (p PackStatus) HasUpdates() bool {
	return p.Commit == "" || len(p.Changed)+len(p.New)+len(p.Gone)+len(p.Pending) > 0
}

// Outdated is the result of checking upstream, and what is recorded of it
// in the user's state file.
type Outdated struct {
	// Checked is when the oldest of the recorded pack checks was made.
	Checked time.Time    `json:"checked"`
	Packs   []PackStatus `json:"packs"`
	Failed  []Failure    `json:"failed"`
}

// Updates is the number of packs with updates.
func (o *Outdated) Updates() int {
	n := 0
	for _, p := range o.Packs {
		if p.HasUpdates() {
			n++
		}
	}
	return n
}

// Stale reports whether the check is older than StaleAfter.
func (o *Outdated) Stale(now time.Time) bool { return now.Sub(o.Checked) > StaleAfter }

// Find returns the status of repo, or nil.
func (o *Outdated) Find(repo string) *PackStatus {
	for i := range o.Packs {
		if o.Packs[i].Repo == repo {
			return &o.Packs[i]
		}
	}
	return nil
}

// Outdated fetches the named packs (every pack when none) into the cache
// and compares them with the lock, skill by skill. vendor/, skills.lock
// and skills.toml are not touched. A pack that cannot be fetched is listed
// in Failed and the rest are still checked. The result is recorded in the
// user's state file (see OutdatedPath) for `list`, the menu and the
// dashboard. each, when set, is told about every pack as it is done.
func (s *Session) Outdated(repos []string, each func(repo string, st *PackStatus, err error)) (*Outdated, error) {
	m := s.Lib.Manifest
	var packs []*manifest.Pack
	if len(repos) == 0 {
		packs = m.Packs
	}
	for _, r := range repos {
		p := m.FindPack(r)
		if p == nil {
			return nil, fmt.Errorf("no pack %q", r)
		}
		packs = append(packs, p)
	}
	out := &Outdated{Checked: time.Now(), Packs: []PackStatus{}, Failed: []Failure{}}
	for _, p := range packs {
		st, err := s.checkPack(p)
		if each != nil {
			each(p.Repo, st, err)
		}
		if err != nil {
			out.Failed = append(out.Failed, Failure{p.Repo, err.Error()})
			continue
		}
		out.Packs = append(out.Packs, *st)
	}
	if err := s.record(out); err != nil {
		s.logf(Warn, "could not record the check: %v", err)
	}
	return out, nil
}

// checkPack fetches one pack's upstream into the cache and compares it
// with the lock.
func (s *Session) checkPack(p *manifest.Pack) (*PackStatus, error) {
	src, err := pack.ParseSource(p.Repo)
	if err != nil {
		return nil, err
	}
	src.Ref = p.Ref
	defer pack.Hold(p.Repo)()
	clone, commit, err := pack.Fetch(src)
	if err != nil {
		return nil, err
	}
	upstream, rejected, err := pack.Scan(clone)
	if err != nil {
		return nil, err
	}
	if len(upstream) == 0 {
		if len(rejected) > 0 {
			return nil, fmt.Errorf("%s: no usable SKILL.md (%s)", p.Repo, rejected[0])
		}
		return nil, fmt.Errorf("%s: no SKILL.md found", p.Repo)
	}
	st := &PackStatus{Repo: p.Repo, Latest: commit, Checked: time.Now(),
		Changed: []string{}, New: []string{}, Gone: []string{}, Pending: []string{}}
	lp := s.Lib.Lock.Packs[p.Repo]
	vendored := map[string]*lock.Skill{}
	// The baseline for "new" is the lock's record of the last update only:
	// the cache clone, update's fallback for old locks, was just replaced.
	var known map[string]bool
	if lp != nil {
		st.Commit = lp.Commit
		vendored = lp.Skills
		if len(lp.Upstream) > 0 {
			known = map[string]bool{}
			for _, n := range lp.Upstream {
				known[n] = true
			}
		}
	}
	up := map[string]bool{}
	for _, u := range upstream {
		up[u.Name] = true
		ls := vendored[u.Name]
		sel := p.Selected(u.Name, u.Category)
		switch {
		case ls != nil && sel:
			h, err := fsutil.HashTree(u.Dir)
			if err != nil {
				return nil, err
			}
			if h != ls.Hash || ls.Category != u.Category {
				st.Changed = append(st.Changed, u.Name)
			}
		case sel:
			st.Pending = append(st.Pending, u.Name)
		}
		if ls == nil && known != nil && !known[u.Name] {
			st.New = append(st.New, u.Name)
		}
	}
	for n, ls := range vendored {
		if !up[n] && p.Selected(n, ls.Category) && (known == nil || known[n]) {
			st.Gone = append(st.Gone, n)
		}
	}
	for _, l := range [][]string{st.Changed, st.New, st.Gone, st.Pending} {
		sort.Strings(l)
	}
	return st, nil
}

// OutdatedPath is the state file holding the last upstream checks, next
// to the user config (it is per user and per machine, not part of any
// library).
func OutdatedPath() string {
	return filepath.Join(filepath.Dir(library.ConfigPath()), "outdated.json")
}

// stateMu serialises read-modify-write of the state file within the
// process (the dashboard's check may end while an update does). Across
// processes the last writer wins; RecordedOutdated drops entries the lock
// has moved past, so a lost write only makes a pack unknown.
var stateMu sync.Mutex

// stateFile is OutdatedPath's content: one record per library root.
type stateFile struct {
	Version   int                  `json:"version"`
	Libraries map[string]*Outdated `json:"libraries"`
}

func readState() (*stateFile, error) {
	st := &stateFile{Version: 1, Libraries: map[string]*Outdated{}}
	b, err := os.ReadFile(OutdatedPath())
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, st); err != nil {
		return nil, fmt.Errorf("%s: %w", OutdatedPath(), err)
	}
	if st.Libraries == nil {
		st.Libraries = map[string]*Outdated{}
	}
	return st, nil
}

func writeState(st *stateFile) error {
	p := OutdatedPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteAtomic(p, append(b, '\n'))
}

// record merges a check into the state file: the packs checked replace
// their earlier entries, a pack that failed keeps its earlier entry and
// is listed as failed, packs no longer subscribed are dropped.
func (s *Session) record(o *Outdated) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	st, err := readState()
	if err != nil {
		return err
	}
	prev := st.Libraries[libKey(s.Lib.Root)]
	if prev == nil {
		prev = &Outdated{}
	}
	byRepo := map[string]PackStatus{}
	for _, p := range prev.Packs {
		byRepo[p.Repo] = p
	}
	failed := map[string]Failure{}
	for _, f := range prev.Failed {
		failed[f.Repo] = f
	}
	for _, p := range o.Packs {
		byRepo[p.Repo] = p
		delete(failed, p.Repo)
	}
	for _, f := range o.Failed {
		failed[f.Repo] = f
	}
	next := &Outdated{Checked: o.Checked, Packs: []PackStatus{}, Failed: []Failure{}}
	for _, p := range s.Lib.Manifest.Packs {
		if e, ok := byRepo[p.Repo]; ok {
			next.Packs = append(next.Packs, e)
		}
		if f, ok := failed[p.Repo]; ok {
			next.Failed = append(next.Failed, f)
		}
	}
	st.Libraries[libKey(s.Lib.Root)] = next
	return writeState(st)
}

// refreshRecorded marks the updated packs as up to date in the state
// file, so `list` and the menu stop counting them. It only rewrites a
// record that exists: a library never checked gets none.
func (s *Session) refreshRecorded(commits map[string]string) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	st, err := readState()
	if err != nil {
		return err
	}
	rec := st.Libraries[libKey(s.Lib.Root)]
	if rec == nil || len(commits) == 0 {
		return nil
	}
	now := time.Now()
	for _, repo := range sortedRepos(commits) {
		fresh := PackStatus{Repo: repo, Commit: commits[repo], Latest: commits[repo], Checked: now,
			Changed: []string{}, New: []string{}, Gone: []string{}, Pending: []string{}}
		if e := rec.Find(repo); e != nil {
			*e = fresh
		} else {
			rec.Packs = append(rec.Packs, fresh)
		}
		var failed []Failure
		for _, f := range rec.Failed {
			if f.Repo != repo {
				failed = append(failed, f)
			}
		}
		rec.Failed = failed
	}
	return writeState(st)
}

// RecordedOutdated returns the last recorded upstream check of this
// library, reconciled with the lock as it is now, without any network
// access; nil when nothing is recorded. A pack whose vendored commit
// changed since its check (updated elsewhere, or the lock was pulled from
// another machine) is left out: its state is unknown until the next check.
func (s *Session) RecordedOutdated() *Outdated {
	st, err := readState()
	if err != nil {
		return nil
	}
	rec := st.Libraries[libKey(s.Lib.Root)]
	if rec == nil {
		return nil
	}
	out := &Outdated{Packs: []PackStatus{}, Failed: []Failure{}}
	for _, p := range s.Lib.Manifest.Packs {
		for _, f := range rec.Failed {
			if f.Repo == p.Repo {
				out.Failed = append(out.Failed, f)
			}
		}
		e := rec.Find(p.Repo)
		if e == nil {
			continue
		}
		lp := s.Lib.Lock.Packs[p.Repo]
		commit := ""
		vendored := map[string]*lock.Skill{}
		if lp != nil {
			commit, vendored = lp.Commit, lp.Skills
		}
		if commit != e.Commit {
			continue
		}
		r := *e
		has := func(n string) bool { return vendored[n] != nil }
		r.Changed = filter(e.Changed, has)
		r.Gone = filter(e.Gone, has)
		r.New = filter(e.New, func(n string) bool { return !has(n) })
		r.Pending = filter(e.Pending, func(n string) bool { return !has(n) })
		out.Packs = append(out.Packs, r)
		if out.Checked.IsZero() || r.Checked.Before(out.Checked) {
			out.Checked = r.Checked
		}
	}
	if len(out.Packs) == 0 && len(out.Failed) == 0 {
		return nil
	}
	if out.Checked.IsZero() {
		out.Checked = rec.Checked
	}
	return out
}

// libKey is the state file's key for a library: its absolute path with
// symbolic links resolved, however it was located.
func libKey(root string) string {
	if a, err := filepath.Abs(root); err == nil {
		root = a
	}
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	return root
}

func sortedRepos(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func filter(in []string, keep func(string) bool) []string {
	out := []string{}
	for _, s := range in {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}

// Ago renders how long before now t was, briefly: "just now", "5m ago",
// "2h ago", "3d ago".
func Ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
