package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skd diff, the edited mark, skd contribute --push to a repository that
// is not on GitHub (a local bare one), and the record cleared once an
// update brings the change back.
func TestContributeCommands(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("work/skills/eng/tdd", "tdd")
	e.repo("work")
	up := e.p("up.git")
	e.git(e.home, "clone", "-q", "--bare", e.p("work"), up)
	e.ok(lib, "add", up, "--all")
	dir := strings.TrimSpace(e.ok(lib, "path", "tdd"))

	if out := e.ok(lib, "diff", "tdd"); !strings.Contains(out, "no changes of yours") {
		t.Errorf("diff before:\n%s", out)
	}
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: tdd\ndescription: d\n---\nbody\nmine\n"), 0o644)
	if out := e.ok(lib, "diff", "tdd"); !strings.Contains(out, "+mine") || !strings.Contains(out, "a/tdd/SKILL.md") {
		t.Errorf("diff:\n%s", out)
	}
	if js := e.js(lib, "diff", "tdd"); js["base"] != true || js["local"] != true {
		t.Errorf("diff --json: %v", js)
	}
	if out := e.ok(lib, "list", "--flat"); !strings.Contains(out, "tdd ✎") || !strings.Contains(out, "you changed it") {
		t.Errorf("list:\n%s", out)
	}

	// Never sent without a go-ahead.
	if _, errOut, code := e.skd(lib, "contribute", "tdd", "--push"); code == 0 || !strings.Contains(errOut, "--yes") {
		t.Fatalf("contribute without --yes: exit %d\n%s", code, errOut)
	}
	if out := e.ok(lib, "contribute", "tdd", "--dry-run"); !strings.Contains(out, "+mine") || !strings.Contains(out, "GitHub only") {
		t.Errorf("dry run:\n%s", out)
	}
	if _, _, code := e.skd(lib, "contribute", "tdd", "--pr", "--yes"); code == 0 {
		t.Error("a pull request elsewhere than GitHub")
	}
	js := e.js(lib, "contribute", "tdd", "--push", "--yes", "-m", "tdd: mine")
	if js["pushed"] != true || js["mode"] != "push" {
		t.Fatalf("contribute: %v", js)
	}
	if got := e.git(up, "log", "-1", "--format=%s"); got != "tdd: mine" {
		t.Errorf("upstream: %q", got)
	}
	if list := e.js(lib, "contribute")["contributions"].([]any); len(list) != 1 {
		t.Fatalf("contributions: %v", list)
	}
	// The change came back with the next update: nothing is "mine" now.
	e.ok(lib, "update", "--yes")
	if list := e.js(lib, "contribute")["contributions"].([]any); len(list) != 0 {
		t.Errorf("contributions after the update: %v", list)
	}
	if out := e.ok(lib, "list", "--flat"); strings.Contains(out, "✎") {
		t.Errorf("still marked:\n%s", out)
	}
}

// skd sync merges what the remote has; a conflict shelves it (non-zero
// exit, library unchanged); --resolve, --continue, --abort.
func TestSyncCommands(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.ok(lib, "new", "mine")
	remote := e.p("remote.git")
	e.git(e.home, "init", "-q", "--bare", remote)
	e.ok(lib, "sync", "--remote", remote)
	other := e.p("other")
	e.git(e.home, "clone", "-q", remote, other)
	md := filepath.Join("skills", "mine", "SKILL.md")
	os.WriteFile(filepath.Join(other, md), []byte("---\nname: mine\ndescription: other\n---\n"), 0o644)
	e.git(other, "commit", "-qam", "other")
	e.git(other, "push", "-q")
	os.WriteFile(filepath.Join(lib, md), []byte("---\nname: mine\ndescription: here\n---\n"), 0o644)

	out, errOut, code := e.skd(lib, "sync")
	if code == 0 || !strings.Contains(out+errOut, "skd sync --resolve") || !strings.Contains(out+errOut, md) {
		t.Fatalf("sync with a conflict: exit %d\n%s%s", code, out, errOut)
	}
	if b, _ := os.ReadFile(filepath.Join(lib, md)); !strings.Contains(string(b), "here") || strings.Contains(string(b), "<<<<<<<") {
		t.Fatalf("library changed:\n%s", b)
	}
	e.ok(lib, "sync", "--resolve")
	if st := e.git(lib, "status", "--porcelain"); !strings.Contains(st, "UU "+md) {
		t.Fatalf("status:\n%s", st)
	}
	if _, _, code := e.skd(lib, "sync", "--continue"); code == 0 {
		t.Fatal("continued with markers")
	}
	e.ok(lib, "sync", "--abort")
	if st := e.git(lib, "status", "--porcelain"); st != "" {
		t.Fatalf("after abort:\n%s", st)
	}
	e.ok(lib, "sync", "--resolve")
	os.WriteFile(filepath.Join(lib, md), []byte("---\nname: mine\ndescription: here and other\n---\n"), 0o644)
	e.ok(lib, "sync", "--continue")
	if got := e.git(remote, "show", "HEAD:"+filepath.ToSlash(md)); !strings.Contains(got, "here and other") {
		t.Errorf("remote:\n%s", got)
	}
	// Nothing left: a plain sync goes through.
	e.ok(lib, "sync")
}
