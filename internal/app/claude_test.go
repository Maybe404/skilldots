package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/manifest"
)

func TestClaudeDisabled(t *testing.T) {
	up := upstream(t)
	home, root := side(t, up)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd,review", "--agent", "agents,claude")
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

// Claude Code names a skill by the directory it finds, the link name, and
// only sees what is linked into its own directory: a vendored skill
// linked under another name, or one not linked for Claude, is not marked
// because of a same-named override.
func TestClaudeDisabledMatchesWhatClaudeSees(t *testing.T) {
	up := upstream(t)
	home, root := side(t, up)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if err := manifest.New([]string{"agents", "claude"}).Save(filepath.Join(root, manifest.FileName)); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "skills", "tdd", "SKILL.md"), skillMD("tdd"))
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd,review,grill")
	runCLI(t, root, "disable", "review", "--agent", "claude")
	os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{"skillOverrides": {"tdd": "off", "review": "off", "grill": "off"}}`), 0o644)
	s, _ := app.Open(root)
	entries, err := s.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[string(e.Kind)+":"+e.LinkName] = e.ClaudeDisabled
	}
	want := map[string]bool{"skills:tdd": true, "vendor:review": false, "vendor:grill": true}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s marked %v, want %v (all: %v)", k, got[k], v, got)
		}
	}
	for k, v := range got {
		if strings.HasPrefix(k, "vendor:") && k != "vendor:review" && k != "vendor:grill" && v {
			t.Errorf("%s (the vendored tdd, linked under another name) is marked", k)
		}
	}
}
