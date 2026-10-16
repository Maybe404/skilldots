package cli_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// configPath is the settings file for this env.
func (e *env) configPath() string {
	if runtime.GOOS == "darwin" {
		return e.p("Library/Application Support/skilldots/config.toml")
	}
	return e.p(".config/skilldots/config.toml")
}

// A theme setting skd does not know does not stop any command: it falls
// back to auto with a warning on stderr (JSON stays clean), and
// `skd config set theme` repairs it.
func TestBadThemeSetting(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	b, err := os.ReadFile(e.configPath())
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(e.configPath(), append(b, []byte("theme = \"blue\"\n")...), 0o644)
	out, errOut, code := e.skd("", "list", "--json")
	if code != 0 || !strings.Contains(errOut, `theme setting "blue"`) {
		t.Fatalf("list --json: exit %d\n%s\n%s", code, out, errOut)
	}
	oneJSON(t, []string{"list"}, out)
	if _, errOut, code := e.skd("", "config", "list"); code != 0 || strings.Count(errOut, "theme setting") != 1 {
		t.Fatalf("config list: exit %d, stderr %q", code, errOut)
	}
	e.ok("", "config", "set", "theme", "auto")
	if _, errOut, _ := e.skd("", "list"); strings.Contains(errOut, "theme setting") {
		t.Fatalf("still warns after the fix: %s", errOut)
	}
	if got := strings.TrimSpace(e.ok("", "config", "get", "theme")); got != "auto" {
		t.Fatalf("theme = %q", got)
	}
	// The library setting written by init is still there and top-level.
	if got := strings.TrimSpace(e.ok("", "config", "get", "library")); filepath.Clean(got) != lib {
		t.Fatalf("library = %q", got)
	}
}

// A settings file that is not valid TOML is reported with its path and
// the parse error, once: as the error of a command that needs it (finding
// the library, init, clone, use, config), not taken for "no library", and
// nothing is created first; as one warning after a command that ran
// without it (the library named on the command line).
func TestBrokenSettingsFile(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.git(e.home, "init", "-q", "--bare", e.p("backup.git"))
	os.WriteFile(e.configPath(), []byte("library = \"unterminated\n"), 0o644)
	once := func(args []string, wantCode int, errOut string, code int) {
		t.Helper()
		if (code == 0) != (wantCode == 0) || strings.Count(errOut, e.configPath()) != 1 || !strings.Contains(errOut, "not valid TOML") ||
			strings.Contains(errOut, "no skilldots library found") || strings.Contains(errOut, "ignored") {
			t.Errorf("skd %v: exit %d, want the broken settings file reported once:\n%s", args, code, errOut)
		}
	}
	for _, args := range [][]string{
		{"list"},
		{"init", "--no-skill", e.p("new")},
		{"clone", e.p("backup.git"), e.p("cloned")},
		{"use", lib},
		{"config", "list"},
		{"config", "get", "editor"},
		{"config", "set", "editor", "vim"},
	} {
		_, errOut, code := e.skd("", args...)
		once(args, 1, errOut, code)
		if strings.Contains(errOut, "warning:") {
			t.Errorf("skd %v: a warning besides the error:\n%s", args, errOut)
		}
	}
	for _, p := range []string{"new", "cloned"} {
		if _, err := os.Stat(e.p(p)); err == nil {
			t.Errorf("%s was created though the settings file is broken", p)
		}
	}
	args := []string{"list", "--library", lib}
	out, errOut, code := e.skd("", args...)
	once(args, 0, errOut, code)
	if strings.Count(errOut, "warning:") != 1 || !strings.Contains(errOut, "were used") || !strings.Contains(out, "skilldots") {
		t.Errorf("skd %v: want one warning and the list:\n%s\n%s", args, out, errOut)
	}
}
