package termbg

import "testing"

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
