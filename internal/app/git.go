package app

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/safetext"
)

// commitPaths is what a library commit takes: everything under the root
// except the temporary files of an interrupted save, which a .gitignore
// written before they were ignored would let through.
var commitPaths = []string{".", ":(exclude,glob).skills.toml.tmp-*", ":(exclude,glob).skills.lock.tmp-*"}

// Commit commits the library's own files and, with push, pushes when the
// branch has an upstream (and the upstream snapshots with it). It
// reports whether a commit was made. It refuses while a skill is being
// resolved: its files hold conflict markers.
func (s *Session) Commit(msg string, push bool) (bool, error) {
	lib := s.Lib
	if names := s.resolving(); len(names) > 0 {
		return false, i18n.Errorf("not committing while %s is being resolved (its files hold conflict markers): finish with skd resolve --continue, or skd resolve --abort", strings.Join(names, ", "))
	}
	if _, err := lib.Git(append([]string{"add", "-A", "--"}, commitPaths...)...); err != nil {
		return false, err
	}
	if _, err := lib.Git(append([]string{"diff", "--cached", "--quiet", "--"}, commitPaths...)...); err == nil {
		return false, nil
	}
	if _, err := lib.Git(append([]string{"commit", "-q", "-m", msg, "--"}, commitPaths...)...); err != nil {
		return false, err
	}
	if _, err := lib.Git("rev-parse", "--abbrev-ref", "@{u}"); push && err == nil {
		if _, err := lib.Git("push", "-q"); err != nil {
			return true, err
		}
		s.pushSnapshots()
	}
	return true, nil
}

// resolving names the skills being resolved (skd resolve).
func (s *Session) resolving() []string {
	var out []string
	for _, repo := range sortedRepoKeys(s.Lib.Lock.Packs) {
		lp := s.Lib.Lock.Packs[repo]
		if lp == nil {
			continue
		}
		for _, n := range slices.Sorted(maps.Keys(lp.Skills)) {
			if ls := lp.Skills[n]; ls != nil && ls.Resolving {
				out = append(out, n)
			}
		}
	}
	return out
}

func sortedRepoKeys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }

// snapshotRefspec pushes and fetches the upstream snapshots
// (engine.SnapshotRefs). They are forced: the machine that pushes the
// library pushes the snapshots its skills.lock was written from; a merge
// on another machine whose own snapshot is gone finds its base in the
// library's history instead.
const snapshotRefspec = "+" + engine.SnapshotRefs + "*:" + engine.SnapshotRefs + "*"

// pushSnapshots pushes the upstream snapshots to origin, when there are
// any. A failure is a warning: the library itself was pushed.
func (s *Session) pushSnapshots() {
	if out, err := s.Lib.Git("for-each-ref", "--count=1", "--format=%(refname)", engine.SnapshotRefs); err != nil || out == "" {
		return
	}
	if _, err := s.Lib.Git("push", "-q", "origin", snapshotRefspec); err != nil {
		s.logf(Warn, "could not push upstream's versions (%s): %v", engine.SnapshotRefs, err)
	}
}

// FetchSnapshots fetches the upstream snapshots another machine pushed
// with the library at root (skd clone). Best effort: an error is
// returned for a warning only.
func FetchSnapshots(root string) error {
	if _, err := library.Git(root, "remote", "get-url", "origin"); err != nil {
		return nil
	}
	_, err := library.Git(root, "fetch", "-q", "origin", snapshotRefspec)
	return err
}

// StageFile stages a file of the library that was just saved (git add),
// in the repository it belongs to: private/ for a skill of yours kept on
// this machine, else the library's own. Saving is staging; skd sync (or
// update --commit) commits. Nothing is done outside a git repository.
func (s *Session) StageFile(path string) error {
	lib := s.Lib
	repo := lib.Root
	if fsutil.Within(lib.PrivateDir(), path) {
		if library.PrivateLink(lib.Root) != "" {
			return nil
		}
		repo = lib.PrivateDir()
	}
	if !fsutil.Within(repo, path) || !library.IsGitRepo(repo) {
		return nil
	}
	rel, err := filepath.Rel(repo, path)
	if err != nil {
		return err
	}
	_, err = library.Git(repo, "add", "--", ":(literal)"+filepath.ToSlash(rel))
	return err
}

// BackupOutcome is the result of Backup; its JSON form is what
// `skd sync --json` prints.
type BackupOutcome struct {
	Committed bool `json:"committed"`
	Pushed    bool `json:"pushed"`
	// Remote is origin's URL, empty when the library has no remote.
	Remote string `json:"remote,omitempty"`
	// PrivateUncommitted is set when private/ (a separate repository that
	// is never pushed here) has uncommitted changes.
	PrivateUncommitted bool `json:"privateUncommitted,omitempty"`
}

// DefaultBackupMessage is the commit message of a backup.
const DefaultBackupMessage = "chore: update skills library"

// Backup commits the library and pushes it to origin: `skd sync`. remote,
// when set, becomes origin first.
func (s *Session) Backup(remote, msg string) (*BackupOutcome, error) {
	lib := s.Lib
	if safetext.Has(remote) {
		return nil, i18n.Errorf("remote %q has control characters: not a git URL", remote)
	}
	if remote != "" {
		if _, err := lib.Git("remote", "get-url", "origin"); err == nil {
			// "--": a remote starting with "-" is a URL, never an option.
			if _, err := lib.Git("remote", "set-url", "--", "origin", remote); err != nil {
				return nil, err
			}
		} else if _, err := lib.Git("remote", "add", "--", "origin", remote); err != nil {
			return nil, err
		}
	}
	if msg == "" {
		msg = DefaultBackupMessage
	}
	out := &BackupOutcome{}
	committed, err := s.Commit(msg, false)
	if err != nil {
		return nil, err
	}
	out.Committed = committed
	if url, err := lib.Git("remote", "get-url", "origin"); err == nil {
		out.Remote = url
		if _, err := lib.Git("rev-parse", "--abbrev-ref", "@{u}"); err != nil {
			if _, err := lib.Git("push", "-q", "-u", "origin", "HEAD"); err != nil {
				return out, err
			}
		} else if _, err := lib.Git("push", "-q"); err != nil {
			return out, err
		}
		out.Pushed = true
		s.pushSnapshots()
	}
	// Not through a link inside the library: git status refreshes the
	// index of whatever repository it names.
	if library.PrivateLink(lib.Root) == "" && fsutil.Exists(filepath.Join(lib.PrivateDir(), ".git")) {
		if st, _ := library.Git(lib.PrivateDir(), "status", "--short"); st != "" {
			out.PrivateUncommitted = true
		}
	}
	return out, nil
}
