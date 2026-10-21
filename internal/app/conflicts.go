package app

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/pack"
)

// An update merges upstream's changes into a vendored skill the user
// changed (engine.Sync). When both changed the same lines, or a file one
// side removed, the update of that skill is shelved: the skill is left
// as it is, and skills.lock records what it waits for. `skd resolve`
// finishes it the way a git merge is finished: it writes the merge with
// conflict markers into the skill, puts the conflicted files into the
// library's index as unmerged, and --continue takes the result once no
// marker is left (or --abort puts the skill back as it was).

// ShelvedInfo is a skill whose update waits for its conflicts to be
// resolved, as `skd resolve` lists it.
type ShelvedInfo struct {
	Repo     string `json:"repo"`
	Name     string `json:"name"`
	LinkName string `json:"linkName"`
	Dir      string `json:"dir"`
	// Upstream is the upstream commit it waits for.
	Upstream  string   `json:"upstream"`
	Resolving bool     `json:"resolving"`
	Files     []string `json:"files"`
}

// Shelved lists the shelved skills, by pack and name.
func (s *Session) Shelved() ([]ShelvedInfo, error) {
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	out := []ShelvedInfo{}
	for _, e := range entries {
		if e.Kind != engine.KindVendor || (e.Shelved == "" && !e.Resolving) {
			continue
		}
		out = append(out, ShelvedInfo{Repo: e.Origin, Name: e.Name, LinkName: e.LinkName, Dir: e.Dir,
			Upstream: e.Shelved, Resolving: e.Resolving, Files: nonNilStrings(e.Conflicts)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// shelvedEntry finds the shelved skill r names, with its lock entry.
// With r empty it is the one skill being resolved (resolving set) or
// shelved; more than one is an error naming them.
func (s *Session) shelvedEntry(r Ref, resolving bool) (*engine.Entry, *lock.Pack, *lock.Skill, error) {
	entries, err := s.Resolve()
	if err != nil {
		return nil, nil, nil, err
	}
	var e *engine.Entry
	if r.Key == "" && r.Kind == "" {
		var names []string
		for i := range entries {
			if entries[i].Kind == engine.KindVendor && (entries[i].Resolving || !resolving && entries[i].Shelved != "") {
				e = &entries[i]
				names = append(names, entries[i].LinkName)
			}
		}
		switch {
		case len(names) == 0 && resolving:
			return nil, nil, nil, i18n.New("no skill is being resolved (skd resolve lists the shelved ones)")
		case len(names) == 0:
			return nil, nil, nil, i18n.New("no skill is shelved")
		case len(names) > 1:
			return nil, nil, nil, i18n.Errorf("%d skills are being resolved (%s); name one", len(names), strings.Join(names, ", "))
		}
	} else if e, err = s.mustEntry(entries, r); err != nil {
		return nil, nil, nil, err
	}
	if e.Kind != engine.KindVendor {
		return nil, nil, nil, i18n.Errorf("%q is not a vendored skill; only updates of vendored skills are shelved", e.LinkName)
	}
	lp := s.Lib.Lock.Packs[e.Origin]
	if lp == nil || lp.Skills[e.Name] == nil {
		return nil, nil, nil, i18n.Errorf("%s is not in %s", e.Name, lock.FileName)
	}
	ls := lp.Skills[e.Name]
	if e.LinkedVia != "" {
		return nil, nil, nil, i18n.Errorf("%q: its directory goes through %s, a symbolic link inside the library; skd writes nothing through it", e.LinkName, s.Lib.Rel(e.LinkedVia))
	}
	return e, lp, ls, nil
}

// upstreamFiles is upstream's version of a shelved skill (the lock's
// Upstream hash): from the library's snapshots, else from the cache
// clone of the pack when it still has that version.
func (s *Session) upstreamFiles(e *engine.Entry, ls *lock.Skill) (engine.FileSet, error) {
	if f := engine.FindBase(s.Lib, e.Origin, e.Name, []string{ls.Category}, ls.Upstream); f != nil {
		return f, nil
	}
	defer pack.Hold(e.Origin)()
	clone := pack.CachePath(e.Origin)
	if fsutil.Exists(filepath.Join(clone, ".git")) {
		if up, _, err := pack.Scan(clone); err == nil {
			if sk := pack.Find(up, e.Name); sk != nil {
				if f, err := engine.ReadFileSet(sk.Dir); err == nil && f.Hash() == ls.Upstream {
					return f, nil
				}
			}
		}
	}
	return nil, i18n.Errorf("upstream's version of %s is not at hand any more; run skd update %s, then skd resolve %s", e.LinkName, e.Origin, e.LinkName)
}

// ResolveOutcome is the result of ResolveStart; its JSON form is what
// `skd resolve <skill> --json` prints.
type ResolveOutcome struct {
	Repo string `json:"repo"`
	Name string `json:"name"`
	Dir  string `json:"dir"`
	// Merged is set when the merge had no conflicts after all (the local
	// copy changed since the update): it is done, nothing to resolve.
	Merged    bool              `json:"merged"`
	Conflicts []engine.Conflict `json:"conflicts"`
	// Backup is the trash ID of the skill as it was before.
	Backup string        `json:"backup,omitempty"`
	Apply  *ApplyOutcome `json:"apply,omitempty"`
}

// ResolveStart starts resolving a shelved skill: the merge of the local
// copy and upstream's version is worked out again and written into the
// skill's directory, conflicts marked as git marks them, after the
// directory as it was went to the trash. The conflicted files are put
// into the library's index as unmerged. A skill already being resolved
// is reported as it is.
func (s *Session) ResolveStart(r Ref) (*ResolveOutcome, error) {
	e, lp, ls, err := s.shelvedEntry(r, false)
	if err != nil {
		return nil, err
	}
	out := &ResolveOutcome{Repo: e.Origin, Name: e.Name, Dir: e.Dir, Conflicts: []engine.Conflict{}}
	if ls.Resolving {
		out.Backup = ls.Backup
		out.Conflicts = conflictKinds(e.Dir, ls.Conflicts)
		return out, nil
	}
	if ls.Shelved == "" {
		return nil, i18n.Errorf("%s has no update waiting: nothing to resolve", e.LinkName)
	}
	if err := s.Lib.Writable(); err != nil {
		return nil, err
	}
	if err := s.writableInLibrary(e.Dir); err != nil {
		return nil, err
	}
	theirs, err := s.upstreamFiles(e, ls)
	if err != nil {
		return nil, err
	}
	ours, err := engine.ReadFileSet(e.Dir)
	if err != nil {
		return nil, err
	}
	base := engine.FindBase(s.Lib, e.Origin, e.Name, []string{ls.Category}, ls.Hash)
	m, err := engine.Merge(base, ours, theirs, ls.Shelved)
	if err != nil {
		return nil, err
	}
	to, err := engine.SetAsideMerged(s.Lib, e.Origin, e.Dir, e.Name, lp, ls)
	if err != nil {
		return nil, err
	}
	out.Backup = engine.TrashID(s.Lib, to)
	if m.Clean() {
		if err := engine.WriteFileSet(e.Dir, m.Files, nil); err != nil {
			return nil, err
		}
		lp.Skills[e.Name] = &lock.Skill{Category: ls.Category, Hash: ls.Upstream}
		out.Merged = true
		out.Apply, err = s.saveAndApply()
		s.recordMerged(e.Origin, e.Name)
		return out, err
	}
	if err := engine.WriteFileSet(e.Dir, m.Files, m.Sidecars); err != nil {
		return nil, err
	}
	if err := engine.StageConflicts(s.Lib, e.Dir, m.Conflicts, base, ours, theirs); err != nil {
		s.logf(Warn, "could not mark the conflicts in the library's git index: %v", err)
	}
	ls.Resolving, ls.Backup, ls.Conflicts = true, out.Backup, engine.LockPaths(m.ConflictPaths())
	out.Conflicts = m.Conflicts
	return out, s.Lib.Save()
}

// conflictKinds tells the kind of each conflicted file of a skill being
// resolved from what is in its directory (the sidecars skd wrote).
func conflictKinds(dir string, paths []string) []engine.Conflict {
	out := []engine.Conflict{}
	for _, p := range paths {
		f := filepath.Join(dir, filepath.FromSlash(p))
		kind := engine.ConflictContent
		switch {
		case fsutil.Lexists(f + engine.DeletedSuffix):
			kind = engine.ConflictDeletedUpstream
		case fsutil.Lexists(f+engine.UpstreamSuffix) && !fsutil.Lexists(f):
			kind = engine.ConflictDeletedLocal
		case fsutil.Lexists(f + engine.UpstreamSuffix):
			kind = engine.ConflictBinary
		}
		out = append(out, engine.Conflict{Path: p, Kind: kind})
	}
	return out
}

// UnresolvedError is ResolveContinue refusing: Files still hold conflict
// markers, or are sidecars skd wrote that are still there.
type UnresolvedError struct {
	Skill string
	Files []string
}

func (e *UnresolvedError) Error() string {
	return i18n.Sprintf("%s still has conflicts in %s: edit them (or git checkout --ours/--theirs <file>), remove the .upstream files, then run skd resolve --continue %s again",
		e.Skill, strings.Join(e.Files, ", "), e.Skill)
}

// ContinueOutcome is the result of ResolveContinue.
type ContinueOutcome struct {
	Repo  string        `json:"repo"`
	Name  string        `json:"name"`
	Apply *ApplyOutcome `json:"apply"`
}

// ResolveContinue finishes resolving a skill (r empty: the one being
// resolved): it refuses while a file of the skill has conflict markers
// or a sidecar is left (UnresolvedError); otherwise the skill's
// directory is added to the library's index (git add), the lock records
// upstream's version as merged, the skill is linked again and the
// history records the merge.
func (s *Session) ResolveContinue(r Ref) (*ContinueOutcome, error) {
	e, lp, ls, err := s.shelvedEntry(r, true)
	if err != nil {
		return nil, err
	}
	if !ls.Resolving {
		return nil, i18n.Errorf("%s is not being resolved: skd resolve %s starts it", e.LinkName, e.LinkName)
	}
	left, err := engine.Unresolved(e.Dir, ls.Conflicts)
	if err != nil {
		return nil, err
	}
	if len(left) > 0 {
		return nil, &UnresolvedError{Skill: e.LinkName, Files: left}
	}
	if err := s.Lib.Writable(); err != nil {
		return nil, err
	}
	if engine.KeepsSnapshots(s.Lib) {
		if _, err := s.Lib.Git("add", "-A", "--", s.Lib.Rel(e.Dir)); err != nil {
			return nil, err
		}
		engine.ForgetStagedConflicts(s.Lib, e.Dir)
	}
	lp.Skills[e.Name] = &lock.Skill{Category: ls.Category, Hash: ls.Upstream}
	out := &ContinueOutcome{Repo: e.Origin, Name: e.Name}
	out.Apply, err = s.saveAndApply()
	s.recordMerged(e.Origin, e.Name)
	return out, err
}

// recordMerged adds a history record saying name of repo was merged
// with upstream (a resolve finished).
func (s *Session) recordMerged(repo, name string) {
	commit := ""
	if lp := s.Lib.Lock.Packs[repo]; lp != nil {
		commit = lp.Commit
	}
	r := &UpdateRecord{Time: time.Now().UTC().Truncate(time.Second), Repo: repo, From: commit, To: commit,
		Changed: []string{}, Added: []string{}, Moved: []string{}, Gone: []string{}, Dropped: []string{}, New: []string{},
		Merged: []string{name}, Commits: []UpstreamCommit{}}
	r.sanitize()
	if err := s.appendHistory([]*UpdateRecord{r}); err != nil {
		s.logf(Warn, "could not write the update history (%s): %v", HistoryFile, err)
	}
}

// AbortOutcome is the result of ResolveAbort.
type AbortOutcome struct {
	Repo string `json:"repo"`
	Name string `json:"name"`
	// Trash is the trash ID of the skill's files as --abort found them.
	Trash string        `json:"trash"`
	Apply *ApplyOutcome `json:"apply"`
}

// ResolveAbort gives up resolving a skill (r empty: the one being
// resolved): its files as they are go to the trash, the copy from before
// the resolve comes back from it, the conflicted files leave the index
// (what was staged before is put back), and the update stays shelved.
func (s *Session) ResolveAbort(r Ref) (*AbortOutcome, error) {
	e, _, ls, err := s.shelvedEntry(r, true)
	if err != nil {
		return nil, err
	}
	if !ls.Resolving {
		return nil, i18n.Errorf("%s is not being resolved: nothing to abort", e.LinkName)
	}
	if err := s.Lib.Writable(); err != nil {
		return nil, err
	}
	backup, err := engine.TrashEntryPath(s.Lib, ls.Backup)
	if err != nil {
		return nil, i18n.Errorf("the copy of %s from before the resolve is not in the trash any more (%v): finish the resolve instead (skd resolve --continue %s)", e.LinkName, err, e.LinkName)
	}
	out := &AbortOutcome{Repo: e.Origin, Name: e.Name}
	if fsutil.Lexists(e.Dir) {
		to, err := engine.Trash(s.Lib, e.Dir)
		if err != nil {
			return nil, err
		}
		s.noteWarn(engine.SetTrashNote(to, &engine.TrashNote{Reason: engine.ReasonResolveAborted, Skills: []string{e.Name}, Repo: e.Origin}))
		out.Trash = engine.TrashID(s.Lib, to)
	}
	if _, err := engine.Restore(s.Lib, s.Reg, ls.Backup); err != nil {
		if out.Trash != "" {
			if _, rerr := engine.Restore(s.Lib, s.Reg, out.Trash); rerr == nil {
				out.Trash = ""
			}
		}
		return nil, err
	}
	s.noteWarn(engine.DropTrashNote(backup))
	if err := engine.UnstageConflicts(s.Lib, e.Dir, ls.Conflicts); err != nil {
		s.logf(Warn, "could not take the conflicts out of the library's git index: %v", err)
	}
	ls.Resolving, ls.Backup = false, ""
	out.Apply, err = s.saveAndApply()
	return out, err
}

// ConflictBlock is one conflict in a file: the lines of each side.
type ConflictBlock struct {
	Index    int    `json:"index"`
	Local    string `json:"local"`
	Upstream string `json:"upstream"`
	Base     string `json:"base,omitempty"`
}

// ConflictFile is a conflicted file of a skill being resolved, read from
// its directory.
type ConflictFile struct {
	Path string `json:"path"`
	// Kind is the engine.Conflict kind.
	Kind   string `json:"kind"`
	Binary bool   `json:"binary"`
	// Blocks are the conflicts left in the file. A file removed on one
	// side has one block: the whole local file (or "" when removed here)
	// and the whole upstream file (or "" when removed upstream).
	Blocks []ConflictBlock `json:"blocks"`
	// Text is the file as it is (with its markers); "" for a binary file
	// or one removed here.
	Text string `json:"text"`
	// Resolved is set when nothing of the conflict is left.
	Resolved bool `json:"resolved"`
}

// ConflictFiles reads the conflicted files of a skill being resolved.
func (s *Session) ConflictFiles(r Ref) ([]ConflictFile, error) {
	e, _, ls, err := s.shelvedEntry(r, true)
	if err != nil {
		return nil, err
	}
	if !ls.Resolving {
		return nil, i18n.Errorf("%s is not being resolved: skd resolve %s starts it", e.LinkName, e.LinkName)
	}
	return readConflicts(e.Dir, ls.Conflicts)
}

func readConflicts(dir string, paths []string) ([]ConflictFile, error) {
	out := []ConflictFile{}
	for _, c := range conflictKinds(dir, paths) {
		f := filepath.Join(dir, filepath.FromSlash(c.Path))
		cf := ConflictFile{Path: c.Path, Kind: c.Kind, Blocks: []ConflictBlock{}}
		local, lerr := os.ReadFile(f)
		switch c.Kind {
		case engine.ConflictBinary:
			cf.Binary = true
			cf.Resolved = !fsutil.Lexists(f + engine.UpstreamSuffix)
		case engine.ConflictDeletedLocal, engine.ConflictDeletedUpstream:
			side := f + engine.UpstreamSuffix
			if c.Kind == engine.ConflictDeletedUpstream {
				side = f + engine.DeletedSuffix
			}
			up, uerr := os.ReadFile(side)
			cf.Resolved = uerr != nil
			cf.Binary = !isText(local) || !isText(up)
			cf.Text = string(local)
			if !cf.Resolved && !cf.Binary {
				cf.Blocks = append(cf.Blocks, ConflictBlock{Index: 0, Local: string(local), Upstream: string(up)})
			}
		default:
			if lerr != nil {
				if os.IsNotExist(lerr) {
					cf.Resolved = true
					break
				}
				return nil, lerr
			}
			if !isText(local) {
				cf.Binary, cf.Resolved = true, !engine.HasMarkers(local, true)
				break
			}
			cf.Text = string(local)
			for _, seg := range parseConflicts(string(local)) {
				if seg.block != nil {
					cf.Blocks = append(cf.Blocks, *seg.block)
				}
			}
			cf.Resolved = len(cf.Blocks) == 0 && !engine.HasMarkers(local, true)
		}
		out = append(out, cf)
	}
	return out, nil
}

func isText(b []byte) bool { return !isBinary(b) }

// segment is a piece of a conflicted file: text as it is, or a block.
type segment struct {
	text  string
	block *ConflictBlock
	// raw is a block's text as it is in the file, markers and all.
	raw string
}

// parseConflicts splits text into plain runs and conflict blocks
// (<<<<<<< … [||||||| …] ======= … >>>>>>>). A block that does not
// close is plain text.
func parseConflicts(text string) []segment {
	lines := strings.SplitAfter(text, "\n")
	var out []segment
	var plain strings.Builder
	flush := func() {
		if plain.Len() > 0 {
			out = append(out, segment{text: plain.String()})
			plain.Reset()
		}
	}
	is := func(l string, c byte) bool {
		l = strings.TrimRight(l, "\r\n")
		return len(l) >= 7 && strings.Count(l[:7], string(c)) == 7 && (len(l) == 7 || l[7] == ' ')
	}
	n := 0
	for i := 0; i < len(lines); i++ {
		if !is(lines[i], '<') {
			plain.WriteString(lines[i])
			continue
		}
		// Find the rest of the block.
		var local, base, up strings.Builder
		part, j, closed := &local, i+1, false
		hasBase := false
		for ; j < len(lines); j++ {
			switch {
			case is(lines[j], '|') && part == &local:
				part, hasBase = &base, true
				continue
			case is(lines[j], '=') && part != &up:
				part = &up
				continue
			case is(lines[j], '>') && part == &up:
				closed = true
			case is(lines[j], '<'):
				j = len(lines) // a block opening inside another: not one
			}
			if closed || j == len(lines) {
				break
			}
			part.WriteString(lines[j])
		}
		if !closed {
			plain.WriteString(lines[i])
			continue
		}
		flush()
		b := &ConflictBlock{Index: n, Local: local.String(), Upstream: up.String()}
		if hasBase {
			b.Base = base.String()
		}
		out = append(out, segment{block: b, raw: strings.Join(lines[i:j+1], "")})
		n++
		i = j
	}
	flush()
	return out
}

// The choices ResolvePick takes.
const (
	PickLocal    = "local"
	PickUpstream = "upstream"
	PickBoth     = "both"
)

// ResolvePick settles one conflict of a skill being resolved by taking
// one side (or both, local first): block is the conflict's index in the
// file (ConflictFile.Blocks). A file removed on one side, or a binary
// one, is settled whole: local keeps the local file (or its absence),
// upstream takes upstream's (or removes it); the sidecar goes.
func (s *Session) ResolvePick(r Ref, path string, block int, choice string) error {
	e, _, ls, err := s.shelvedEntry(r, true)
	if err != nil {
		return err
	}
	if !ls.Resolving {
		return i18n.Errorf("%s is not being resolved: skd resolve %s starts it", e.LinkName, e.LinkName)
	}
	if choice != PickLocal && choice != PickUpstream && choice != PickBoth {
		return i18n.Errorf("choose %s, %s or %s, not %q", PickLocal, PickUpstream, PickBoth, choice)
	}
	if !slices.Contains(ls.Conflicts, path) {
		return i18n.Errorf("%s has no conflict in %q", e.LinkName, path)
	}
	f := filepath.Join(e.Dir, filepath.FromSlash(path))
	if !fsutil.Within(e.Dir, f) || fsutil.LinkOnPath(e.Dir, f) != "" {
		return i18n.Errorf("%q is not a file of %s", path, e.LinkName)
	}
	kind := conflictKinds(e.Dir, []string{path})[0].Kind
	if kind != engine.ConflictContent {
		if choice == PickBoth {
			return i18n.Errorf("%s: a file removed on one side, or not text, is taken from one side whole", path)
		}
		return pickWhole(f, kind, choice)
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return err
	}
	st, err := os.Stat(f)
	if err != nil {
		return err
	}
	var text strings.Builder
	found := false
	for _, seg := range parseConflicts(string(b)) {
		switch {
		case seg.block == nil:
			text.WriteString(seg.text)
		case seg.block.Index != block:
			text.WriteString(seg.raw)
		default:
			found = true
			if choice != PickUpstream {
				text.WriteString(seg.block.Local)
			}
			if choice != PickLocal {
				text.WriteString(seg.block.Upstream)
			}
		}
	}
	if !found {
		return i18n.Errorf("%s has no conflict %d", path, block)
	}
	if err := fsutil.WriteAtomic(f, []byte(text.String())); err != nil {
		return err
	}
	return os.Chmod(f, st.Mode().Perm())
}

// pickWhole settles a conflict that has no markers by taking one side.
func pickWhole(f, kind, choice string) error {
	up, gone := f+engine.UpstreamSuffix, f+engine.DeletedSuffix
	switch {
	case choice == PickLocal:
		// The local file, or its absence, stays.
	case kind == engine.ConflictDeletedUpstream:
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return err
		}
	default:
		if fsutil.Lexists(f) {
			if err := os.Remove(f); err != nil {
				return err
			}
		}
		if err := os.Rename(up, f); err != nil {
			return err
		}
	}
	for _, sc := range []string{up, gone} {
		if err := os.Remove(sc); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
