package lock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An old lock (no upstream list, no paths) and categories in Chinese
// with spaces and emoji load and save as they are.
func TestNormalTextIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	body := `version = 1
[packs."owner/repo"]
commit = "abcdef1234567"
[packs."owner/repo".skills."评审"]
category = "工程 类 🚀"
hash = "h1"
[links."agents/评审"]
mode = "link"
source = "vendor/owner/repo/工程 类 🚀/评审"
`
	os.WriteFile(path, []byte(body), 0o644)
	l, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}
	l, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if l.Packs["owner/repo"].Skills["评审"].Category != "工程 类 🚀" || l.Links["agents/评审"] == nil {
		t.Errorf("text changed: %+v", l.Packs["owner/repo"])
	}
}

func TestControlCharactersRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	body := "version = 1\n[links.\"agents/a\"]\nmode = \"link\"\nsource = \"s\"\npath = \"/x/a\\u009b\"\n"
	os.WriteFile(path, []byte(body), 0o644)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), `links."agents/a".path = "/x/a\u009b"`) || !strings.Contains(err.Error(), "git checkout -- skills.lock") {
		t.Fatalf("Load: %v", err)
	}
	if strings.ContainsRune(err.Error(), '\u009b') {
		t.Errorf("error holds the character: %q", err)
	}
	l := &Lock{Version: 1, Packs: map[string]*Pack{"x\x1b": {Commit: "abcdef1"}}, Links: map[string]*Link{}}
	if err := l.Save(path); err == nil || !strings.Contains(err.Error(), "not saving") {
		t.Errorf("Save: %v", err)
	}
}
