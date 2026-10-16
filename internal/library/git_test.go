//go:build unix

package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeOldGit puts a git first on PATH that, like git before 2.24, refuses
// --end-of-options and otherwise prints its arguments.
func fakeOldGit(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	fake := "#!/bin/sh\nfor a in \"$@\"; do\n  if [ \"$a\" = --end-of-options ]; then echo \"error: unknown option \\`end-of-options'\" >&2; exit 129; fi\ndone\necho \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
}

// With a git older than 2.24 the commands still work: the arguments are
// passed again without --end-of-options when none of them could be taken
// for an option; otherwise the error says which git skd needs.
func TestOldGit(t *testing.T) {
	fakeOldGit(t)
	dir := t.TempDir()
	out, err := Git(dir, "log", "-1", "--end-of-options", "abc1234", "--", "-path")
	if err != nil || out != "log -1 abc1234 -- -path" {
		t.Fatalf("out %q, err %v", out, err)
	}
	_, err = Git(dir, "merge-base", "--is-ancestor", "--end-of-options", "--output=/x", "b")
	if err == nil || !strings.Contains(err.Error(), "git 2.24 or later") {
		t.Fatalf("an option after the marker: err = %v", err)
	}
}
