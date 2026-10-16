package safetext

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	for _, c := range []struct {
		in   string
		max  int
		want string
	}{
		{"plain 中文 👍🏽", 0, "plain 中文 👍🏽"},
		{"a\x1b[2Jb\tc\u009bd\xffe", 0, "a[2Jb cd�e"},
		{"title\x1b]0;pwned\x07!", 0, "title]0;pwned!"},
		{"right\u202eleft\u2066x\u2069\u200f\u061c", 0, "rightleftx"},
		{"one\ntwo\r\nthree\u2028four\x7f", 0, "one two  three four"},
		{"abcdef", 3, "abc…"},
		{"ab  cdef", 3, "ab…"},
		{"abc", 3, "abc"},
	} {
		if got := Clean(c.in, c.max); got != c.want {
			t.Errorf("Clean(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
		if Has(Clean(c.in, c.max)) {
			t.Errorf("Has(Clean(%q)) is true", c.in)
		}
	}
	if Has("ok 中文") || !Has("a\u202eb") || !Has("a\nb") || !Has("\xff") {
		t.Error("Has")
	}
}

func TestLines(t *testing.T) {
	in := "git fetch: fatal\r\nremote: \x1b[31mred\u202e\nlast"
	if got, want := Lines(in), "git fetch: fatal\nremote: [31mred\nlast"; got != want {
		t.Errorf("Lines = %q, want %q", got, want)
	}
	if s := "two\nlines"; Lines(s) != s {
		t.Error("clean text changed")
	}
	if strings.ContainsRune(Lines("a\tb"), '\t') {
		t.Error("tab kept")
	}
}

func TestScreen(t *testing.T) {
	const evil = "\x1b]0;t\x07\x1b[2J\x1b]52;c;eA==\x07\u009b6n\u202eok\x00\x7f\r\t"
	for _, c := range []struct {
		in   string
		sgr  bool
		want string
	}{
		{"plain\n中文 👍🏽\n", true, "plain\n中文 👍🏽\n"},
		{"\x1b[1;38;2;1;2;3mbold\x1b[0m\x1b[m", true, "\x1b[1;38;2;1;2;3mbold\x1b[0m\x1b[m"},
		{"\x1b[1mbold\x1b[0m", false, "[1mbold[0m"},
		{"a" + evil + "b\n", true, "a]0;t[2J]52;c;eA==6nok  b\n"},
		{"\x1b[2J\x1b[31", true, "[2J[31"},
		{"bad\xffbyte", true, "bad�byte"},
	} {
		got := Screen(c.in, c.sgr)
		if got != c.want {
			t.Errorf("Screen(%q, %v) = %q, want %q", c.in, c.sgr, got, c.want)
		}
		rest := got
		if c.sgr {
			for {
				i := strings.Index(rest, "\x1b[")
				if i < 0 {
					break
				}
				rest = rest[:i] + rest[i+sgrLen(rest[i:]):]
			}
		}
		if Has(strings.ReplaceAll(rest, "\n", "")) {
			t.Errorf("Screen(%q) left %q", c.in, rest)
		}
	}
}

func TestDoc(t *testing.T) {
	doc := map[string]any{
		"targets": map[string]any{"default": []any{"agents", "a\x1b[2J"}},
		"pack": []any{
			map[string]any{"repo": "ok/中文 skills 👍", "use": []any{"tdd", "x\u202e"},
				"skill": map[string]any{"b\u009b": map[string]any{"as": "fine"}}},
		},
		"n": int64(3),
	}
	got := strings.Join(Doc(doc), "\n")
	want := strings.Join([]string{
		`key pack[0].skill."b\u009b"`,
		`pack[0].use[1] = "x\u202e"`,
		`targets.default[1] = "a\x1b[2J"`,
	}, "\n")
	if got != want {
		t.Errorf("Doc =\n%s\nwant\n%s", got, want)
	}
	if Has(strings.ReplaceAll(got, "\n", "")) {
		t.Error("Doc's report holds control characters")
	}
	if d := Doc(map[string]any{"a": []any{"中文 描述", "emoji 🚀", "space and-dash"}}); d != nil {
		t.Errorf("normal text reported: %v", d)
	}
}

// Known places are strict; the rest only refuses what is never text.
func TestDocSchema(t *testing.T) {
	doc := map[string]any{
		"pack": []any{map[string]any{
			"use":   []any{"a\tb"},
			"note":  "x\ty\nz\r",
			"bad":   "e\x1b[8m",
			"skill": map[string]any{"s\t": map[string]any{"as": "a\n", "why": "w\n"}},
		}},
		"notes\t": map[string]any{"k\n": "v\t", "b\u202e": "ok"},
		"del":     "\x7f",
	}
	got := strings.Join(DocSchema(doc, []string{"pack.use", "pack.skill.*", "pack.skill.*.as"}), "\n")
	want := strings.Join([]string{
		`del = "\x7f"`,
		`key "notes\t"."b\u202e"`,
		`pack[0].bad = "e\x1b[8m"`,
		`key pack[0].skill."s\t"`,
		`pack[0].skill."s\t".as = "a\n"`,
		`pack[0].use[0] = "a\tb"`,
	}, "\n")
	if got != want {
		t.Errorf("DocSchema =\n%s\nwant\n%s", got, want)
	}
	for s, want := range map[string]bool{"a\tb\r\n": false, "中文 🚀": false, "\x1b": true, "\x07": true, "\x7f": true, "\u0085": true, "‮": true, "\x0b": true, "\xff": true} {
		if Dangerous(s) != want {
			t.Errorf("Dangerous(%q) = %v", s, !want)
		}
	}
}
