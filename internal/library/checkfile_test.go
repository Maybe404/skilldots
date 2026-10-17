package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skd writes its own files in the library only where they are: not
// through a link that leads outside the library or nowhere (a cloned
// library could hold skills.lock -> ../.codex/config.toml), nor to any
// file but a plain one directly in the library's folder (skills.toml ->
// .git/config would have skd rewrite the library's git settings). Such
// a link, and a library reached through a link, are fine.
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
	os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0o755)
	os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[core]\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".gitattributes"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(root, "skills.history.jsonl"), []byte(""), 0o644)
	os.Symlink(".git/config", filepath.Join(root, "gitconfig"))
	os.Symlink(".git/hooks/pre-commit", filepath.Join(root, "hook"))
	os.Symlink(".gitattributes", filepath.Join(root, "attrs"))
	os.Symlink("skills.history.jsonl", filepath.Join(root, "own"))
	os.Symlink("private", filepath.Join(root, "dir"))
	os.Symlink(filepath.Join(root, "plain"), filepath.Join(root, "sibling"))
	for name, bad := range map[string]bool{"out": true, "dangling": true, "in": true, "plain": false, "absent": false, "self": true,
		"gitconfig": true, "hook": true, "attrs": true, "own": true, "dir": true, "sibling": false} {
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
	if err := CheckFile(link, filepath.Join(link, "sibling")); err != nil {
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

// skills.toml -> .git/config: the link stays inside the library, but
// Save would rewrite the library's git settings with the manifest.
func TestSaveRefusesALinkIntoGit(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	cfg := "[targets]\ndefault = []\n"
	os.WriteFile(filepath.Join(root, ".git", "config"), []byte(cfg), 0o644)
	os.Symlink(".git/config", filepath.Join(root, "skills.toml"))
	l, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	l.Manifest.Targets.Default = []string{"claude"}
	if err := l.Save(); err == nil {
		t.Fatal("saved through skills.toml -> .git/config")
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".git", "config")); string(b) != cfg {
		t.Fatalf(".git/config rewritten: %q", b)
	}
}
