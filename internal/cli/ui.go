package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

func init() {
	// Skip the OSC background-colour query: terminals that do not answer it
	// (some embedded consoles, `script`, CI) would otherwise block every
	// styled render. Our palette reads fine on light and dark backgrounds.
	lipgloss.DefaultRenderer().SetHasDarkBackground(true)
}

// Styles. lipgloss disables colour on its own when stdout is not a terminal
// or NO_COLOR is set.
var (
	sTitle  = lipgloss.NewStyle().Bold(true)
	sDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	sAccent = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	sOK     = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	sWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	sErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	sHead   = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true)
)

func dim(s string) string    { return sDim.Render(s) }
func accent(s string) string { return sAccent.Render(s) }
func ok(s string) string     { return sOK.Render(s) }
func warn(s string) string   { return sWarn.Render(s) }
func bad(s string) string    { return sErr.Render(s) }
func bold(s string) string   { return sTitle.Render(s) }

// newTable builds a borderless table with a dimmed header row.
func newTable(headers ...string) *table.Table {
	return table.New().
		Border(lipgloss.HiddenBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderRow(false).BorderColumn(false).
		Headers(headers...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return sHead.PaddingRight(2)
			}
			return lipgloss.NewStyle().PaddingRight(2)
		})
}

// truncate shortens s to n runes with an ellipsis.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// plural returns "1 skill" / "3 skills".
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// joinOr renders a list or a placeholder when empty.
func joinOr(items []string, empty string) string {
	if len(items) == 0 {
		return dim(empty)
	}
	return strings.Join(items, ", ")
}
