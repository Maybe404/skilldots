package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/cli"
	"github.com/Maybe404/skilldots/internal/hostile"
	"github.com/Maybe404/skilldots/internal/safetext"
)

// gate fails the test when out, everything a command printed for a
// person (colour off, so none of it is skd's own), holds a control
// character other than a line break, a bidi control or an invalid byte,
// or when the command panicked.
func gate(t *testing.T, what string, out string, code int) {
	t.Helper()
	if i := strings.IndexFunc(strings.ReplaceAll(out, "\n", ""), safetext.Unsafe); i >= 0 || strings.Contains(out, "\uFFFD\uFFFD") {
		t.Errorf("skd %s printed control characters:\n%q", what, out)
	}
	if code == 2 || strings.Contains(out, "goroutine ") || strings.Contains(out, "panic:") {
		t.Errorf("skd %s panicked (exit %d):\n%s", what, code, out)
	}
}

// hostileCommands are the commands the regression gate runs, in order
// (the last ones take the pack apart). In the arguments "@pack" is the
// pack's repository, "@trash" a trash entry whose origin is hostile, and
// "@evil" hostile.Evil. Every command of the tree must be in it
// (TestHostileCommandsCoverTheTree): a new command gets a line here.
var hostileCommands = [][]string{
	{"show", "@pack"},
	{"show", "x/y@evil"},
	{"outdated"},
	{"apply", "-n"},
	{"apply"},
	{"update", "@pack"},
	{"history"},
	{"history", "@pack", "-n", "0"},
	{"history", "nope@evil"},
	{"update"},
	{"add", "@pack", "--all"},
	{"add", "@pack", "--skill", "nope@evil"},
	{"add", "@pack", "--alias", "a@evil"},
	{"add", "x/y@evil"},
	{"list"},
	{"list", "--all"},
	{"list", "--flat"},
	{"list", "@pack"},
	{"list", "nope@evil"},
	{"list", "--agent", "agents"},
	{"list", "--agent", "nope@evil"},
	{"targets"},
	{"path", "tdd"},
	{"path", "nope@evil"},
	{"enable", "tdd", "--agent", "claude"},
	{"enable", "nope@evil"},
	{"disable", "tdd", "--agent", "nope@evil"},
	{"disable", "tdd", "--agent", "claude"},
	{"scan"},
	{"adopt", "-n"},
	{"adopt", "--yes"},
	{"apply", "--force"},
	{"trash"},
	{"trash", "restore", "@trash"},
	{"trash", "restore", "nope@evil"},
	{"trash", "empty"},
	{"config", "list"},
	{"config", "get", "editor"},
	{"config", "set", "editor", "code@evil"},
	{"config", "set", "nope@evil", "x"},
	{"edit", "tdd"},
	{"edit", "tdd", "--force"},
	{"edit", "nope@evil"},
	{"fork", "tdd", "--as", "nope@evil"},
	{"fork", "review"},
	{"new", "nope@evil"},
	{"new", "mine"},
	{"sync", "--remote", "nope@evil"},
	{"sync"},
	{"use", "@lib"},
	{"use", "nope@evil"},
	{"init", "@lib"},
	{"init", "@new", "--no-skill"},
	{"clone", "nope@evil"},
	{"remove", "tdd"},
	{"remove", "nope@evil"},
	{"purge", "nope@evil"},
	{"purge", "@pack", "--yes"},
}

// TestHostileCommandsCoverTheTree keeps the gate complete: every command
// skd has is run against the hostile library.
func TestHostileCommandsCoverTheTree(t *testing.T) {
	ran := map[string]bool{}
	for _, args := range hostileCommands {
		var words []string
		for _, a := range args {
			if strings.HasPrefix(a, "-") || strings.HasPrefix(a, "@") || strings.Contains(a, "@") {
				break
			}
			words = append(words, a)
			ran[strings.Join(words, " ")] = true
		}
	}
	var walk func(prefix string, c *cobra.Command)
	walk = func(prefix string, c *cobra.Command) {
		for _, sub := range c.Commands() {
			p := strings.TrimSpace(prefix + " " + sub.Name())
			if !ran[p] {
				t.Errorf("skd %s is not run against the hostile library: add it to hostileCommands", p)
			}
			walk(p, sub)
		}
	}
	walk("", cli.Root())
}

// hostileLibrary builds the library of the gate: a pack from a hostile
// upstream that changed since, a hostile history, trash, agent directory
// and settings file. It returns the library and the pack's repository.
func hostileLibrary(t *testing.T, e *env) (lib, up string) {
	lib, up, _ = hostileLibraryLures(t, e)
	return lib, up
}

// hostileLibraryLures is hostileLibrary that also returns the trash
// entries that must never be restored.
func hostileLibraryLures(t *testing.T, e *env) (lib, up string, lures hostile.Lures) {
	t.Helper()
	lib = e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	git := func(dir string, args ...string) { e.git(dir, args...) }
	up = e.p("up")
	hostile.Upstream(up, git)
	if _, errOut, code := e.skd(lib, "add", up, "--all"); code != 0 {
		t.Fatalf("add: exit %d\n%s", code, errOut)
	}
	hostile.Change(up, git)
	lures = hostile.Extras(lib, e.p(".agents/skills"), e.home)
	// Something in the way of a link: apply skips it, --force moves it
	// aside.
	os.Remove(e.p(".agents/skills/review"))
	e.write(".agents/skills/review/SKILL.md", "---\nname: review\ndescription: "+hostile.Evil+"\n---\n")
	b, _ := os.ReadFile(e.configPath())
	hostile.Settings(e.configPath())
	if f, err := os.OpenFile(e.configPath(), os.O_APPEND|os.O_WRONLY, 0); err == nil {
		f.Write(b) // keep the library setting init wrote
		f.Close()
	}
	return lib, up, lures
}

func expand(args []string, lib, up string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		a = strings.ReplaceAll(a, "@pack", up)
		a = strings.ReplaceAll(a, "@lib", lib)
		a = strings.ReplaceAll(a, "@new", filepath.Join(filepath.Dir(lib), "lib2"))
		a = strings.ReplaceAll(a, "@trash", "20240102-030405/t"+hostile.Evil)
		out[i] = strings.ReplaceAll(a, "@evil", hostile.Evil)
	}
	return out
}

// TestHostileLibraryPrintsClean is the regression gate for "nothing skd
// prints for a person carries control characters": against a library
// where everything read from outside carries escape sequences, BEL, C1
// and bidi controls, every command (dry runs, error paths, the trash,
// the settings) prints none of them, on stdout and stderr, with and
// without --json (whose stdout is data and not checked), and none
// panics. The binary is built without the screen writer (rawoutput), so
// this checks the commands' own cleaning, not the last line of defence.
func TestHostileLibraryPrintsClean(t *testing.T) {
	e := newEnv(t)
	lib, up, lures := hostileLibraryLures(t, e)
	// The trash of someone else's library: no entry it lists, and no id
	// reaching out of it, moves a directory where skd never takes one
	// from, or moves one from outside the trash. `skd trash` says which
	// cannot be restored.
	out, _, _ := e.skd(lib, "trash", "--json")
	var listed []struct {
		ID         string `json:"id"`
		Restorable bool   `json:"restorable"`
		Problem    string `json:"problem"`
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("trash --json: %v\n%s", err, out)
	}
	for _, it := range listed {
		if it.Restorable || it.Problem == "" {
			t.Errorf("trash lists %q as restorable (%q)", it.ID, it.Problem)
		}
	}
	if out, _, _ := e.skd(lib, "trash"); strings.Count(out, "cannot be restored") != len(listed) {
		t.Errorf("trash does not say which cannot be restored:\n%s", out)
	}
	for _, id := range lures.IDs {
		for _, json := range []bool{false, true} {
			a := []string{"trash", "restore", id}
			if json {
				a = append(a, "--json")
			}
			out, errOut, code := e.skd(lib, a...)
			if json {
				out = ""
			}
			gate(t, strings.Join(a, " "), out+errOut, code)
			if code == 0 {
				t.Errorf("skd trash restore %q succeeded:\n%s", id, out)
			}
		}
	}
	for _, p := range lures.Never {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("restoring created %q", p)
		}
	}
	for _, p := range lures.Kept {
		if _, err := os.Lstat(filepath.Join(p, "SKILL.md")); err != nil {
			t.Errorf("restoring moved %q", p)
		}
	}
	for _, json := range []bool{false, true} {
		for _, args := range hostileCommands {
			if json && slices.Contains([]string{"purge", "remove", "new", "fork", "sync", "clone", "init", "use", "adopt", "update"}, args[0]) {
				continue // these change the library; once is enough
			}
			a := expand(args, lib, up)
			name := strings.Join(args, " ")
			if json {
				a, name = append(a, "--json"), name+" --json"
			}
			out, errOut, code := e.skd(lib, a...)
			if json {
				out = "" // data
			}
			gate(t, name, out+errOut, code)
			if testing.Verbose() {
				t.Logf("skd %s: exit %d\n%s%s", name, code, out, errOut)
			}
		}
	}
	// What was refused left nothing behind: no directory restored to a
	// hostile origin, no skill adopted under a hostile name, no hostile
	// setting or remote.
	restored := e.p(".agents/skills/" + strings.TrimSpace("o"+hostile.Evil)) // the origin as recorded, trimmed
	for _, p := range []string{restored, filepath.Join(lib, "skills", "odd"+hostile.Evil)} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("%q was created", p)
		}
	}
	if b, _ := os.ReadFile(e.configPath()); strings.Contains(string(b), "code") {
		t.Errorf("config set stored a hostile value:\n%s", b)
	}
	if out, _, _ := e.skd(lib, "show", up); !strings.Contains(out, "review") {
		t.Errorf("show:\n%s", out)
	}
	if remotes := e.git(lib, "remote", "-v"); strings.Contains(remotes, "nope") {
		t.Errorf("sync stored a hostile remote: %q", remotes)
	}
	// Normal text, that only looks unusual, comes through.
	out, _, _ = e.skd(lib, "show", up)
	if !strings.Contains(out, "中文 描述 🚀") {
		t.Errorf("show lost the description of review:\n%s", out)
	}
}

// The build people run, last line of defence included, prints the
// hostile library cleanly too: a smoke test of the guarded binary (the
// gate above checks the commands without it).
func TestGuardedBuildPrintsClean(t *testing.T) {
	e := newEnv(t)
	lib, up, lures := hostileLibraryLures(t, e)
	for _, args := range [][]string{
		{"list", "--all"}, {"show", up}, {"outdated"}, {"apply", "-n"}, {"history"}, {"trash"},
		{"trash", "restore", lures.IDs[0]}, {"config", "list"}, {"scan"}, {"show", "x/y" + hostile.Evil},
	} {
		out, errOut, code := e.run(skdGuarded, lib, args...)
		gate(t, "(guarded) "+strings.Join(args, " "), out+errOut, code)
		if strings.Contains(out+errOut, "\x1b") {
			t.Errorf("skd %s printed an escape sequence:\n%q", strings.Join(args, " "), out+errOut)
		}
	}
}

// A manifest or lock with control characters anywhere, in any value,
// array element or key, is refused with the place named and quoted, and
// every command says so cleanly.
func TestHostileManifestAndLockAreRefused(t *testing.T) {
	E := hostile.TOML(hostile.Evil)
	K := hostile.TOML("k" + hostile.Evil) // a key
	manifests := map[string]string{
		"targets.default[1]":        "[targets]\ndefault = [\"agents\", " + E + "]\n",
		"targets.custom.":           "[targets]\ndefault = [\"agents\"]\n[targets.custom." + K + "]\npath = \"/x\"\n",
		"targets.custom.my.path":    "[targets]\ndefault = [\"agents\"]\n[targets.custom.my]\npath = " + E + "\n",
		"targets.custom.my.mode":    "[targets]\ndefault = [\"agents\"]\n[targets.custom.my]\npath = \"/x\"\nmode = " + E + "\n",
		"pack[0].repo":              "[[pack]]\nrepo = " + E + "\nuse = []\n",
		"pack[0].ref":               "[[pack]]\nrepo = \"x/y\"\nref = " + E + "\nuse = []\n",
		"pack[0].alias":             "[[pack]]\nrepo = \"x/y\"\nalias = " + E + "\nuse = []\n",
		"pack[0].use[0]":            "[[pack]]\nrepo = \"x/y\"\nuse = [" + E + "]\n",
		"pack[0].skip[0]":           "[[pack]]\nrepo = \"x/y\"\nuse = [\"*\"]\nskip = [" + E + "]\n",
		"pack[0].targets[0]":        "[[pack]]\nrepo = \"x/y\"\nuse = []\ntargets = [" + E + "]\n",
		"pack[0].skill.":            "[[pack]]\nrepo = \"x/y\"\nuse = []\n[pack.skill." + K + "]\nas = \"z\"\n",
		"pack[0].skill.tdd.as":      "[[pack]]\nrepo = \"x/y\"\nuse = []\n[pack.skill.tdd]\nas = " + E + "\n",
		"pack[0].skill.tdd.targets": "[[pack]]\nrepo = \"x/y\"\nuse = []\n[pack.skill.tdd]\ntargets = [" + E + "]\n",
		"app[0].name":               "[[app]]\nname = " + E + "\npath = \"/x\"\n",
		"app[0].path":               "[[app]]\nname = \"a\"\npath = " + E + "\n",
		"app[0].targets[0]":         "[[app]]\nname = \"a\"\npath = \"/x\"\ntargets = [" + E + "]\n",
		"local[0].name":             "[[local]]\nname = " + E + "\n",
		"local[0].targets[0]":       "[[local]]\nname = \"a\"\ntargets = [" + E + "]\n",
		"unknown":                   "unknown = " + E + "\n",
		"key ":                      K + " = \"x\"\n",
	}
	locks := map[string]string{
		"key packs.":                    "version = 1\n[packs." + K + "]\ncommit = \"abcdef1\"\n",
		"packs.\"x/y\".commit":          "version = 1\n[packs.\"x/y\"]\ncommit = " + E + "\n",
		"packs.\"x/y\".ref":             "version = 1\n[packs.\"x/y\"]\ncommit = \"abcdef1\"\nref = " + E + "\n",
		"packs.\"x/y\".upstream[0]":     "version = 1\n[packs.\"x/y\"]\ncommit = \"abcdef1\"\nupstream = [" + E + "]\n",
		"key packs.\"x/y\".skills.":     "version = 1\n[packs.\"x/y\"]\ncommit = \"abcdef1\"\n[packs.\"x/y\".skills." + K + "]\nhash = \"h\"\n",
		"packs.\"x/y\".skills.tdd.cat":  "version = 1\n[packs.\"x/y\"]\ncommit = \"abcdef1\"\n[packs.\"x/y\".skills.tdd]\ncategory = " + E + "\nhash = \"h\"\n",
		"packs.\"x/y\".skills.tdd.hash": "version = 1\n[packs.\"x/y\"]\ncommit = \"abcdef1\"\n[packs.\"x/y\".skills.tdd]\nhash = " + E + "\n",
		"key links.":                    "version = 1\n[links." + K + "]\nmode = \"link\"\nsource = \"skills/a\"\n",
		"links.\"agents/a\".mode":       "version = 1\n[links.\"agents/a\"]\nmode = " + E + "\nsource = \"skills/a\"\n",
		"links.\"agents/a\".source":     "version = 1\n[links.\"agents/a\"]\nmode = \"link\"\nsource = " + E + "\n",
		"links.\"agents/a\".hash":       "version = 1\n[links.\"agents/a\"]\nmode = \"copy\"\nsource = \"skills/a\"\nhash = " + E + "\n",
		"links.\"agents/a\".path":       "version = 1\n[links.\"agents/a\"]\nmode = \"link\"\nsource = \"skills/a\"\npath = " + E + "\n",
	}
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	mPath, lPath := filepath.Join(lib, "skills.toml"), filepath.Join(lib, "skills.lock")
	goodM, _ := os.ReadFile(mPath)
	goodL, _ := os.ReadFile(lPath)
	// The map keys are what the refusal must name.
	refused := func(file, place, body string) {
		t.Helper()
		out, errOut, code := e.skd(lib, "list")
		gate(t, file+" "+place, out+errOut, code)
		if code == 0 || !strings.Contains(errOut, file) || !strings.Contains(errOut, "control characters") || !strings.Contains(errOut, place) {
			t.Errorf("%s with %s: exit %d, want a refusal naming %s:\n%s\n%s", file, place, code, place, body, errOut)
		}
	}
	for place, body := range manifests {
		os.WriteFile(mPath, []byte(body), 0o644)
		refused("skills.toml", place, body)
	}
	os.WriteFile(mPath, goodM, 0o644)
	for place, body := range locks {
		os.WriteFile(lPath, []byte(body), 0o644)
		refused("skills.lock", place, body)
	}
	os.WriteFile(lPath, goodL, 0o644)

	// Every command, against a manifest that is hostile everywhere, then
	// a lock that is.
	everywhere := map[string]string{
		mPath: "[targets]\ndefault = [\"agents\", " + E + "]\n[targets.custom." + K + "]\npath = " + E + "\nmode = " + E + "\n" +
			"[[pack]]\nrepo = " + E + "\nref = " + E + "\nalias = " + E + "\nuse = [" + E + "]\nskip = [" + E + "]\ntargets = [" + E + "]\n" +
			"[pack.skill." + K + "]\nas = " + E + "\ntargets = [" + E + "]\n" +
			"[[app]]\nname = " + E + "\npath = " + E + "\ntargets = [" + E + "]\n[[local]]\nname = " + E + "\ntargets = [" + E + "]\n",
		lPath: "version = 1\n[packs." + K + "]\ncommit = " + E + "\nref = " + E + "\nupstream = [" + E + "]\n" +
			"[packs." + K + ".skills." + K + "]\ncategory = " + E + "\nhash = " + E + "\n" +
			"[links." + K + "]\nmode = " + E + "\nsource = " + E + "\nhash = " + E + "\npath = " + E + "\n",
	}
	for _, path := range []string{mPath, lPath} {
		os.WriteFile(mPath, goodM, 0o644)
		os.WriteFile(lPath, goodL, 0o644)
		os.WriteFile(path, []byte(everywhere[path]), 0o644)
		if _, errOut, code := e.skd(lib, "list"); code == 0 || strings.Count(errOut, "control characters") < 10 {
			t.Errorf("%s hostile everywhere: exit %d, want every place named:\n%s", filepath.Base(path), code, errOut)
		}
		for _, args := range hostileCommands {
			a := expand(args, lib, "x/y")
			out, errOut, code := e.skd(lib, a...)
			gate(t, filepath.Base(path)+": "+strings.Join(args, " "), out+errOut, code)
		}
	}
}

// foreign reports what in a terminal transcript cannot be skd's own: a
// prompt draws with cursor movement and colour (CSI sequences), but
// never sets the title or the clipboard (OSC), rings the bell, clears
// the screen, uses a C1 control or reorders text with a bidi control.
func foreign(out string) string {
	for _, bad := range []string{"\x1b]", "\x07", "\x1b[2J", "pwned\x07"} {
		if strings.Contains(out, bad) {
			return bad
		}
	}
	if i := strings.IndexFunc(out, func(r rune) bool { return r >= 0x80 && r <= 0x9f || safetext.Unsafe(r) && r > 0x7f }); i >= 0 {
		return out[i:min(len(out), i+8)]
	}
	return ""
}

// The prompts a person answers at a terminal show the hostile library's
// names, descriptions and categories cleaned too.
func TestHostilePromptsDrawClean(t *testing.T) {
	e := newEnv(t)
	lib, up := hostileLibrary(t, e)
	// Something to update.
	e.write("up/skills/eng/tdd/again.md", hostile.Evil)
	e.git(up, "add", "-A")
	e.git(up, "commit", "-qm", "again "+hostile.Evil)
	keys := func(k ...string) []typed {
		out := []typed{{wait: 1500 * time.Millisecond}}
		for _, s := range k {
			out = append(out, typed{wait: 400 * time.Millisecond, keys: s})
		}
		return append(out, typed{wait: 400 * time.Millisecond, keys: "\x03"})
	}
	for _, c := range []struct {
		args []string
		keys []typed
	}{
		{[]string{"update"}, keys("j")},
		{[]string{"add", "'" + up + "'"}, keys("\r", "j", "j")}, // pick skills: names, categories, descriptions
		{[]string{"adopt"}, keys("j", "j")},
		{[]string{"edit", "tdd"}, keys("j")},
		{[]string{"purge", "'" + up + "'"}, keys("j")},
		{[]string{"--simple"}, keys("j", "j", "j", "\r", "j")},
	} {
		args := c.args
		// A real size, so the prompts draw everything.
		out := e.interactiveShell(lib, c.keys, "stty rows 50 cols 160; skd "+strings.Join(args, " "))
		if bad := foreign(out); bad != "" {
			t.Errorf("skd %s at a terminal printed %q:\n%q", strings.Join(args, " "), bad, out)
		}
		if testing.Verbose() {
			t.Logf("skd %s:\n%s", strings.Join(args, " "), out)
		}
	}
}
