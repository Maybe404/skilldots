package lock

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
	l := &Lock{Version: 1, Packs: map[string]*Pack{}, Links: map[string]*Link{}}
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	l.Packs["a/b"] = &Pack{Commit: "abc"}
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("Save wrote over the file in place instead of renaming a new one over it")
	}
	got, err := Load(path)
	if err != nil || got.Packs["a/b"] == nil || got.Packs["a/b"].Commit != "abc" {
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
	real := filepath.Join(dir, "elsewhere", FileName)
	os.MkdirAll(filepath.Dir(real), 0o755)
	l := &Lock{Version: 1, Packs: map[string]*Pack{}, Links: map[string]*Link{}}
	if err := l.Save(real); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, FileName)
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	l.Version = 2
	if err := l.Save(link); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Lstat(link); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a regular file")
	}
	if got, err := Load(real); err != nil || got.Version != 2 {
		t.Fatalf("target not updated: %+v %v", got, err)
	}
}

func TestSaveRefusesReadOnlyFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs file permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	l := &Lock{Version: 1}
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	os.Chmod(path, 0o444)
	if err := (&Lock{Version: 2}).Save(path); err == nil || !errors.Is(err, fs.ErrPermission) {
		t.Errorf("save to read-only lock: %v", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("read-only skills.lock was replaced")
	}
}
