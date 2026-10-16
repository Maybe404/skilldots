package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

func init() {
	// Skip the OSC background-colour query: terminals that do not answer it
	// (some embedded consoles, `script`, CI) would otherwise block every
	// styled render. Our palette reads fine on light and dark backgrounds.
	lipgloss.DefaultRenderer().SetHasDarkBackground(true)
}

// Palette. One accent (violet) for brand and selection, one secondary
// (cyan) for links and counts, and a muted grey for everything that should
// not compete for attention. lipgloss degrades these to the terminal's
// capabilities and disables colour when stdout is not a terminal or
// NO_COLOR is set.
var (
	cAccent = lipgloss.Color("#A78BFA")
	cCyan   = lipgloss.Color("#67E8F9")
	cMuted  = lipgloss.Color("#8B8FA3")
	cFaint  = lipgloss.Color("#3F4257")
	cOK     = lipgloss.Color("#4ADE80")
	cWarn   = lipgloss.Color("#FBBF24")
	cErr    = lipgloss.Color("#F87171")
	cText   = lipgloss.Color("#E5E7EB")

	sTitle  = lipgloss.NewStyle().Bold(true).Foreground(cText)
	sDim    = lipgloss.NewStyle().Foreground(cMuted)
	sAccent = lipgloss.NewStyle().Foreground(cAccent)
	sCyan   = lipgloss.NewStyle().Foreground(cCyan)
	sOK     = lipgloss.NewStyle().Foreground(cOK)
	sWarn   = lipgloss.NewStyle().Foreground(cWarn)
	sErr    = lipgloss.NewStyle().Foreground(cErr)
	sHead   = lipgloss.NewStyle().Foreground(cMuted).Bold(true)
	sBrand  = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sCard   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cFaint).Padding(0, 2)
)

func cyan(s string) string { return sCyan.Render(s) }

// theme is the huh theme shared by every prompt so the menu, pickers and
// confirmations look like one product.
func theme() *huh.Theme {
	t := huh.ThemeBase()
	t.Focused.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder()).BorderLeft(true).PaddingLeft(1)
	t.Focused.Title = t.Focused.Title.Foreground(cText).Bold(true)
	t.Focused.Description = t.Focused.Description.Foreground(cMuted)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(cAccent).SetString("❯ ")
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(cAccent).SetString("❯ ")
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(cAccent)
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(cOK).SetString("◉ ")
	t.Focused.UnselectedPrefix = t.Focused.UnselectedPrefix.Foreground(cFaint).SetString("○ ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(cText)
	t.Focused.Option = t.Focused.Option.Foreground(cText)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Background(cAccent).Foreground(lipgloss.Color("#1A1B26")).Bold(true)
	t.Focused.BlurredButton = t.Focused.BlurredButton.Background(cFaint).Foreground(cText)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(cAccent)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(cAccent)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(cFaint)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(cErr)
	t.Help.ShortKey = t.Help.ShortKey.Foreground(cMuted)
	t.Help.ShortDesc = t.Help.ShortDesc.Foreground(cFaint)
	t.Help.ShortSeparator = t.Help.ShortSeparator.Foreground(cFaint)
	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	return t
}

// ask runs one or more prompts as a themed form.
func ask(fields ...huh.Field) error {
	return huh.NewForm(huh.NewGroup(fields...)).WithTheme(theme()).WithShowHelp(true).Run()
}

// card renders the branded header block.
func card(lines ...string) string {
	return sCard.Render(strings.Join(lines, "\n"))
}

// brand is the product mark used at the top of interactive screens.
func brand() string {
	v := Version
	if v == "" || v == "dev" {
		v = "dev"
	}
	return sBrand.Render("◆ skilldots") + "  " + dim("v"+v)
}

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
