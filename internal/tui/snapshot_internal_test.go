package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/pack"
)

// The dashboard reads the age of a pack's commit with git log. A commit
// that is an option (a lock built in memory, past Load's check) stays a
// revision to git: --output=<file> writes nothing.
func TestPackMetaCommitIsNeverAnOption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(home, "cache"))
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	cache := pack.CachePath("a/b")
	for _, args := range [][]string{{"init", "-q", cache}, {"-C", cache, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	root := filepath.Join(home, "lib")
	os.MkdirAll(root, 0o755)
	exec.Command("git", "init", "-q", root).Run()
	target := filepath.Join(home, "written")
	pk := &Pack{Repo: "a/b"}
	packMeta(&library.Library{Root: root}, &lock.Pack{Commit: "--output=" + target}, pk)
	if _, err := os.Stat(target); err == nil {
		t.Fatal("git took the commit as an option and wrote the file")
	}
}
