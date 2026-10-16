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

func TestDA1Done(t *testing.T) {
	for s, want := range map[string]bool{
		"\x1b]11;rgb:0/0/0\x07\x1b[?62;22c": true,
		"\x1b[?1;2c":                        true,
		"\x1b]11;rgb:0/0/0\x07\x1b[?62;2":   false,
		"\x1b]11;rgb:0/0/0\x07":             false,
	} {
		if got := da1Done(s); got != want {
			t.Errorf("%q: %v", s, got)
		}
	}
}
