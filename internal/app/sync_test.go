package app_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

// The remote given to skd sync --remote is a URL to git even when it
// starts with "-": never an option to git remote add or set-url.
func TestBackupRemoteIsNeverAnOption(t *testing.T) {
	up := upstream(t)
	_, root := side(t, up)
	origin := func() string {
		out, _ := exec.Command("git", "-C", root, "remote", "get-url", "origin").Output()
		return strings.TrimSpace(string(out))
	}
	for _, remote := range []string{"-backup.git", "--mirror=fetch"} {
		s, _ := app.Open(root)
		// Pushing there fails (nothing is there); the remote is set first.
		s.Backup(remote, "")
		if got := origin(); got != remote {
			t.Errorf("remote %q: origin is %q", remote, got)
		}
	}
}
