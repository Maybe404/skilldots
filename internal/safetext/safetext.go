// Package safetext makes text from outside skd (upstream SKILL.md files,
// directory names, library files, git output) safe to print on a
// terminal: nothing in it can move the cursor, rewrite the screen, change
// the window title or reorder what is shown.
package safetext

import (
	"fmt"
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
// defence: line breaks stay, colour codes (SGR, ESC [ … m) stay when sgr
// is set, and everything else is as Clean leaves it: no other escape
// sequence can start, so nothing can move the cursor, clear the screen,
// set the window title or write the clipboard. Text from outside skd is
// cleaned where it is read or printed; this only catches what was missed.
func Screen(s string, sgr bool) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r != '\n' && (Unsafe(r) || space(r)) }) && utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	s = strings.ToValidUTF8(s, "�")
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '\x1b' && sgr:
			if m := sgrLen(s[i:]); m > 0 {
				b.WriteString(s[i : i+m])
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
func Doc(v any) []string {
	var out []string
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch v := v.(type) {
		case string:
			if Has(v) {
				out = append(out, path+" = "+strconv.Quote(v))
			}
		case []any:
			for i, e := range v {
				walk(fmt.Sprintf("%s[%d]", path, i), e)
			}
		case []map[string]any:
			for i, e := range v {
				walk(fmt.Sprintf("%s[%d]", path, i), e)
			}
		case map[string]any:
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				p := docKey(path, k)
				if Has(k) {
					out = append(out, "key "+p)
				}
				walk(p, v[k])
			}
		}
	}
	walk("", v)
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
