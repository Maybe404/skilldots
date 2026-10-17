package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/manifest"
)

func TestAgentSuspensionPreservesSelectionsAndUnmanagedFiles(t *testing.T) {
	home, root := side(t, t.TempDir())
	write(t, filepath.Join(root, "skills", "mine", "SKILL.md"), skillMD("mine"))
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetDefaultAgents([]string{"agents", "claude"}); err != nil {
		t.Fatal(err)
	}
	unmanaged := filepath.Join(home, ".claude", "skills", "user-owned", "SKILL.md")
	write(t, unmanaged, "do not change")
	if _, err = s.SetAgentEnabled("claude", false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(filepath.Join(home, ".claude", "skills", "mine")); !os.IsNotExist(err) {
		t.Fatalf("managed link remains: %v", err)
	}
	if _, err = os.Stat(filepath.Join(home, ".agents", "skills", "mine", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(unmanaged); err != nil || string(b) != "do not change" {
		t.Fatalf("unmanaged file changed: %q %v", b, err)
	}
	m, err := manifest.Load(filepath.Join(root, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Targets.Default) != 2 || len(m.Targets.Disabled) != 1 {
		t.Fatalf("selections lost: %+v", m.Targets)
	}
	// Reopening is what both the CLI and the desktop do on the next operation.
	s, err = app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(app.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(filepath.Join(home, ".claude", "skills", "mine")); !os.IsNotExist(err) {
		t.Fatal("apply recreated suspended link")
	}
	if _, err = s.SetAgentEnabled("claude", true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(home, ".claude", "skills", "mine", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetAgentEnabled("unknown-agent", false); err == nil {
		t.Fatal("unknown target accepted")
	}
}
