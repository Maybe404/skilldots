package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
)

// Syncing the library with its remote (skd sync) is a git merge: what the
// remote has that the library has not is merged in before pushing. When
// both changed the same lines the sync is shelved: the merge is aborted
// and the library stays as it was. skd sync --resolve redoes the merge
// and leaves git's conflict state in the library (markers, unmerged
// entries) for resolving; --continue commits the merge and pushes, and
// --abort gives up (git merge --abort). Paths are relative to the
// library's root.

// SyncRef is the key the desktop app resolves a sync's conflicts by,
// with the same calls as a skill's (resolveStart, resolvePick, …).
const SyncRef = "@sync"

// Syncing reports whether a sync's merge is being resolved (a merge is
// under way in the library: MERGE_HEAD).
func (s *Session) Syncing() bool {
	_, err := s.Lib.Git("rev-parse", "-q", "--verify", "MERGE_HEAD")
	return err == nil
}

// unmerged lists the paths the index has unmerged.
func (s *Session) unmerged() []string {
	out, err := s.Lib.Git("diff", "--name-only", "-z", "--diff-filter=U")
	if err != nil {
		return nil
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	return paths
}

// syncConflictsPath keeps the files a sync being resolved conflicted on.
func (s *Session) syncConflictsPath() (string, error) {
	p, err := s.Lib.Git("rev-parse", "--git-path", "skilldots/sync-conflicts")
	if err != nil {
		return "", err
	}
	return s.Lib.Abs(p), nil
}

// syncShelvedPath keeps the files of a shelved sync, so that it is still
// known after the app or the terminal is closed: in the repository's git
// directory, this machine's.
func (s *Session) syncShelvedPath() (string, error) {
	p, err := s.Lib.Git("rev-parse", "--git-path", "skilldots/sync-shelved")
	if err != nil {
		return "", err
	}
	return s.Lib.Abs(p), nil
}

// setSyncShelved records a shelved sync's files (none: no sync is
// shelved).
func (s *Session) setSyncShelved(files []string) {
	p, err := s.syncShelvedPath()
	if err != nil {
		return
	}
	if len(files) == 0 {
		os.Remove(p)
		return
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(strings.Join(files, "\x00")), 0o644); err != nil {
		s.logf(Warn, "could not record the shelved sync: %v", err)
	}
}

// SyncShelved lists the files of the last sync, shelved because they
// conflict, until a sync goes through; empty while one is being resolved
// (Syncing says so then) or when none is shelved.
func (s *Session) SyncShelved() []string {
	out := []string{}
	if s.Syncing() {
		return out
	}
	p, err := s.syncShelvedPath()
	if err != nil {
		return out
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return out
	}
	for _, l := range strings.Split(string(b), "\x00") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// SyncConflictPaths are the files a sync being resolved conflicts on
// (none when no sync is).
func (s *Session) SyncConflictPaths() []string {
	if !s.Syncing() {
		return nil
	}
	return s.syncConflicts()
}

func (s *Session) syncConflicts() []string {
	p, err := s.syncConflictsPath()
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return s.unmerged()
	}
	var out []string
	for _, l := range strings.Split(string(b), "\x00") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// pull merges the remote's commits into the library (after fetching).
// With keep, a merge that conflicts is left for resolving (and its
// files recorded); without, it is aborted. It returns the conflicting
// files. A merge that brought something in is linked (out.Apply).
func (s *Session) pull(out *BackupOutcome, keep bool) ([]string, error) {
	lib := s.Lib
	if _, err := lib.Git("fetch", "-q", "origin"); err != nil {
		return nil, err
	}
	if n, err := lib.Git("rev-list", "--count", "HEAD..@{u}"); err != nil || n == "0" {
		return nil, err
	}
	// Merged by its name (origin/main), which git writes into the conflict
	// markers: "@{u}" there would tell nobody which side is the remote.
	theirs := "@{u}"
	if up, err := lib.Git("rev-parse", "--abbrev-ref", "@{u}"); err == nil && up != "" {
		theirs = up
	}
	if _, err := lib.Git("merge", "--no-edit", "-q", theirs); err != nil {
		conflicts := s.unmerged()
		if len(conflicts) == 0 || !keep {
			if s.Syncing() {
				lib.Git("merge", "--abort")
			}
			if len(conflicts) == 0 {
				return nil, err
			}
			return conflicts, nil
		}
		p, perr := s.syncConflictsPath()
		if perr == nil {
			os.MkdirAll(filepath.Dir(p), 0o755)
			perr = os.WriteFile(p, []byte(strings.Join(conflicts, "\x00")), 0o644)
		}
		if perr != nil {
			s.logf(Warn, "could not record the files of the sync being resolved: %v", perr)
		}
		return conflicts, nil
	}
	out.Pulled = true
	out.Apply = s.applyPulled()
	return nil, nil
}

// applyPulled links what a merge brought in: the library is opened again
// (skills.toml and skills.lock may have changed) and applied.
func (s *Session) applyPulled() *ApplyOutcome {
	fresh, err := Open(s.Lib.Root)
	if err != nil {
		s.logf(Warn, "could not link what was pulled: %v; run skd apply", err)
		return nil
	}
	fresh.Log = s.Log
	s.Lib, s.Reg = fresh.Lib, fresh.Reg
	ap, err := s.Apply(ApplyOptions{})
	if err != nil {
		s.logf(Warn, "could not link what was pulled: %v; run skd apply", err)
	}
	return ap
}

// SyncResolveOutcome is the result of SyncResolve.
type SyncResolveOutcome struct {
	// Files are the conflicts to resolve; none when the merge went
	// through after all (the sync then finished: Sync).
	Files []ConflictFile `json:"files"`
	Sync  *BackupOutcome `json:"sync,omitempty"`
}

// SyncResolve redoes a shelved sync's merge and leaves it for resolving:
// the conflicting files hold git's markers and are unmerged in the
// index, as after a git merge. Agents linked to skills among them read
// the markers until the sync is finished or aborted. A sync already
// being resolved is reported as it is.
func (s *Session) SyncResolve() (*SyncResolveOutcome, error) {
	if s.Syncing() {
		files, err := s.SyncConflictFiles()
		return &SyncResolveOutcome{Files: files}, err
	}
	lib := s.Lib
	if _, err := lib.Git("rev-parse", "--abbrev-ref", "@{u}"); err != nil {
		return nil, i18n.New("the library has no remote branch to sync with (skd sync --remote <git-url> sets one)")
	}
	if _, err := s.Commit(DefaultBackupMessage, false); err != nil {
		return nil, err
	}
	out := &BackupOutcome{Committed: true, Conflicts: []string{}}
	conflicts, err := s.pull(out, true)
	if err != nil {
		return nil, err
	}
	if len(conflicts) == 0 {
		// Nothing conflicts (any more): the sync goes through.
		if _, err := lib.Git("push", "-q"); err != nil {
			return nil, err
		}
		out.Pushed = true
		s.setSyncShelved(nil)
		s.pushSnapshots()
		return &SyncResolveOutcome{Files: []ConflictFile{}, Sync: out}, nil
	}
	files, err := s.SyncConflictFiles()
	return &SyncResolveOutcome{Files: files}, err
}

// stages are the index stages (1 base, 2 local, 3 remote) path has.
func (s *Session) stages(path string) map[string]bool {
	out, _ := s.Lib.Git("ls-files", "-u", "--", ":(literal)"+path)
	st := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(strings.SplitN(l, "\t", 2)[0]); len(f) == 3 {
			st[f[2]] = true
		}
	}
	return st
}

// SyncConflictFiles reads the files a sync being resolved conflicts on,
// as they are now.
func (s *Session) SyncConflictFiles() ([]ConflictFile, error) {
	if !s.Syncing() {
		return nil, i18n.New("no sync is being resolved")
	}
	out := []ConflictFile{}
	for _, p := range s.syncConflicts() {
		cf := ConflictFile{Path: p, Kind: engine.ConflictContent, Blocks: []ConflictBlock{}}
		st := s.stages(p)
		b, rerr := os.ReadFile(s.Lib.Abs(p))
		switch {
		case rerr == nil && isText(b) && engine.HasMarkers(b, true):
			cf.Text = string(b)
			for _, seg := range parseConflicts(string(b)) {
				if seg.block != nil {
					cf.Blocks = append(cf.Blocks, *seg.block)
				}
			}
		case len(st) == 0:
			cf.Resolved = true
			cf.Text = string(b)
		case st["2"] && st["3"] && (rerr != nil || !isText(b)):
			cf.Kind, cf.Binary = engine.ConflictBinary, true
		case st["2"] && st["3"]:
			// Its markers are gone: resolved, not added yet.
			cf.Resolved, cf.Text = true, string(b)
		default:
			cf.Kind = engine.ConflictDeletedLocal
			if !st["3"] {
				cf.Kind = engine.ConflictDeletedUpstream
			}
			local, remote := s.showStage("2", p), s.showStage("3", p)
			cf.Text = string(b)
			if isText([]byte(local)) && isText([]byte(remote)) {
				cf.Blocks = append(cf.Blocks, ConflictBlock{Index: 0, Local: local, Upstream: remote})
			} else {
				cf.Binary = true
			}
		}
		out = append(out, cf)
	}
	return out, nil
}

// SyncPick settles one conflict of a sync being resolved, as ResolvePick
// does a skill's: block in a file with markers; a file removed on one
// side or not text whole (local keeps this library's, upstream takes the
// remote's). A file left without markers is added (git add).
func (s *Session) SyncPick(path string, block int, choice string) error {
	if !s.Syncing() {
		return i18n.New("no sync is being resolved")
	}
	if choice != PickLocal && choice != PickUpstream && choice != PickBoth {
		return i18n.Errorf("choose %s, %s or %s, not %q", PickLocal, PickUpstream, PickBoth, choice)
	}
	if !slices.Contains(s.syncConflicts(), path) {
		return i18n.Errorf("the sync has no conflict in %q", path)
	}
	f := s.Lib.Abs(path)
	if !fsutil.Within(s.Lib.Root, f) || fsutil.LinkOnPath(s.Lib.Root, f) != "" {
		return i18n.Errorf("%q is not a file of the library", path)
	}
	lit := ":(literal)" + path
	if b, err := os.ReadFile(f); err == nil && isText(b) && engine.HasMarkers(b, true) {
		if err := pickBlock(f, path, block, choice); err != nil {
			return err
		}
		if b, err := os.ReadFile(f); err == nil && !engine.HasMarkers(b, true) {
			_, err := s.Lib.Git("add", "--", lit)
			return err
		}
		return nil
	}
	if choice == PickBoth {
		return i18n.Errorf("%s: a file removed on one side, or not text, is taken from one side whole", path)
	}
	stage, side := "2", "--ours"
	if choice == PickUpstream {
		stage, side = "3", "--theirs"
	}
	if !s.stages(path)[stage] {
		_, err := s.Lib.Git("rm", "-q", "--", lit)
		return err
	}
	if _, err := s.Lib.Git("checkout", side, "--", lit); err != nil {
		return err
	}
	_, err := s.Lib.Git("add", "--", lit)
	return err
}

// SyncUnresolvedError is SyncContinue refusing: Files still conflict.
type SyncUnresolvedError struct{ Files []string }

func (e *SyncUnresolvedError) Error() string {
	return i18n.Sprintf("the sync still has conflicts in %s: edit them (or git checkout --ours/--theirs <file>, git add <file>), then run skd sync --continue again",
		strings.Join(e.Files, ", "))
}

// SyncContinue finishes a sync being resolved: it refuses while a
// conflicted file holds markers or stays unmerged (SyncUnresolvedError);
// otherwise the files are added, the merge is committed, what it brought
// in is linked, and the library is pushed.
func (s *Session) SyncContinue() (*BackupOutcome, error) {
	if !s.Syncing() {
		return nil, i18n.New("no sync is being resolved")
	}
	lib := s.Lib
	var left []string
	for _, p := range s.syncConflicts() {
		if b, err := os.ReadFile(lib.Abs(p)); err == nil && isText(b) && engine.HasMarkers(b, true) {
			left = append(left, p)
			continue
		}
		st := s.stages(p)
		if len(st) > 0 && st["2"] && st["3"] {
			if b, err := os.ReadFile(lib.Abs(p)); err == nil && isText(b) {
				if _, err := lib.Git("add", "--", ":(literal)"+p); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, p := range s.unmerged() {
		if !slices.Contains(left, p) {
			left = append(left, p)
		}
	}
	if len(left) > 0 {
		return nil, &SyncUnresolvedError{Files: left}
	}
	if _, err := lib.Git("commit", "-q", "--no-edit"); err != nil {
		return nil, err
	}
	if p, err := s.syncConflictsPath(); err == nil {
		os.Remove(p)
	}
	out := &BackupOutcome{Committed: true, Pulled: true, Conflicts: []string{}}
	out.Remote, _ = lib.Git("remote", "get-url", "origin")
	out.Apply = s.applyPulled()
	if _, err := s.Lib.Git("push", "-q"); err != nil {
		return out, err
	}
	out.Pushed = true
	s.setSyncShelved(nil)
	s.pushSnapshots()
	s.notePrivate(out)
	return out, nil
}

// showStage is the content of path at an index stage, as it is ("" when
// there is none).
func (s *Session) showStage(stage, path string) string {
	cmd := exec.Command("git", "show", ":"+stage+":"+path)
	cmd.Dir = s.Lib.Root
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(b)
}

// SyncAbort gives up a sync being resolved (git merge --abort): the
// library is as it was before; the sync stays shelved.
func (s *Session) SyncAbort() error {
	if !s.Syncing() {
		return i18n.New("no sync is being resolved")
	}
	files := s.syncConflicts()
	if _, err := s.Lib.Git("merge", "--abort"); err != nil {
		return err
	}
	if p, err := s.syncConflictsPath(); err == nil {
		os.Remove(p)
	}
	// Shelved again, until a sync goes through.
	s.setSyncShelved(files)
	return nil
}
