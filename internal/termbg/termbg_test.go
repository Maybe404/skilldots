package termbg

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in    string
		light bool
		ok    bool
	}{
		{"\x1b]11;rgb:ffff/ffff/ffff\x1b\\\x1b[?62;22c", true, true},
		{"\x1b]11;rgb:0000/0000/0000\x07\x1b[?1;2c", false, true},
		{"\x1b]11;rgb:1a1a/1b1b/2626\x1b\\", false, true},
		{"\x1b]11;rgb:fd/f6/e3\x07", true, true}, // solarized light, 2 digits
		{"\x1b]11;rgba:ffff/ffff/ffff/ffff\x07", true, true},
		{"\x1b[?62;22c", false, false}, // DA1 only: OSC 11 unsupported
		{"\x1b]11;rgb:zz/00/00\x07", false, false},
		{"", false, false},
	}
	for _, c := range cases {
		r, g, b, ok := parse(c.in)
		if ok != c.ok {
			t.Errorf("%q: ok = %v", c.in, ok)
			continue
		}
		if ok && (luminance(r, g, b) > 0.5) != c.light {
			t.Errorf("%q: light = %v", c.in, !c.light)
		}
	}
}

func TestSplit(t *testing.T) {
	osc := "\x1b]11;rgb:0/0/0\x07"
	st := "\x1b]11;rgb:ffff/ffff/ffff\x1b\\"
	da1 := "\x1b[?62;22c"
	for _, c := range []struct {
		in      string
		final   bool
		replies string
		keys    string
		da1     bool
	}{
		{osc + da1, false, osc + da1, "", true},
		{st + da1, false, st + da1, "", true},
		{da1, false, da1, "", true},
		{osc + "\x1b[?62;2", false, osc, "", false}, // DA1 still arriving
		{osc, false, osc, "", false},
		// Keys typed while waiting, before, between and after the replies.
		{"?" + osc + "j" + da1 + "q", false, osc + da1, "?jq", true},
		{"\x1b[A?", true, "", "\x1b[A?", false}, // arrow key: not a reply
		{"? ", true, "", "? ", false},
		// An unfinished sequence at the end: a reply in progress unless the
		// wait is over and it cannot be one.
		{"?\x1b", false, "", "?", false},
		{"?\x1b", true, "", "?\x1b", false},
		{"?\x1b[", true, "", "?\x1b[", false},
		{"?\x1b]11;rgb:00", true, "", "?", false},
	} {
		replies, keys, d := split([]byte(c.in), c.final)
		if replies != c.replies || string(keys) != c.keys || d != c.da1 {
			t.Errorf("split(%q, %v) = %q, %q, %v; want %q, %q, %v", c.in, c.final, replies, keys, d, c.replies, c.keys, c.da1)
		}
	}
}

// reads feeds gather one chunk per read; nil stands for the wait being
// cancelled. grace is counted.
func reads(chunks ...[]byte) (func([]byte) (int, error), *int) {
	graces := 0
	return func(b []byte) (int, error) {
		if len(chunks) == 0 || chunks[0] == nil {
			return 0, errors.New("canceled")
		}
		n := copy(b, chunks[0])
		chunks = chunks[1:]
		return n, nil
	}, &graces
}

func TestGatherKeyAfterAnswer(t *testing.T) {
	osc := []byte("\x1b]11;rgb:0/0/0\x07")
	da1 := []byte("\x1b[?62;22c")
	cat := func(bs ...[]byte) []byte { return bytes.Join(bs, nil) }
	for _, c := range []struct {
		name   string
		chunks [][]byte
		keys   string
		graces int
	}{
		{"answer only", [][]byte{cat(osc, da1)}, "", 0},
		{"arrow in the same read", [][]byte{cat(osc, da1, []byte("\x1b[B"))}, "\x1b[B", 0},
		// Esc pressed right after the answer: nothing more comes.
		{"lone esc", [][]byte{cat(osc, da1, []byte("\x1b")), nil}, "\x1b", 1},
		// An arrow key whose bytes were read in two parts.
		{"split arrow", [][]byte{cat(osc, da1, []byte("\x1b")), []byte("[B")}, "\x1b[B", 1},
		{"split csi", [][]byte{cat(osc, da1, []byte("\x1b[")), []byte("5~")}, "\x1b[5~", 1},
		{"split ss3", [][]byte{cat(osc, da1, []byte("j\x1bO")), []byte("B")}, "j\x1bOB", 1},
	} {
		read, _ := reads(c.chunks...)
		graces := 0
		reply, keys, ok := gather(read, func() { graces++ })
		if !ok || !strings.Contains(reply, "]11;") || string(keys) != c.keys || graces != c.graces {
			t.Errorf("%s: gather = %q, %q, %v (graces %d); want keys %q, graces %d", c.name, reply, keys, ok, graces, c.keys, c.graces)
		}
	}
}
