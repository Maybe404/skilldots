package skillmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		// Scalars as written, never resolved to a number, a boolean or null.
		{"float name", "name: 1.0\ndescription: x", "1.0", "x", false},
		{"octal-looking name", "name: 010\ndescription: x", "010", "x", false},
		{"hex-looking name", "name: 0x1F\ndescription: x", "0x1F", "x", false},
		{"exponent name", "name: 1e3\ndescription: x", "1e3", "x", false},
		{"boolean-looking name", "name: True\ndescription: x", "True", "x", false},
		{"null name", "name: null\ndescription: x", "null", "x", false},
		{"tilde name", "name: ~\ndescription: x", "~", "x", false},
		{"date name", "name: 2024-01-02\ndescription: x", "2024-01-02", "x", false},
		{"list name", "name: [a]\ndescription: x", "[a]", "x", false},
		{"quoted empty name", "name: \"\"\ndescription: x", "dir", "x", true},
		{"block name", "name: |\n  foo\ndescription: x", "foo", "x", false},
		{"spaced name", "name: \"  foo  \"\ndescription: x", "foo", "x", false},
		// YAML drops a comment after " #", as agents reading YAML do.
		{"comment", "name: a # the name\ndescription: x", "a", "x", false},
		{"repeated key", "name: a\nname: b\ndescription: x", "b", "x", false},
		// The line reader: a blank line does not end a block value.
		{"not YAML, blank line in block", "name: a: b\ndescription: >\n  One\n\n  two.", "a: b", "One two.", false},
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

// Without a closing "---" the body's indented lines are not read into the
// description.
func TestReadUnclosedFrontmatter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dir")
	os.MkdirAll(dir, 0o755)
	body := "---\nname: a\ndescription: >\n  Short.\n# Body\n\n    indented code\n  more body\n"
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644)
	m, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "a" || m.Description != "" {
		t.Fatalf("got name %q desc %q", m.Name, m.Description)
	}
}

// Upstream frontmatter with thousands of repeated keys, at the top level
// or in a flow mapping, is read in a moment: decoding it into a map used
// to take seconds and gigabytes.
func TestReadRepeatedKeysIsFast(t *testing.T) {
	for name, front := range map[string]string{
		"block": "name: a\n" + strings.Repeat("k: v\n", 6000),
		"flow":  "name: a\nz: {" + strings.Repeat("k: ,", 6000) + "}\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "dir")
			os.MkdirAll(dir, 0o755)
			os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\n"+front+"---\nbody\n"), 0o644)
			start := time.Now()
			m, err := Read(dir)
			if took := time.Since(start); took > 200*time.Millisecond {
				t.Fatalf("took %v", took)
			}
			if err != nil || m.Name != "a" {
				t.Fatalf("%+v %v", m, err)
			}
		})
	}
}
