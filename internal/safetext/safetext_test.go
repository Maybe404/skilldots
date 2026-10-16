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
		{"right‮left⁦x⁩‏؜", 0, "rightleftx"},
		{"one\ntwo\r\nthree four\x7f", 0, "one two  three four"},
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
	if Has("ok 中文") || !Has("a‮b") || !Has("a\nb") || !Has("\xff") {
		t.Error("Has")
	}
}

func TestLines(t *testing.T) {
	in := "git fetch: fatal\r\nremote: \x1b[31mred‮\nlast"
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
