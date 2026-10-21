package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Adopting copies named otherwise than in the library: one that holds
// exactly a subscribed skill is that skill (installed for the agent, the
// copy to the trash); one whose SKILL.md name is taken is copied under
// its directory's name, which the dry run says. Neither leaves a name
// clash for apply to skip ever after.
func TestAdoptRenamedCopies(t *testing.T) {
	e := newEnv(t)
	e.installed(".claude")
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.ok(lib, "targets", "default", "agents")
	e.write("toolkit/skills/tdd/SKILL.md", "---\nname: tdd\ndescription: test first\n---\nbody\n")
	up := e.repo("toolkit")
	e.ok(lib, "add", up, "--skill", "tdd")
	e.write(".claude/skills/tdd-copy/SKILL.md", "---\nname: tdd\ndescription: test first\n---\nbody\n")
	e.write(".claude/skills/other/SKILL.md", "---\nname: tdd\ndescription: something else\n---\nmine\n")

	out := e.ok(lib, "adopt", "-n")
	if !strings.Contains(out, "already in the library as tdd") || !strings.Contains(out, "as other: its SKILL.md name tdd is taken") {
		t.Fatalf("adopt -n:\n%s", out)
	}
	out = e.ok(lib, "adopt", "--yes")
	if strings.Contains(out, "name conflict") || strings.Contains(out, "both") {
		t.Fatalf("adopt:\n%s", out)
	}
	isLink(t, e.p(".claude/skills/tdd"), lib)
	isLink(t, e.p(".claude/skills/other"), lib)
	if _, err := os.Lstat(e.p(".claude/skills/tdd-copy")); !os.IsNotExist(err) {
		t.Fatalf("the copy of tdd stayed: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(lib, "skills", "other", "SKILL.md")); !strings.Contains(string(b), "name: other") || !strings.Contains(string(b), "mine") {
		t.Fatalf("skills/other:\n%s", b)
	}
	if out, errOut, _ := e.skd(lib, "apply"); !strings.Contains(out, "up to date") || strings.Contains(out+errOut, "skipped") || strings.Contains(out+errOut, "conflict") {
		t.Fatalf("apply after adopt:\n%s%s", out, errOut)
	}
}

// A broken trash-notes.json is set aside the first time skd moves
// something to the trash, and the command says so: the warning is not
// lost on the way.
func TestBrokenTrashNotesAreSaid(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.ok(lib, "new", "foo")
	notes := filepath.Join(filepath.Dir(e.configPath()), "trash-notes.json")
	os.MkdirAll(filepath.Dir(notes), 0o755)
	os.WriteFile(notes, []byte("{broken"), 0o644)
	out, errOut, code := e.skd(lib, "remove", "foo")
	if code != 0 || !strings.Contains(out+errOut, "trash-notes.json was not valid JSON") {
		t.Fatalf("exit %d:\n%s%s", code, out, errOut)
	}
	backups, _ := filepath.Glob(notes + ".broken-*")
	if len(backups) != 1 {
		t.Fatalf("backups %v", backups)
	}
	if b, _ := os.ReadFile(backups[0]); string(b) != "{broken" {
		t.Fatalf("backup holds %q", b)
	}
}
