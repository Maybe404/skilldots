package engine

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/Maybe404/skilldots/internal/targets"
)

// richDir builds a directory with everything a hash-based copy ignores.
func richDir(t *testing.T, dir string) {
	t.Helper()
	writeSkill(t, dir, filepath.Base(dir))
	for p, c := range map[string]string{".git/HEAD": "ref", "node_modules/m/index.js": "js", "sub/notes.txt": "n"} {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755)
		os.WriteFile(filepath.Join(dir, p), []byte(c), 0o644)
	}
	os.Symlink("sub/notes.txt", filepath.Join(dir, "link"))
	os.Chmod(filepath.Join(dir, "sub", "notes.txt"), 0o600)
}

func checkRich(t *testing.T, dir string) {
	t.Helper()
	for _, p := range []string{".git/HEAD", "node_modules/m/index.js", "sub/notes.txt", "SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("%s missing in %s", p, dir)
		}
	}
	if l, err := os.Readlink(filepath.Join(dir, "link")); err != nil || l != "sub/notes.txt" {
		t.Errorf("symlink not preserved: %q %v", l, err)
	}
	if st, err := os.Stat(filepath.Join(dir, "sub", "notes.txt")); err == nil && st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v not preserved", st.Mode().Perm())
	}
}

func TestTrashRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name      string
		renameErr error
		wantErr   bool
	}{
		{"rename", nil, false},
		{"cross-device", &os.LinkError{Op: "rename", Err: syscall.EXDEV}, false},
		{"other error", &os.LinkError{Op: "rename", Err: syscall.EACCES}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lib, _, home := newTestLib(t)
			src := filepath.Join(home, "agent", "skill")
			richDir(t, src)
			if tc.renameErr != nil {
				rename = func(string, string) error { return tc.renameErr }
				defer func() { rename = os.Rename }()
			}
			to, err := Trash(lib, src)
			if tc.wantErr {
				if err == nil || !errors.Is(err, syscall.EACCES) {
					t.Fatalf("want the rename error, got %v", err)
				}
				checkRich(t, src) // untouched
				if m, _ := filepath.Glob(filepath.Join(lib.Root, TrashDir, "*", "skill")); len(m) != 0 {
					t.Fatalf("partial trash entry %v", m)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(src); !os.IsNotExist(err) {
				t.Fatal("source still there")
			}
			checkRich(t, to)
			items, _ := ListTrash(lib)
			if len(items) != 1 || items[0].Origin != src {
				t.Fatalf("trash list %+v", items)
			}
			back, err := Restore(lib, items[0].ID)
			if err != nil || back != src {
				t.Fatalf("restore %q %v", back, err)
			}
			checkRich(t, src)
		})
	}
}

// In copy mode, a copy someone added .git, node_modules or a symlink to
// is moved to the trash whole when skilldots unlinks it.
func TestCopyModeExtrasGoToTrash(t *testing.T) {
	lib, _, home := newTestLib(t)
	reg, err := targets.NewRegistry(map[string]struct{ Path, Mode string }{"box": {Path: filepath.Join(home, "box"), Mode: "copy"}})
	if err != nil {
		t.Fatal(err)
	}
	SetRegistry(reg)
	lib.Manifest.Targets.Default = []string{"box"}
	writeSkill(t, filepath.Join(lib.Root, "skills", "s"), "s")
	entries, _, _ := Resolve(lib, reg)
	if _, err := Apply(lib, MakePlan(lib, entries, false)); err != nil {
		t.Fatal(err)
	}
	cp := filepath.Join(home, "box", "s")
	if l := lib.Lock.Links["box/s"]; l == nil || l.Mode != "copy" {
		t.Fatalf("not copied: %+v", l)
	}
	// Only things the content hash does not see, so the copy still counts
	// as unmodified.
	os.MkdirAll(filepath.Join(cp, ".git"), 0o755)
	os.WriteFile(filepath.Join(cp, ".git", "HEAD"), []byte("ref"), 0o644)
	os.MkdirAll(filepath.Join(cp, "node_modules", "m"), 0o755)
	os.WriteFile(filepath.Join(cp, "node_modules", "m", "index.js"), []byte("js"), 0o644)
	os.Symlink("SKILL.md", filepath.Join(cp, "link"))
	lib.Manifest.Targets.Default = []string{}
	entries, _, _ = Resolve(lib, reg)
	plan := MakePlan(lib, entries, false)
	res, err := Apply(lib, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Backups) != 1 {
		t.Fatalf("backups %v (plan %+v)", res.Backups, plan.ChangeList())
	}
	items, _ := ListTrash(lib)
	if len(items) != 1 {
		t.Fatalf("trash %+v", items)
	}
	for _, p := range []string{".git/HEAD", "node_modules/m/index.js", "SKILL.md"} {
		if _, err := os.Stat(filepath.Join(items[0].Path, p)); err != nil {
			t.Errorf("%s lost", p)
		}
	}
	if l, err := os.Readlink(filepath.Join(items[0].Path, "link")); err != nil || l != "SKILL.md" {
		t.Errorf("symlink lost: %v", err)
	}

	// A pristine copy is simply replaced, nothing trashed.
	lib.Manifest.Targets.Default = []string{"box"}
	entries, _, _ = Resolve(lib, reg)
	Apply(lib, MakePlan(lib, entries, false))
	lib.Manifest.Targets.Default = []string{}
	entries, _, _ = Resolve(lib, reg)
	if res, _ := Apply(lib, MakePlan(lib, entries, false)); len(res.Backups) != 0 {
		t.Fatalf("pristine copy trashed: %v", res.Backups)
	}
}

func TestCopyModeDSStoreDirectoryGoesToTrash(t *testing.T) {
	lib, _, home := newTestLib(t)
	reg, _ := targets.NewRegistry(map[string]struct{ Path, Mode string }{"box": {Path: filepath.Join(home, "box"), Mode: "copy"}})
	SetRegistry(reg)
	lib.Manifest.Targets.Default = []string{"box"}
	writeSkill(t, filepath.Join(lib.Root, "skills", "s"), "s")
	entries, _, _ := Resolve(lib, reg)
	Apply(lib, MakePlan(lib, entries, false))
	cp := filepath.Join(home, "box", "s")
	os.MkdirAll(filepath.Join(cp, ".DS_Store"), 0o755)
	os.WriteFile(filepath.Join(cp, ".DS_Store", "notes.txt"), []byte("n"), 0o644)
	lib.Manifest.Targets.Default = []string{}
	entries, _, _ = Resolve(lib, reg)
	if _, err := Apply(lib, MakePlan(lib, entries, false)); err != nil {
		t.Fatal(err)
	}
	if m, _ := filepath.Glob(filepath.Join(lib.Root, TrashDir, "*", "s", ".DS_Store", "notes.txt")); len(m) != 1 {
		t.Fatal("notes.txt under a .DS_Store directory was deleted on disable")
	}
}
