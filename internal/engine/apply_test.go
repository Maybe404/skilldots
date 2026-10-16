package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/targets"
)

func writeSkill(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestForceApplyBacksUpUnmanagedDir(t *testing.T) {
	root := t.TempDir()
	lib := &library.Library{Root: root, Manifest: manifest.New([]string{"agents"}),
		Lock: &lock.Lock{Version: 1, Packs: map[string]*lock.Pack{}, Links: map[string]*lock.Link{}}}
	writeSkill(t, filepath.Join(root, "skills", "mine"), "mine")

	home := t.TempDir()
	t.Setenv("HOME", home)
	reg, err := targets.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	SetRegistry(reg)
	agents := filepath.Join(home, ".agents", "skills")
	stale := filepath.Join(agents, "mine")
	writeSkill(t, stale, "mine")
	os.WriteFile(filepath.Join(stale, "notes.txt"), []byte("user data"), 0o644)

	entries, err := Resolve(lib, reg)
	if err != nil {
		t.Fatal(err)
	}
	plan := MakePlan(lib, entries, true)
	res, err := Apply(lib, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Backups) != 1 {
		t.Fatalf("expected one backup, got %v", res.Backups)
	}
	if st, err := os.Lstat(stale); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s should now be a symlink", stale)
	}
	matches, _ := filepath.Glob(filepath.Join(agents, BackupDir, "*", "mine", "notes.txt"))
	if len(matches) != 1 {
		t.Fatalf("user data not preserved in backup: %v", matches)
	}

	// A second apply replaces nothing and backs up nothing.
	entries, _ = Resolve(lib, reg)
	plan = MakePlan(lib, entries, true)
	if plan.Changes() {
		t.Fatalf("expected no changes, got %+v", plan.Counts())
	}
}
