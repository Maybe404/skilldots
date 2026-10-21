package app

import (
	"context"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/lock"
)

// The user's own changes to a vendored skill ("my edits"): what differs
// between the upstream version the skill is based on (the lock's hash,
// found as a merge finds its base: engine.FindBase) and its files as
// they are. A skill is edited when its files are not what skd vendored
// (fsutil.Pristine against the lock's hash).

// vendoredLock is the lock's entry of a vendored entry, or nil.
func (s *Session) vendoredLock(e *engine.Entry) *lock.Skill {
	if e.Kind != engine.KindVendor {
		return nil
	}
	lp := s.Lib.Lock.Packs[e.Origin]
	if lp == nil {
		return nil
	}
	return lp.Skills[e.Name]
}

// edited reports whether a vendored entry's files are not what skd
// vendored.
func (s *Session) edited(e *engine.Entry) bool {
	ls := s.vendoredLock(e)
	return ls != nil && !e.Missing && fsutil.Exists(e.Dir) && !fsutil.Pristine(e.Dir, ls.Hash)
}

// MarkEdited sets Edited on the vendored entries the user changed. It
// reads every file of every vendored skill, so it is done only where the
// mark is shown (lists), never in Resolve.
func (s *Session) MarkEdited(entries []engine.Entry) {
	for i := range entries {
		entries[i].Edited = s.edited(&entries[i])
	}
}

// LocalChanges is the diff of a vendored skill's files against the
// upstream version they are based on: the user's edits, file by file,
// as a SkillDiff (Status "changed", Local set, LocalFiles the files that
// differ). Base is false when that version is not found (no upstream
// snapshot, and never committed in the library): then nothing is
// compared and Files is empty.
func (s *Session) LocalChanges(ctx context.Context, r Ref) (*SkillDiff, error) {
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	e, err := s.mustEntry(entries, r)
	if err != nil {
		return nil, err
	}
	ls := s.vendoredLock(e)
	if ls == nil {
		return nil, i18n.Errorf("%q is not a vendored skill: its files are all yours", e.LinkName)
	}
	sd := &SkillDiff{Name: e.Name, Status: "changed", LocalFiles: []string{}, Files: []FileDiff{}}
	if !s.edited(e) {
		return sd, nil
	}
	sd.Local = true
	base := engine.FindBase(s.Lib, e.Origin, e.Name, []string{ls.Category}, ls.Hash)
	if base == nil {
		sd.LocalUnknown = true
		return sd, nil
	}
	ours, err := engine.ReadFileSet(e.Dir)
	if err != nil {
		return nil, err
	}
	sd.Base = true
	baseFiles, ourFiles := diffable(base), diffable(ours)
	sd.LocalFiles = changedPaths(baseFiles, ourFiles)
	if sd.LocalFiles == nil {
		sd.LocalFiles = []string{}
	}
	local := map[string]bool{}
	for _, f := range sd.LocalFiles {
		local[f] = true
	}
	sd.Files, sd.MoreFiles = diffFiles(baseFiles, ourFiles, local)
	return sd, nil
}
