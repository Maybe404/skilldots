package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The user's own repository, end to end with a local bare one: add --own
// (no license warning), new --in, an edit in place, contribute <pack>
// --push sending both and pulling them back, a push from elsewhere
// coming in with update; skd own and its --off.
func TestOwnRepositoryCommands(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("work/skills/tdd", "tdd")
	e.repo("work")
	up := e.p("up.git")
	e.git(e.home, "clone", "-q", "--bare", e.p("work"), up)

	out, errOut, code := e.skd(lib, "add", up, "--all", "--own", "--alias", "mine")
	if code != 0 || strings.Contains(out+errOut, "license") || !strings.Contains(out, "skd new <name> --in") {
		t.Fatalf("add --own: exit %d\n%s%s", code, out, errOut)
	}
	if js := e.js(lib, "own"); len(js["own"].([]any)) != 1 {
		t.Fatalf("own: %v", js)
	}
	js := e.js(lib, "new", "notes", "--in", "mine")
	if js["draft"] != true || js["repoPath"] != "skills/notes" {
		t.Fatalf("new --in: %v", js)
	}
	notes, _ := js["path"].(string)
	if out := e.ok(lib, "list", "mine"); !strings.Contains(out, "notes +") || !strings.Contains(out, "not pushed yet") {
		t.Errorf("list:\n%s", out)
	}
	if out := e.ok(lib, "list"); !strings.Contains(out, "yours") {
		t.Errorf("list overview:\n%s", out)
	}
	tdd := strings.TrimSpace(e.ok(lib, "path", "tdd"))
	os.WriteFile(filepath.Join(tdd, "SKILL.md"), []byte("---\nname: tdd\ndescription: d\n---\nbody\nmine\n"), 0o644)
	os.WriteFile(filepath.Join(notes, "SKILL.md"), []byte("---\nname: notes\ndescription: n\n---\nTake notes.\n"), 0o644)
	if out := e.ok(lib, "list", "mine"); !strings.Contains(out, "tdd ✎") || !strings.Contains(out, "have not pushed it") {
		t.Errorf("list after the edit:\n%s", out)
	}

	dry := e.ok(lib, "contribute", "mine", "--dry-run")
	for _, want := range []string{"notes  new, at skills/notes", "+Take notes.", "+mine", "--push pushes to"} {
		if !strings.Contains(dry, want) {
			t.Errorf("dry run lacks %q:\n%s", want, dry)
		}
	}
	if _, errOut, code := e.skd(lib, "contribute", "mine", "--push"); code == 0 || !strings.Contains(errOut, "--yes") {
		t.Fatalf("sent without --yes: exit %d %s", code, errOut)
	}
	res := e.js(lib, "contribute", "mine", "--push", "--yes")
	if res["pushed"] != true || res["pulledBack"] == nil || len(res["skills"].([]any)) != 2 {
		t.Fatalf("contribute: %v", res)
	}
	if got := e.git(up, "log", "-1", "--format=%s"); got != "Add notes; update tdd" {
		t.Errorf("commit %q", got)
	}
	if got := e.git(up, "show", "HEAD:skills/notes/SKILL.md"); !strings.Contains(got, "Take notes.") {
		t.Errorf("upstream notes:\n%s", got)
	}
	if out := e.ok(lib, "list", "mine"); strings.Contains(out, "✎") || strings.Contains(out, " +") {
		t.Errorf("still marked after the push:\n%s", out)
	}

	// Pushed from another machine; the update brings it in, and reports
	// nothing about what was pushed from here.
	e.git(e.home, "clone", "-q", up, e.p("elsewhere"))
	e.write("elsewhere/skills/notes/SKILL.md", "---\nname: notes\ndescription: n\n---\nTake notes.\nFrom elsewhere.\n")
	e.git(e.p("elsewhere"), "commit", "-qam", "elsewhere")
	e.git(e.p("elsewhere"), "push", "-q", "origin", "HEAD")
	upd := e.js(lib, "update", "mine", "--yes")
	rep := upd["packs"].([]any)[0].(map[string]any)
	if ch, _ := rep["Changed"].([]any); len(ch) != 1 || ch[0] != "notes" {
		t.Fatalf("update: %v", rep)
	}
	if b, _ := os.ReadFile(filepath.Join(notes, "SKILL.md")); !strings.Contains(string(b), "From elsewhere.") {
		t.Errorf("notes:\n%s", b)
	}

	// Not a pack's and a skill's name at once.
	e.ok(lib, "new", "mine")
	if _, errOut, code := e.skd(lib, "contribute", "mine", "--dry-run"); code == 0 || !strings.Contains(errOut, "both the pack") {
		t.Errorf("ambiguous: exit %d %s", code, errOut)
	}

	e.ok(lib, "own", up, "--off")
	if _, errOut, code := e.skd(lib, "new", "later", "--in", up); code == 0 || !strings.Contains(errOut, "skd own") {
		t.Errorf("new --in a pack no longer own: exit %d %s", code, errOut)
	}
}

// A repository with no skill is refused unless it is the user's own.
func TestAddEmptyOwnRepository(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.write("work/README.md", "mine\n")
	e.repo("work")
	up := e.p("empty.git")
	e.git(e.home, "clone", "-q", "--bare", e.p("work"), up)
	if _, errOut, code := e.skd(lib, "add", up, "--all"); code == 0 || !strings.Contains(errOut, "--own") {
		t.Fatalf("empty, not own: exit %d %s", code, errOut)
	}
	e.ok(lib, "add", up, "--own")
	if js := e.js(lib, "new", "first", "--in", up); js["repoPath"] != "skills/first" {
		t.Fatalf("new: %v", js)
	}
	e.js(lib, "contribute", up, "--push", "--yes")
	if got := e.git(up, "show", "HEAD:skills/first/SKILL.md"); !strings.Contains(got, "name: first") {
		t.Errorf("upstream:\n%s", got)
	}
}
