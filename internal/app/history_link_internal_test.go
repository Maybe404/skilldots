package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// A cloned library's history file may be a link to one of the user's
// files (a shell's startup file, where a recorded commit subject would
// run): skd does not append to it, nor rewrite it.
func TestHistoryIsNotWrittenThroughALinkOutOfTheLibrary(t *testing.T) {
	d, _ := filepath.EvalSymlinks(t.TempDir())
	root := filepath.Join(d, "lib")
	os.MkdirAll(root, 0o755)
	rc := filepath.Join(d, ".bashrc")
	os.WriteFile(rc, []byte("export X=1\n{\"applyError\": \"x\", \"repo\": \"a\"}\n"), 0o644)
	os.Symlink("../.bashrc", filepath.Join(root, HistoryFile))
	s := &Session{Lib: &library.Library{Root: root, Manifest: manifest.New(nil),
		Lock: &lock.Lock{Version: 1, Packs: map[string]*lock.Pack{}, Links: map[string]*lock.Link{}}}}
	if err := s.appendHistory([]*UpdateRecord{{Repo: "a", Changed: []string{"$(touch pwned)"}}}); err == nil {
		t.Error("appended through the link")
	}
	s.settleApplyErrors(map[string]bool{})
	if b, _ := os.ReadFile(rc); string(b) != "export X=1\n{\"applyError\": \"x\", \"repo\": \"a\"}\n" {
		t.Fatalf("changed:\n%s", b)
	}
}
