package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Someone else's library may commit private -> .. (git add -f): skd
// clone and skd init --from never set up a git repository, or anything
// else, through it, and sync does not run git there. Before, clone ran
// git init wherever the link led: a ~/.git that made the whole home
// folder a repository.
func TestPrivateLinkLeadsNowhere(t *testing.T) {
	e := newEnv(t)
	e.write("evil/skills.toml", "[targets]\ndefault = [\"agents\"]\n")
	e.write("evil/skills.lock", "")
	if err := os.Symlink("..", e.p("evil/private")); err != nil {
		t.Fatal(err)
	}
	evil := e.repo("evil")
	lib := e.p("lib")
	out, errOut, code := e.skd("", "clone", evil, lib)
	if code != 0 || !strings.Contains(out+errOut, "symbolic link") {
		t.Fatalf("clone: exit %d\n%s\n%s", code, out, errOut)
	}
	if _, err := os.Lstat(e.p(".git")); err == nil {
		t.Fatal("clone made the home folder a git repository")
	}
	if l, err := os.Readlink(filepath.Join(lib, "private")); err != nil || l != ".." {
		t.Fatalf("private changed: %q %v", l, err)
	}

	// sync does not look at what private/ leads to: a repository with
	// uncommitted changes there is not reported (or touched).
	e.write("other/f", "x")
	other := e.repo("other")
	e.write("other/g", "uncommitted")
	os.Remove(filepath.Join(lib, "private"))
	os.Symlink(other, filepath.Join(lib, "private"))
	e.git(lib, "remote", "remove", "origin")
	index, _ := os.Stat(filepath.Join(other, ".git", "index"))
	js := e.js(lib, "sync")
	if js["privateUncommitted"] != nil {
		t.Fatalf("sync looked through private/: %v", js)
	}
	if after, _ := os.Stat(filepath.Join(other, ".git", "index")); !after.ModTime().Equal(index.ModTime()) {
		t.Fatal("sync changed the index of the repository private/ leads to")
	}

	// init --from a directory whose private/ is a link: refused before
	// anything is created.
	os.MkdirAll(e.p("from"), 0o755)
	os.Symlink("..", e.p("from/private"))
	out, errOut, code = e.skd("", "init", "--no-skill", e.p("from"))
	if code == 0 || !strings.Contains(errOut, "symbolic link") {
		t.Fatalf("init: exit %d\n%s\n%s", code, out, errOut)
	}
	if entries, _ := os.ReadDir(e.p("from")); len(entries) != 1 {
		t.Fatalf("init created %v", entries)
	}
	if _, err := os.Lstat(e.p(".git")); err == nil {
		t.Fatal("init made the home folder a git repository")
	}
}
