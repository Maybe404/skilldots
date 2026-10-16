//go:build unix

package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A git older than 2.24 does not know --end-of-options; the error says
// which version skd needs instead of only git's complaint.
func TestOldGitIsNamed(t *testing.T) {
	bin := t.TempDir()
	fake := "#!/bin/sh\necho \"error: unknown option \\`end-of-options'\" >&2\nexit 129\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	_, err := Git(t.TempDir(), "merge-base", "--is-ancestor", "--end-of-options", "a", "b")
	if err == nil || !strings.Contains(err.Error(), "git 2.24 or later") {
		t.Fatalf("err = %v", err)
	}
	// Any other failure is git's own message.
	if _, err := Git(t.TempDir(), "status"); err == nil || strings.Contains(err.Error(), "2.24") {
		t.Fatalf("err = %v", err)
	}
}
