package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeEditor installs a windowed "editor" (named code) that writes the
// directory it was given to a file, and returns a function that waits for
// the next one.
func (e *env) fakeEditor() func() string {
	e.t.Helper()
	got := e.p("opened")
	e.write("bin/code", "#!/bin/sh\nprintf '%s\\n' \"$1\" >> '"+got+"'\n")
	os.Chmod(e.p("bin/code"), 0o755)
	e.ok("", "config", "set", "editor", e.p("bin/code"))
	seen := 0
	return func() string {
		e.t.Helper()
		for i := 0; i < 100; i++ {
			b, _ := os.ReadFile(got)
			if lines := strings.Fields(string(b)); len(lines) > seen {
				seen++
				return lines[seen-1]
			}
			time.Sleep(50 * time.Millisecond)
		}
		e.t.Fatal("the editor was not started")
		return ""
	}
}

func TestEdit(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("up/skills/eng/tdd", "tdd")
	up := e.repo("up")
	e.ok("", "add", up, "--skill", "tdd")
	e.ok("", "new", "notes")
	next := e.fakeEditor()

	r := e.js("", "edit", "notes")
	if r["path"] != filepath.Join(lib, "skills", "notes") || r["editorFrom"] != "editor setting" {
		t.Fatalf("edit notes: %v", r)
	}
	if got := next(); got != filepath.Join(lib, "skills", "notes") {
		t.Fatalf("opened %s", got)
	}
	// Vendored, no terminal: refused, nothing opened, nothing forked.
	_, errOut, code := e.skd("", "edit", "tdd")
	if code == 0 || !strings.Contains(errOut, "--fork") || !strings.Contains(errOut, "skd update replaces") {
		t.Fatalf("vendored without a choice: exit %d\n%s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(lib, "skills", "tdd")); err == nil {
		t.Fatal("forked without being asked")
	}
	e.ok("", "edit", "tdd", "--force")
	if got := next(); !strings.Contains(got, filepath.Join("vendor")) {
		t.Fatalf("--force opened %s", got)
	}
	e.ok("", "edit", "tdd", "--fork")
	if got := next(); got != filepath.Join(lib, "skills", "tdd") {
		t.Fatalf("--fork opened %s", got)
	}
	if b, _ := os.ReadFile(filepath.Join(lib, "skills.toml")); strings.Contains(string(b), "'tdd'") {
		t.Fatalf("the fork did not deselect upstream:\n%s", b)
	}
	// A terminal editor with no terminal: says so instead of hanging.
	e.write("bin/vim", "#!/bin/sh\nexit 0\n")
	os.Chmod(e.p("bin/vim"), 0o755)
	e.ok("", "config", "set", "editor", e.p("bin/vim"))
	_, errOut, code = e.skd("", "edit", "notes")
	if code == 0 || !strings.Contains(errOut, "in the terminal") || !strings.Contains(errOut, "skd config set editor-kind window") {
		t.Fatalf("vim without a terminal: exit %d\n%s", code, errOut)
	}
}

// An editor command skd does not know is taken for a terminal editor.
// When it opens a window, editor-kind says so, and it then opens skills
// without a terminal, --force and --fork included; what the refusal
// suggested works.
func TestEditCustomWindowEditor(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("up/skills/eng/tdd", "tdd")
	up := e.repo("up")
	e.ok("", "add", up, "--skill", "tdd")
	e.ok("", "new", "notes")
	got := e.p("opened")
	e.write("bin/myeditor", "#!/bin/sh\nprintf '%s\\n' \"$1\" >> '"+got+"'\n")
	os.Chmod(e.p("bin/myeditor"), 0o755)
	e.ok("", "config", "set", "editor", e.p("bin/myeditor"))
	_, errOut, code := e.skd("", "edit", "tdd", "--force")
	if code == 0 || !strings.Contains(errOut, "skd config set editor-kind window") {
		t.Fatalf("unknown editor without a terminal: exit %d\n%s", code, errOut)
	}
	e.ok("", "config", "set", "editor-kind", "window")
	e.ok("", "edit", "tdd", "--force")
	e.ok("", "edit", "notes")
	opened := func() []string {
		for i := 0; i < 100; i++ {
			if b, _ := os.ReadFile(got); len(strings.Fields(string(b))) == 2 {
				return strings.Fields(string(b))
			}
			time.Sleep(50 * time.Millisecond)
		}
		return nil
	}()
	if len(opened) != 2 || !slices.Contains(opened, filepath.Join(lib, "skills", "notes")) {
		t.Fatalf("opened %v", opened)
	}
	// And the other way: a known window editor run in a terminal.
	e.write("bin/code", "#!/bin/sh\nexit 0\n")
	os.Chmod(e.p("bin/code"), 0o755)
	e.ok("", "config", "set", "editor", e.p("bin/code"))
	e.ok("", "config", "set", "editor-kind", "terminal")
	if _, errOut, code := e.skd("", "edit", "notes"); code == 0 || !strings.Contains(errOut, "runs in a terminal") {
		t.Fatalf("editor-kind terminal: exit %d\n%s", code, errOut)
	}
}

// --fork with an editor that cannot run here fails before forking: the
// library is as it was.
func TestEditForkNeedsAWorkingEditor(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.skill("up/skills/eng/tdd", "tdd")
	up := e.repo("up")
	e.ok("", "add", up, "--skill", "tdd")
	e.write("bin/vim", "#!/bin/sh\nexit 0\n")
	os.Chmod(e.p("bin/vim"), 0o755)
	manifest, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
	for _, ed := range []string{e.p("bin/vim"), e.p("bin/missing")} {
		e.ok("", "config", "set", "editor", ed)
		_, errOut, code := e.skd("", "edit", "tdd", "--fork")
		if code == 0 {
			t.Fatalf("%s: forked and opened without a terminal", ed)
		}
		if _, err := os.Stat(filepath.Join(lib, "skills", "tdd")); err == nil {
			t.Fatalf("%s: forked although the editor could not run:\n%s", ed, errOut)
		}
		if b, _ := os.ReadFile(filepath.Join(lib, "skills.toml")); string(b) != string(manifest) {
			t.Fatalf("%s: skills.toml changed:\n%s", ed, b)
		}
	}
}
