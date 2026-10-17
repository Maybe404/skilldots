package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Text that only looks unusual (Chinese, emoji, spaces) is not taken for
// control characters anywhere in the manifest, and it survives a save.
func TestNormalTextIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	body := `[targets]
default = ["agents", "我的 agent"]
[targets.custom."我的 agent"]
path = "/Users/张三/My Skills 🚀"
mode = "copy"

[[pack]]
repo = "owner/repo"
alias = "中文 别名 🚀"
use = ["tdd", "工程/评审"]
skip = ["eng/*"]
targets = ["我的 agent"]

[pack.skill."评审"]
as = "review-zh"
targets = ["agents"]

[[app]]
name = "surge"
path = "/Applications/Surge 5.app/Contents/Resources/Skills/surge"

[[local]]
name = "mine"
targets = []
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	m2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if m2.Packs[0].Alias != "中文 别名 🚀" || m2.Targets.Custom["我的 agent"].Path != "/Users/张三/My Skills 🚀" || m2.Packs[0].Skill["评审"].As != "review-zh" {
		t.Errorf("text changed: %+v", m2.Packs[0])
	}
	if err := m2.CheckAlias("别名 🚀", ""); err != nil {
		t.Errorf("CheckAlias: %v", err)
	}
}

// Every value, list element and key is checked, and the refusal names
// each place without printing what it holds.
func TestControlCharactersRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	body := "[[pack]]\nrepo = \"x/y\"\nuse = [\"ok\", \"t\\u001b[2J\"]\n[pack.skill.\"a\\u202e\"]\nas = \"b\\u0007\"\n"
	os.WriteFile(path, []byte(body), 0o644)
	_, err := Load(path)
	if err == nil {
		t.Fatal("loaded")
	}
	for _, want := range []string{`pack[0].use[1] = "t\x1b[2J"`, `key pack[0].skill."a\u202e"`, `pack[0].skill."a\u202e".as = "b\a"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %s:\n%v", want, err)
		}
	}
	if strings.ContainsAny(err.Error(), "\x1b\x07\u202e") {
		t.Errorf("error holds the characters: %q", err)
	}
	// Nor is such a manifest ever written.
	m := New([]string{"agents"})
	m.Packs = append(m.Packs, &Pack{Repo: "x/y", Use: []string{"t\x1b[2J"}})
	if err := m.Save(path); err == nil || !strings.Contains(err.Error(), "not saving") {
		t.Errorf("Save: %v", err)
	}
	if err := m.CheckAlias("a\x1b", ""); err == nil {
		t.Error("CheckAlias accepts control characters")
	}
}

// What skd reads is held to "no control characters at all"; a table or
// key it does not know (someone's notes) may hold tabs and line breaks,
// but never an escape, BEL, DEL, C1, a bidi control or invalid UTF-8.
func TestUnknownPlacesMayHoldNotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	const head = "[targets]\ndefault = [\"agents\"]\n"
	for what, body := range map[string]string{
		"multi-line note":                 head + "[notes]\ntext = \"\"\"\nline one\n\tindented\r\nline three\n\"\"\"\n",
		"tab in a top-level unknown key":  "\"a\\tb\" = 1\n" + head,
		"tab in an unknown pack field":    head + "[[pack]]\nrepo = \"x/y\"\nuse = []\nnote = \"a\\tb\"\n",
		"line break in a skill's field":   head + "[[pack]]\nrepo = \"x/y\"\nuse = []\n[pack.skill.tdd]\nwhy = \"a\\nb\\r\\n\"\n",
		"tab in an unknown custom field":  head + "[targets.custom.my]\npath = \"/x\"\ncomment = \"\\t\"\n",
		"array of notes":                  head + "[notes]\nlist = [\"a\\tb\", \"c\\nd\"]\n",
		"unknown key under unknown table": head + "[notes.\"x\\ty\"]\nv = 1\n",
	} {
		os.WriteFile(path, []byte(body), 0o644)
		if _, err := Load(path); err != nil {
			t.Errorf("%s: %v", what, err)
		}
	}
	for what, c := range map[string]struct{ body, place string }{
		"escape in a note":            {head + "[notes]\ntext = \"a\\u001b[2Jb\"\n", "notes.text"},
		"BEL in a note":               {head + "[notes]\ntext = \"a\\u0007\"\n", "notes.text"},
		"C1 in a note":                {head + "[notes]\ntext = \"a\\u009b31m\"\n", "notes.text"},
		"bidi in a note":              {head + "[notes]\ntext = \"a\\u202eb\"\n", "notes.text"},
		"DEL in a note":               {"notes = \"a\\u007f\"\n" + head, "notes"},
		"vertical tab in a note":      {"notes = \"a\\u000b\"\n" + head, "notes"},
		"escape in an unknown key":    {"\"k\\u001b\" = 1\n" + head, "key "},
		"escape in a note array":      {head + "[notes]\nlist = [\"ok\", \"\\u001b]0;t\\u0007\"]\n", "notes.list[1]"},
		"tab in a selected skill":     {head + "[[pack]]\nrepo = \"x/y\"\nuse = [\"a\\tb\"]\n", "pack[0].use[0]"},
		"line break in a skill key":   {head + "[[pack]]\nrepo = \"x/y\"\nuse = []\n[pack.skill.\"a\\nb\"]\nas = \"z\"\n", "key pack[0].skill."},
		"tab in a custom target name": {head + "[targets.custom.\"a\\tb\"]\npath = \"/x\"\n", "key targets.custom."},
		"CR in an app path":           {head + "[[app]]\nname = \"a\"\npath = \"/x\\r\"\n", "app[0].path"},
		"tab in a target":             {"[targets]\ndefault = [\"agents\\t\"]\n", "targets.default[0]"},
		"line break in an alias":      {head + "[[pack]]\nrepo = \"x/y\"\nalias = \"a\\n\"\nuse = []\n", "pack[0].alias"},
	} {
		os.WriteFile(path, []byte(c.body), 0o644)
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), c.place) || !strings.Contains(err.Error(), "control characters") {
			t.Errorf("%s: %v, want a refusal naming %s", what, err, c.place)
		}
	}
	// Invalid UTF-8 never loads.
	os.WriteFile(path, []byte(head+"[notes]\ntext = \"a\xffb\"\n"), 0o644)
	if _, err := Load(path); err == nil {
		t.Error("invalid UTF-8 in a note loaded")
	}
}
