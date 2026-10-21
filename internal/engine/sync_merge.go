package engine

import (
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/safetext"
)

// mergeSkill merges upstream's new version of s into the user's changed
// copy at dest: base is what prev (the lock's entry) records as vendored,
// found in the snapshots or the library's history (categories are the
// ones it may have had there).
func mergeSkill(lib *library.Library, repo string, s pack.Skill, dest string, prev *lock.Skill, categories []string, commit string) (*MergeResult, error) {
	ours, err := ReadFileSet(dest)
	if err != nil {
		return nil, err
	}
	theirs, err := ReadFileSet(s.Dir)
	if err != nil {
		return nil, err
	}
	base := FindBase(lib, repo, s.Name, append([]string{prev.Category}, categories...), prev.Hash)
	return Merge(base, ours, theirs, commit)
}

// SetAsideMerged moves the skill at dest, as it is before a merge is
// written in its place, to the trash: the merge (regular files only, as
// a vendored copy holds) then goes into a new directory, and anything
// else the old one held (.git, node_modules, links) stays in the trash
// with it, as when an update replaces a copy. The trash note keeps the
// lock's entry before the merge, so that restoring it brings back what
// the merge started from. It returns where in the trash it is.
func SetAsideMerged(lib *library.Library, repo, dest, name string, lp *lock.Pack, prev *lock.Skill) (string, error) {
	to, err := Trash(lib, dest)
	if err != nil {
		return "", i18n.Errorf("moving %s to trash: %w", dest, err)
	}
	was := *prev
	note := &TrashNote{Reason: ReasonMerged, Skills: []string{name}, Repo: repo,
		Lock: &lock.Pack{Commit: lp.Commit, Ref: lp.Ref, Skills: map[string]*lock.Skill{name: &was}}}
	warnf(SetTrashNote(to, note))
	return to, nil
}

// filesChanged reports whether the files of dir (as HashTree sees them)
// are not the ones hash records.
func filesChanged(dir, hash string) bool {
	h, err := fsutil.HashTree(dir)
	return err != nil || h != hash
}

// LockPaths are the paths of conflicted files as the lock records them:
// one that could not be written there (control characters) is left out.
func LockPaths(paths []string) []string {
	var out []string
	for _, p := range paths {
		if !safetext.Has(p) {
			out = append(out, p)
		}
	}
	return out
}

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return append([]string{}, l...)
}
