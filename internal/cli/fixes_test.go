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
