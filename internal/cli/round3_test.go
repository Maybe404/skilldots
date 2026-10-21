package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
