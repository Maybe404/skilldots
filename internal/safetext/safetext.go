// Package safetext makes text from outside skd (upstream SKILL.md files,
// directory names, library files, git output) safe to print on a
// terminal: nothing in it can move the cursor, rewrite the screen, change
// the window title or reorder what is shown.
package safetext

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Unsafe reports whether r must never reach a terminal: a control
// character (C0, DEL, C1) or a bidi control that reorders the text around
// it.
func Unsafe(r rune) bool {
	return unicode.IsControl(r) || bidi(r)
}

func bidi(r rune) bool {
	return r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069 || r == 0x200E || r == 0x200F || r == 0x061C
}

// space are the controls that stand for white space: they become one.
func space(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' || r == 0x2028 || r == 0x2029
}

// Has reports whether s holds anything Clean would change apart from its
// length: an unsafe character or invalid UTF-8.
func Has(s string) bool {
	return !utf8.ValidString(s) || strings.ContainsFunc(s, func(r rune) bool { return Unsafe(r) || space(r) && r != ' ' })
}

// Clean is s fit to print on one terminal line: tabs, line breaks and the
// Unicode line separators become spaces, other control characters (C0,
// DEL, C1) and bidi controls are dropped, invalid UTF-8 becomes U+FFFD,
// and with max > 0 the result is cut to max characters with "…".
func Clean(s string, max int) string {
	if max <= 0 && !Has(s) {
		return s
	}
	var b strings.Builder
	n := 0
	for _, r := range strings.ToValidUTF8(s, "�") {
		switch {
		case space(r):
			r = ' '
		case Unsafe(r):
			continue
		}
		if max > 0 && n == max {
			return strings.TrimRight(b.String(), " ") + "…"
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// Lines is Clean without a length limit for text of several lines (an
// error with git's output in it): line breaks are kept, a CR LF becomes
// one line break, and each line is cleaned.
func Lines(s string) string {
	if !Has(strings.ReplaceAll(s, "\n", "")) {
		return s
	}
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = Clean(l, 0)
	}
	return strings.Join(lines, "\n")
}

// Screen is text on its way to a terminal, cleaned as a last line of
// defence: line breaks stay, colour codes stay when sgr is set (only
// those that cannot hide text, see Screener), and everything else is as
// Clean leaves it: no other escape sequence can start, so nothing can
// move the cursor, clear the screen, set the window title or write the
// clipboard. Text from outside skd is cleaned where it is read or
// printed; this only catches what was missed.
func Screen(s string, sgr bool) string { return new(Screener).Screen(s, sgr) }

// Screener is Screen for a stream: it remembers the colours set so far,
// so a foreground and a background set in two writes are compared too.
// Its zero value is ready; it is not safe for concurrent use.
type Screener struct{ fg, bg string }

// Screen cleans s like the function Screen, keeping an SGR sequence only
// when every parameter in it is one skd draws with: reset, bold, faint,
// italic, underline (any style), reverse (a text input's cursor) and
// their ends, and foreground or background colours (16, 256 or RGB) and
// their defaults; and only when it does not leave the foreground and the
// background the same colour. Conceal, blink and the rest are dropped
// whole: with them a foreign sequence that got this far could hide the
// text that follows.
func (sc *Screener) Screen(s string, sgr bool) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r != '\n' && (Unsafe(r) || space(r)) }) && utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	s = strings.ToValidUTF8(s, "\uFFFD")
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '\x1b' && sgr:
			if m := sgrLen(s[i:]); m > 0 {
				if fg, bg, ok := sgrApply(s[i+2:i+m-1], sc.fg, sc.bg); ok {
					sc.fg, sc.bg = fg, bg
					b.WriteString(s[i : i+m])
				}
				i += m
				continue
			}
		case r == '\n':
			b.WriteByte('\n')
		case space(r):
			b.WriteByte(' ')
		case !Unsafe(r):
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}

// sgrApply applies the parameters of one SGR sequence to the colours fg
// and bg ("" for the terminal's default) and reports whether the
// sequence may be shown (see Screener.Screen).
func sgrApply(params, fg, bg string) (string, string, bool) {
	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		sub := strings.Split(ps[i], ":")
		n, err := strconv.Atoi(sub[0])
		if sub[0] == "" {
			n, err = 0, nil
		}
		if err != nil {
			return fg, bg, false
		}
		switch {
		case n == 0:
			fg, bg = "", ""
		case n == 1 || n == 2 || n == 3 || n == 4 || n == 7 || n >= 21 && n <= 24 || n == 27:
		case n == 39:
			fg = ""
		case n == 49:
			bg = ""
		case n >= 30 && n <= 37:
			fg = "c" + strconv.Itoa(n-30)
		case n >= 90 && n <= 97:
			fg = "c" + strconv.Itoa(n-90+8)
		case n >= 40 && n <= 47:
			bg = "c" + strconv.Itoa(n-40)
		case n >= 100 && n <= 107:
			bg = "c" + strconv.Itoa(n-100+8)
		case n == 38 || n == 48:
			var c string
			var ok bool
			if len(sub) > 1 {
				c, ok = sgrColor(sub[1:])
			} else {
				var used int
				c, used, ok = sgrColorArgs(ps[i+1:])
				i += used
			}
			if !ok {
				return fg, bg, false
			}
			if n == 38 {
				fg = c
			} else {
				bg = c
			}
		default:
			return fg, bg, false
		}
		if len(sub) > 1 && n != 4 && n != 38 && n != 48 {
			return fg, bg, false // only underline styles and colours have parts
		}
	}
	if fg != "" && fg == bg {
		return fg, bg, false
	}
	return fg, bg, true
}

// sgrColorArgs reads the colour after a 38 or 48 given with semicolons
// ("5;n" or "2;r;g;b") and how many parameters it took.
func sgrColorArgs(ps []string) (string, int, bool) {
	if len(ps) >= 2 && ps[0] == "5" {
		c, ok := sgrColor(ps[:2])
		return c, 2, ok
	}
	if len(ps) >= 4 && ps[0] == "2" {
		c, ok := sgrColor(ps[:4])
		return c, 4, ok
	}
	return "", 0, false
}

// sgrColor names the colour of "5", n or "2", [colour space,] r, g, b
// so that the same colour has one name whichever way it was given.
func sgrColor(p []string) (string, bool) {
	num := func(s string) (int, bool) {
		n, err := strconv.Atoi(s)
		return n, err == nil && n >= 0 && n <= 255
	}
	switch {
	case len(p) == 2 && p[0] == "5":
		n, ok := num(p[1])
		if ok && n < 16 {
			return "c" + strconv.Itoa(n), true
		}
		return "i" + strconv.Itoa(n), ok
	case (len(p) == 4 || len(p) == 5) && p[0] == "2":
		rgb := p[len(p)-3:]
		r, ok1 := num(rgb[0])
		g, ok2 := num(rgb[1])
		b, ok3 := num(rgb[2])
		return fmt.Sprintf("#%02x%02x%02x", r, g, b), ok1 && ok2 && ok3
	}
	return "", false
}

// sgrLen is the length of the SGR sequence s starts with, 0 when it does
// not start with one.
func sgrLen(s string) int {
	if len(s) < 3 || s[0] != '\x1b' || s[1] != '[' {
		return 0
	}
	for i := 2; i < len(s); i++ {
		switch c := s[i]; {
		case c == 'm':
			return i + 1
		case c >= '0' && c <= '9' || c == ';' || c == ':':
		default:
			return 0
		}
	}
	return 0
}

// Doc lists every string value and every key of a decoded document
// (TOML or JSON decoded into maps, slices and strings) that Has unsafe
// text, as `place = "value"` with both quoted so the list itself prints
// safely: `pack[0].use[1] = "tdd\x1b[2J"`, `key pack[0].skill."a\u202e"`.
// Keys and indexes are visited in order, so the list is stable.
func Doc(v any) []string { return doc(v, nil) }

// DocSchema is Doc for a document of which skd knows the places in
// strict: those (names, paths, settings skd prints or uses) are held to
// Has, as Doc does. Everything else, the tables and keys of a newer
// version or of the person editing the file, is only held to Dangerous,
// so a multi-line string with tabs there does not make the file
// unreadable. A pattern is a dotted list of keys, array indexes left
// out, with "*" for any key: "pack.use" is every pack[i].use[j],
// "pack.skill.*" every key of a pack's skill table (and its value when
// it is a string), "pack.skill.*.as" each of their as.
func DocSchema(v any, strict []string) []string {
	pats := make([][]string, len(strict))
	for i, p := range strict {
		pats[i] = strings.Split(p, ".")
	}
	return doc(v, pats)
}

// Dangerous reports whether s holds what must never reach a terminal
// even as text skd does not use: invalid UTF-8, or an unsafe character
// other than tab, line feed and carriage return (ESC, BEL, other C0
// controls, DEL, C1, bidi controls).
func Dangerous(s string) bool {
	return !utf8.ValidString(s) || strings.ContainsFunc(s, func(r rune) bool {
		return Unsafe(r) && r != '\t' && r != '\n' && r != '\r'
	})
}

// doc walks v; with pats nil every place is strict.
func doc(v any, pats [][]string) []string {
	match := func(keys []string) bool {
		if pats == nil {
			return true
		}
	next:
		for _, p := range pats {
			if len(p) != len(keys) {
				continue
			}
			for i := range p {
				if p[i] != "*" && p[i] != keys[i] {
					continue next
				}
			}
			return true
		}
		return false
	}
	bad := func(s string, strict bool) bool {
		if strict {
			return Has(s)
		}
		return Dangerous(s)
	}
	var out []string
	var walk func(path string, keys []string, v any)
	walk = func(path string, keys []string, v any) {
		switch v := v.(type) {
		case string:
			if bad(v, match(keys)) {
				out = append(out, path+" = "+strconv.Quote(v))
			}
		case []any:
			for i, e := range v {
				walk(fmt.Sprintf("%s[%d]", path, i), keys, e)
			}
		case []map[string]any:
			for i, e := range v {
				walk(fmt.Sprintf("%s[%d]", path, i), keys, e)
			}
		case map[string]any:
			names := make([]string, 0, len(v))
			for k := range v {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, k := range names {
				p := docKey(path, k)
				if bad(k, match(append(slices.Clone(keys), "*"))) {
					out = append(out, "key "+p)
				}
				walk(p, append(slices.Clone(keys), k), v[k])
			}
		}
	}
	walk("", nil, v)
	return out
}

// docKey is path extended by key k, quoted unless it is a bare key.
func docKey(path, k string) string {
	bare := k != "" && !strings.ContainsFunc(k, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	})
	if !bare {
		k = strconv.Quote(k)
	}
	if path == "" {
		return k
	}
	return path + "." + k
}
