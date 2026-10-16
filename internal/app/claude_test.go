package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
)

func TestClaudeDisabled(t *testing.T) {
	up := upstream(t)
	home, root := side(t, up)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd,review")
	settings := filepath.Join(home, ".claude", "settings.json")
	marked := func() map[string]bool {
		t.Helper()
		s, err := app.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := s.Resolve()
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, e := range entries {
			if e.Kind == engine.KindVendor && e.ClaudeDisabled {
				out[e.Name] = true
			}
		}
		return out
	}
	// No file, an unreadable one, the wrong shape: nothing, no error.
	for _, body := range []string{"", "{not json", `{"skillOverrides": ["tdd"]}`, `{"skillOverrides": {"tdd": "on"}}`} {
		os.Remove(settings)
		if body != "" {
			os.WriteFile(settings, []byte(body), 0o644)
		}
		if m := marked(); len(m) != 0 {
			t.Fatalf("%q: %v", body, m)
		}
	}
	body := `{"model": "x", "skillOverrides": {"tdd": "off", "review": "on", "other": "off"}}`
	os.WriteFile(settings, []byte(body), 0o644)
	before, _ := os.Stat(settings)
	if m := marked(); !m["tdd"] || m["review"] || len(m) != 1 {
		t.Fatalf("marked: %v", m)
	}
	// Read only: skd never writes Claude's settings, even when it links.
	runCLI(t, root, "disable", "tdd")
	runCLI(t, root, "enable", "tdd", "--agent", "claude")
	after, _ := os.Stat(settings)
	if b, _ := os.ReadFile(settings); string(b) != body || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("settings.json was written:\n%s", b)
	}
	// CLAUDE_CONFIG_DIR moves the file.
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if m := marked(); len(m) != 0 {
		t.Fatalf("with CLAUDE_CONFIG_DIR: %v", m)
	}
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"skillOverrides": {"review": "off"}}`), 0o644)
	if m := marked(); !m["review"] || len(m) != 1 {
		t.Fatalf("with CLAUDE_CONFIG_DIR: %v", m)
	}
}
