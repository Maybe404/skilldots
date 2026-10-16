// Package safetext makes text from outside skd (upstream SKILL.md files,
// directory names, library files, git output) safe to print on a
// terminal: nothing in it can move the cursor, rewrite the screen, change
// the window title or reorder what is shown.
package safetext

import (
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
