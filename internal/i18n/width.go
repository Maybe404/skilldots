package i18n

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Text is measured in terminal cells, not bytes or runes: a Chinese
// character takes two. These use the same measure as lipgloss and the
// dashboard (ansi.StringWidth), so a table built here lines up with one
// built there, whatever the locale.

// Width is the number of terminal cells s takes.
func Width(s string) int { return ansi.StringWidth(s) }

// PadRight pads s with spaces to w cells; a longer s is left alone.
func PadRight(s string, w int) string {
	if n := w - Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// PadLeft pads s on the left with spaces to w cells.
func PadLeft(s string, w int) string {
	if n := w - Width(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

// Truncate shortens s to at most w cells, ending it with "…" when it was
// cut. It never splits a character.
func Truncate(s string, w int) string {
	if Width(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}
