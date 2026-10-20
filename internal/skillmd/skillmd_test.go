package skillmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetName(t *testing.T) {
	cases := map[string]string{
		"---\nname: up\ndescription: d\n---\nbody name: x\n": "---\nname: new\ndescription: d\n---\nbody name: x\n",
		"---\ndescription: d\n---\nbody\n":                   "---\nname: new\ndescription: d\n---\nbody\n",
		"just text\n":                                        "---\nname: new\n---\njust text\n",
	}
	for in, want := range cases {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(in), 0o644)
		if err := SetName(dir, "new"); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
		if string(b) != want {
			t.Errorf("in %q:\n got %q\nwant %q", in, b, want)
		}
		if m, _ := Read(dir); m.Name != "new" {
			t.Errorf("Read after SetName: %q", m.Name)
		}
	}
}

func TestRead(t *testing.T) {
	cases := []struct {
		name, front   string
		wantName      string
		wantDesc      string
		wantEmptyName bool
	}{
		{"plain", "name: a\ndescription: Does a thing.", "a", "Does a thing.", false},
		{"folded", "name: a\ndescription: >\n  Builds pages\n  from Markdown.\nlicense: MIT", "a", "Builds pages from Markdown.", false},
		{"literal", "name: a\ndescription: |-\n  Line one.\n  Line two.", "a", "Line one. Line two.", false},
		{"plain multi-line", "name: a\ndescription:\n  Composition patterns\n  for React.", "a", "Composition patterns for React.", false},
		{"quoted with a colon", "name: a\ndescription: \"Use when: the user asks.\"", "a", "Use when: the user asks.", false},
		// Not YAML (": " in a plain scalar): read line by line, as before.
		{"not YAML", "name: a\ndescription: Use when: the user asks.", "a", "Use when: the user asks.", false},
		{"not YAML, folded", "name: a: b\ndescription: >\n  Two\n  lines.", "a: b", "Two lines.", false},
		{"empty name", "name:\ndescription: x", "dir", "x", true},
		{"number name", "name: 2048\ndescription: x", "2048", "x", false},
		{"list description", "name: a\ndescription: [x, y]", "a", "", false},
		{"crlf", "name: a\r\ndescription: >\r\n  One\r\n  two.", "a", "One two.", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "dir")
			os.MkdirAll(dir, 0o755)
			body := "---\n" + c.front + "\n---\n# Body\n"
			if c.name == "crlf" {
				body = "---\r\n" + c.front + "\r\n---\r\n# Body\r\n"
			}
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			m, err := Read(dir)
			if err != nil {
				t.Fatal(err)
			}
			if m.Name != c.wantName || m.Description != c.wantDesc || m.EmptyName != c.wantEmptyName {
				t.Fatalf("got name %q desc %q empty %v; want %q %q %v", m.Name, m.Description, m.EmptyName, c.wantName, c.wantDesc, c.wantEmptyName)
			}
		})
	}
}
