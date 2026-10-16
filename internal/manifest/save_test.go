package manifest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := New([]string{"agents"}).Save(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	m := New([]string{"claude"})
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// A rename puts a new file in place; writing in place would keep the
	// same one, and a crash half-way would leave it torn.
	if os.SameFile(before, after) {
		t.Error("Save wrote over the file in place instead of renaming a new one over it")
	}
	if runtime.GOOS != "windows" && after.Mode().Perm() != 0o600 {
		t.Errorf("permissions not kept: %v", after.Mode().Perm())
	}
	got, err := Load(path)
	if err != nil || len(got.Targets.Default) != 1 || got.Targets.Default[0] != "claude" {
		t.Fatalf("reload: %+v %v", got, err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}

func TestSaveThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", FileName)
	os.MkdirAll(filepath.Dir(real), 0o755)
	if err := New([]string{"agents"}).Save(real); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, FileName)
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := New([]string{"claude"}).Save(link); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Lstat(link); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a regular file")
	}
	got, err := Load(real)
	if err != nil || got.Targets.Default[0] != "claude" {
		t.Fatalf("target not updated: %+v %v", got, err)
	}
}

func TestSaveFailureKeepsOldFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs a read-only directory")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := New([]string{"agents"}).Save(path); err != nil {
		t.Fatal(err)
	}
	os.Chmod(dir, 0o555)
	defer os.Chmod(dir, 0o755)
	if err := New([]string{"claude"}).Save(path); err == nil {
		t.Fatal("save into a read-only directory succeeded")
	}
	got, err := Load(path)
	if err != nil || got.Targets.Default[0] != "agents" {
		t.Fatalf("old manifest damaged: %+v %v", got, err)
	}
}

// A read-only skills.toml in a writable directory stays as it is: a
// rename would otherwise replace it regardless of its own mode, also
// through a symlink.
func TestSaveRefusesReadOnlyFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs file permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := New([]string{"agents"}).Save(path); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.toml")
	os.Symlink(path, link)
	before, _ := os.ReadFile(path)
	os.Chmod(path, 0o444)
	for _, p := range []string{path, link} {
		err := New([]string{"claude"}).Save(p)
		if err == nil || !errors.Is(err, fs.ErrPermission) {
			t.Errorf("save to read-only %s: %v", p, err)
		}
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("read-only skills.toml was replaced")
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o444 {
		t.Errorf("mode changed: %v", st.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}
