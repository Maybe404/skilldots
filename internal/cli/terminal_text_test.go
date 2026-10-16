package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/safetext"
)

// clean fails the test when out holds a control character (other than
// line breaks) or a bidi control: something that could rewrite the
// terminal or reorder what it shows.
func clean(t *testing.T, what, out string) {
	t.Helper()
	if strings.ContainsFunc(strings.ReplaceAll(out, "\n", ""), safetext.Unsafe) || strings.Contains(out, "��") {
		t.Errorf("%s printed control characters:\n%q", what, out)
	}
}

// Text from an upstream repository (a SKILL.md description, a directory
// name used as category, the directory of a skill that is rejected) and
// from library files never reaches the terminal with control characters
// or bidi controls in it: not in show, add, list --all, update, nor in
// warnings and errors.
func TestUpstreamTextReachesTheTerminalClean(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	evil := "\x1b]0;pwned\x07\x1b[2J\u202eevil\u009b31m"
	e.write("up/skills/eng/tdd/SKILL.md", "---\nname: tdd\ndescription: tests first "+evil+"\n---\n")
	e.skill("up/skills/cat"+evil+"/hidden", "hidden")
	e.write("up/skills/badname/SKILL.md", "---\nname: bad"+evil+"\n---\n")
	up := e.repo("up")

	for _, args := range [][]string{{"show", up}, {"add", up, "--all"}, {"list", "--all"}, {"list", up}} {
		out, errOut, code := e.skd(lib, args...)
		if code != 0 {
			t.Fatalf("skd %v: exit %d\n%s\n%s", args, code, out, errOut)
		}
		clean(t, strings.Join(args, " "), out+errOut)
		if args[0] == "show" && (!strings.Contains(out, "tests first") || !strings.Contains(errOut, "control characters")) {
			t.Errorf("show: description or rejection missing:\n%s\n%s", out, errOut)
		}
	}
	// --json: the description is display text and is cleaned too; the
	// skill in a directory with escapes is rejected, not offered.
	js := e.js(lib, "show", up)
	for _, sk := range js["skills"].([]any) {
		m := sk.(map[string]any)
		if safetext.Has(m["Description"].(string)) || safetext.Has(m["Category"].(string)) {
			t.Errorf("show --json: %q", m)
		}
		if m["Name"] == "hidden" {
			t.Errorf("show --json offers the skill in a directory with escapes: %v", m)
		}
	}
	// The source given to add.
	out, errOut, code := e.skd(lib, "add", "x/y"+evil)
	if code == 0 || !strings.Contains(errOut, "control characters") {
		t.Errorf("add with escapes in the source: exit %d\n%s", code, errOut)
	}
	clean(t, "add bad source", out+errOut)
	// What other tools put in an agent directory.
	e.skill(".agents/skills/odd"+evil, "odd")
	out, errOut, _ = e.skd(lib, "scan")
	if !strings.Contains(out, "odd") {
		t.Errorf("scan does not list the entry:\n%s", out)
	}
	clean(t, "scan", out+errOut)

	e.write("up/skills/eng/tdd/x.md", "x")
	e.skill("up/skills/new"+evil+"/later", "later")
	e.git(up, "add", "-A")
	e.git(up, "commit", "-qm", "more "+evil)
	// A skill in the lock that is no longer selected (a hand-edited
	// lock): update drops it and says so. (A lock whose names hold
	// control characters is refused: TestHostileManifestAndLockAreRefused.)
	lockPath := filepath.Join(lib, "skills.lock")
	b, _ := os.ReadFile(lockPath)
	os.WriteFile(lockPath, append(b, []byte("\n[packs.'"+up+"'.skills.gone]\nhash = 'x'\n")...), 0o644)
	out, errOut, code = e.skd(lib, "update")
	if code != 0 {
		t.Fatalf("update: exit %d\n%s\n%s", code, out, errOut)
	}
	clean(t, "update", out+errOut)
	if !strings.Contains(out, "gone") {
		t.Errorf("update does not report the gone skill:\n%s", out)
	}
	out, errOut, _ = e.skd(lib, "history")
	clean(t, "history", out+errOut)

	// A library file written to attack: a lock whose category holds
	// escapes, a manifest whose repo does.
	b, _ = os.ReadFile(lockPath)
	bad := strings.Replace(string(b), `category = 'eng'`, `category = "eng\u001b[2J\u202e"`, 1)
	if bad == string(b) {
		t.Fatalf("no category in the lock:\n%s", b)
	}
	os.WriteFile(lockPath, []byte(bad), 0o644)
	out, errOut, _ = e.skd(lib, "list", "--all")
	clean(t, "list with a bad lock", out+errOut)
	if !strings.Contains(errOut, "tdd") {
		t.Errorf("no warning about the bad category:\n%s", errOut)
	}
	os.WriteFile(lockPath, b, 0o644)
	mPath := filepath.Join(lib, "skills.toml")
	m, _ := os.ReadFile(mPath)
	os.WriteFile(mPath, append(m, []byte("\n[[pack]]\nrepo = \"x/y\\u001b]0;t\\u0007\"\nuse = []\n")...), 0o644)
	out, errOut, code = e.skd(lib, "list")
	if code == 0 || !strings.Contains(errOut, "skills.toml") || !strings.Contains(errOut, "control characters") {
		t.Fatalf("bad manifest: exit %d\n%s\n%s", code, out, errOut)
	}
	clean(t, "list with a bad manifest", out+errOut)
}
