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

// A field this version does not know (a newer skd's) may hold tabs and
// line breaks; the fields skd reads may not, and nothing may hold an
// escape, C1 or a bidi control.
func TestUnknownLockFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	const head = "version = 1\n[packs.\"x/y\"]\ncommit = \"abcdef1\"\n"
	for _, body := range []string{
		head + "note = \"a\\tb\\nc\"\n",
		"note = \"\"\"\nline\n\tmore\n\"\"\"\n" + head + "[packs.\"x/y\".skills.s]\nhash = \"h\"\nwhy = \"\\r\\n\"\n",
	} {
		os.WriteFile(path, []byte(body), 0o644)
		if _, err := Load(path); err != nil {
			t.Errorf("%q: %v", body, err)
		}
	}
	for place, body := range map[string]string{
		"note":                         head + "note = \"a\\u001b[2J\"\n",
		"packs.\"x/y\".why":            head + "why = \"\\u202e\"\n",
		"packs.\"x/y\".upstream[0]":    head + "upstream = [\"a\\tb\"]\n",
		"key packs.\"x/y\".skills.":    head + "[packs.\"x/y\".skills.\"a\\nb\"]\nhash = \"h\"\n",
		"links.\"agents/a\".source":    "version = 1\n[links.\"agents/a\"]\nmode = \"link\"\nsource = \"s\\t\"\n",
		"key links.":                   "version = 1\n[links.\"agents/a\\t\"]\nmode = \"link\"\nsource = \"s\"\n",
		"packs.\"x/y\".skills.s.extra": head + "[packs.\"x/y\".skills.s]\nhash = \"h\"\nextra = \"\\u009b\"\n",
	} {
		os.WriteFile(path, []byte(body), 0o644)
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), place) {
			t.Errorf("%s: %v", place, err)
		}
	}
}
