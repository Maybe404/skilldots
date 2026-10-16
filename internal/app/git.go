package app

import (
	"path/filepath"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
)

// commitPaths is what a library commit takes: everything under the root
// except the temporary files of an interrupted save, which a .gitignore
// written before they were ignored would let through.
var commitPaths = []string{".", ":(exclude,glob).skills.toml.tmp-*", ":(exclude,glob).skills.lock.tmp-*"}

// Commit commits the library's own files and, with push, pushes when the
// branch has an upstream. It reports whether a commit was made.
func (s *Session) Commit(msg string, push bool) (bool, error) {
	lib := s.Lib
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
	}
	return true, nil
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
	if remote != "" {
		if _, err := lib.Git("remote", "get-url", "origin"); err == nil {
			if _, err := lib.Git("remote", "set-url", "origin", remote); err != nil {
				return nil, err
			}
		} else if _, err := lib.Git("remote", "add", "origin", remote); err != nil {
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
	}
	if fsutil.Exists(filepath.Join(lib.PrivateDir(), ".git")) {
		if st, _ := library.Git(lib.PrivateDir(), "status", "--short"); st != "" {
			out.PrivateUncommitted = true
		}
	}
	return out, nil
}
