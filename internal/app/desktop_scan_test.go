package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/scan"
)

// Before any library exists, the skills in the agents' directories are
// all unmanaged, and counting them changes nothing.
func TestScanWithoutLibrary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SKILLDOTS_LIBRARY", "")
	claude := filepath.Join(home, ".claude", "skills")
	write(t, filepath.Join(claude, "mine", "SKILL.md"), skillMD("mine"))
	write(t, filepath.Join(claude, "notes", "README.md"), "not a skill\n")
	elsewhere := filepath.Join(home, "elsewhere", "linked")
	write(t, filepath.Join(elsewhere, "SKILL.md"), skillMD("linked"))
	if err := os.Symlink(elsewhere, filepath.Join(claude, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "gone"), filepath.Join(claude, "broken")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(home, ".agents", "skills", "shared", "SKILL.md"), skillMD("shared"))

	rep, err := app.ScanWithoutLibrary()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]scan.State{}
	for _, it := range rep.Unmanaged() {
		got[it.Target+"/"+it.Name] = it.State
	}
	want := map[string]scan.State{"claude/mine": scan.Copy, "claude/linked": scan.ExtLink, "claude/broken": scan.Broken, "agents/shared": scan.Copy}
	if len(got) != len(want) {
		t.Fatalf("unmanaged %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: %v, want %v (all: %v)", k, got[k], v, got)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".skilldots")); !os.IsNotExist(err) {
		t.Fatal("scanning made a library")
	}
	if _, err := os.Lstat(filepath.Join(claude, "broken")); err != nil {
		t.Fatal("scanning removed the broken link")
	}
}
