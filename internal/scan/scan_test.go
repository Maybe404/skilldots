package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/targets"
)

// Records of a target no longer in use are listed as "left" only when
// their path is a link into the library as skd makes them: a forged
// record whose path names the user's own directory (or a link of
// theirs elsewhere) is not listed, so nothing suggests removing it.
func TestLeftListsOnlyVerifiedLinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	root := t.TempDir()
	src := filepath.Join(root, "skills", "s")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: s\n---\n"), 0o644)
	gone := filepath.Join(home, "gone-agent")
	os.MkdirAll(gone, 0o755)
	left := filepath.Join(gone, "s")
	os.Symlink(src, left)
	docs := filepath.Join(home, "Documents")
	os.MkdirAll(docs, 0o755)
	elsewhere := filepath.Join(home, "mylink")
	os.Symlink(home, elsewhere)
	lib := &library.Library{Root: root, Manifest: manifest.New([]string{}),
		Lock: &lock.Lock{Version: 1, Packs: map[string]*lock.Pack{}, Links: map[string]*lock.Link{
			"gone/s":    {Mode: "link", Source: "skills/s", Path: left},
			"evil/docs": {Mode: "link", Source: "skills/s", Path: docs},
			"evil/link": {Mode: "link", Source: "skills/s", Path: elsewhere},
			"evil/copy": {Mode: "copy", Source: "skills/s", Path: docs, Hash: "x"},
		}}}
	reg, err := targets.NewRegistry("", nil)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Run(lib, reg)
	if err != nil {
		t.Fatal(err)
	}
	var got []Item
	for _, it := range rep.Items {
		if it.State == Left {
			got = append(got, it)
		}
	}
	if len(got) != 1 || got[0].Path != left || got[0].Target != "gone" {
		t.Fatalf("left %+v", got)
	}
}
