package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skd writes its own files in the library only where they are: not
// through a link that leads outside the library or nowhere (a cloned
// library could hold skills.lock -> ../.codex/config.toml). A link to a
// file inside the library, and a library reached through a link, are
// fine.
func TestCheckFile(t *testing.T) {
	d, _ := filepath.EvalSymlinks(t.TempDir())
	root := filepath.Join(d, "lib")
	os.MkdirAll(filepath.Join(root, "private"), 0o755)
	os.WriteFile(filepath.Join(d, "config.toml"), []byte("precious = 1\n"), 0o644)
	os.WriteFile(filepath.Join(root, "private", "skills.toml"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(root, "plain"), []byte(""), 0o644)
	os.Symlink("../config.toml", filepath.Join(root, "out"))
	os.Symlink(filepath.Join(d, "nowhere"), filepath.Join(root, "dangling"))
	os.Symlink("private/skills.toml", filepath.Join(root, "in"))
	os.Symlink(".", filepath.Join(root, "self"))
	for name, bad := range map[string]bool{"out": true, "dangling": true, "in": false, "plain": false, "absent": false, "self": true} {
		err := CheckFile(root, filepath.Join(root, name))
		if (err != nil) != bad {
			t.Errorf("%s: %v", name, err)
		}
		if err != nil && !strings.Contains(err.Error(), "symbolic link") {
			t.Errorf("%s: %v", name, err)
		}
	}
	link := filepath.Join(d, "lib-link")
	os.Symlink(root, link)
	if err := CheckFile(link, filepath.Join(link, "in")); err != nil {
		t.Errorf("through a linked root: %v", err)
	}
	if err := CheckFile(link, filepath.Join(link, "out")); err == nil {
		t.Error("out through a linked root allowed")
	}
	// Save refuses before writing anything.
	os.Remove(filepath.Join(root, "out"))
	os.Symlink("../config.toml", filepath.Join(root, "skills.lock"))
	os.WriteFile(filepath.Join(root, "skills.toml"), []byte("[targets]\ndefault = []\n"), 0o644)
	l, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Save(); err == nil {
		t.Fatal("saved through a link out of the library")
	}
	if b, _ := os.ReadFile(filepath.Join(d, "config.toml")); string(b) != "precious = 1\n" {
		t.Fatalf("overwritten: %q", b)
	}
}
