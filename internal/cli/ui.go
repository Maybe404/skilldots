package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"golang.org/x/term"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/termbg"
)

func init() {
	// lipgloss never asks the terminal itself (see internal/termbg's init:
	// termenv would wait up to five seconds); setupTheme asks with a short
	// timeout and sets the result.
	usePalette(darkPalette)
}

// palette is one colour scheme. Both share hues: one accent (violet) for
// brand and selection, one secondary (cyan) for links and counts, a muted
// grey for everything that should not compete for attention, and the
// three status colours. lipgloss degrades these to the terminal's
// capabilities and disables colour when stdout is not a terminal or
// NO_COLOR is set.
type palette struct {
	// Text, Muted and Faint are text in decreasing importance, each at
	// least 4.5:1 on Ink and on the dashboard's surfaces (Faint only where
	// it is drawn; TestPaletteContrast checks).
	Accent, Cyan, Muted, Faint, OK, Warn, Err, Text lipgloss.Color
	// Line is for what is not text: borders, dividers, the surfaces'
	// tint and the unfocused button.
	Line lipgloss.Color
	// Ink is the background end of the scheme: text on accent buttons,
	// and the base the dashboard derives its surfaces from.
	Ink   lipgloss.Color
	Light bool
}

var (
	darkPalette = palette{Accent: "#A78BFA", Cyan: "#67E8F9", Muted: "#B4B8CA", Faint: "#9599AE", Line: "#3F4257",
		OK: "#4ADE80", Warn: "#FBBF24", Err: "#F87171", Text: "#E5E7EB", Ink: "#1A1B26"}
	// Same hues, dark enough to read on white.
	lightPalette = palette{Accent: "#6D28D9", Cyan: "#0E7490", Muted: "#4A4F62", Faint: "#62677B", Line: "#A3A7B8",
		OK: "#15803D", Warn: "#B45309", Err: "#B91C1C", Text: "#1E2030", Ink: "#FFFFFF", Light: true}
)

var (
	pal palette

	cAccent, cCyan, cMuted, cFaint, cOK, cWarn, cErr, cText lipgloss.Color

	sTitle, sDim, sAccent, sCyan, sOK, sWarn, sErr, sHead, sBrand, sCard lipgloss.Style
)

// usePalette makes p the colours of every prompt, table and the dashboard.
func usePalette(p palette) {
	pal = p
	cAccent, cCyan, cMuted, cFaint, cOK, cWarn, cErr, cText = p.Accent, p.Cyan, p.Muted, p.Faint, p.OK, p.Warn, p.Err, p.Text
	sTitle = lipgloss.NewStyle().Bold(true).Foreground(cText)
	sDim = lipgloss.NewStyle().Foreground(cMuted)
	sAccent = lipgloss.NewStyle().Foreground(cAccent)
	sCyan = lipgloss.NewStyle().Foreground(cCyan)
	sOK = lipgloss.NewStyle().Foreground(cOK)
	sWarn = lipgloss.NewStyle().Foreground(cWarn)
	sErr = lipgloss.NewStyle().Foreground(cErr)
	sHead = lipgloss.NewStyle().Foreground(cMuted).Bold(true)
	sBrand = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sCard = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.Line).Padding(0, 2)
	lipgloss.DefaultRenderer().SetHasDarkBackground(!p.Light)
}

// EnvTheme overrides the theme when --theme is not given.
const EnvTheme = "SKILLDOTS_THEME"

// setupTheme picks the palette: --theme, else $SKILLDOTS_THEME, else the
// theme setting (skd config set theme), else auto.
// auto asks the terminal for its background (termbg.Timeout at most) and
// assumes dark when it cannot tell. Nothing is asked when no colour would
// be printed anyway: --json, NO_COLOR, TERM=dumb, or stdout not a terminal.
func setupTheme(flag string) error {
	mode := strings.ToLower(strings.TrimSpace(flag))
	src := "--theme"
	if mode == "" {
		mode, src = strings.ToLower(strings.TrimSpace(os.Getenv(EnvTheme))), "$"+EnvTheme
	}
	if mode == "" {
		mode, src = strings.ToLower(strings.TrimSpace(config.Get(config.Theme))), "the theme setting (skd config set theme)"
		// A bad setting must not stop every command, `skd config set
		// theme` included: it falls back to auto, with a warning on
		// stderr (never stdout, which may carry JSON).
		if k, _ := config.Lookup(config.Theme); k.Check(mode) != nil {
			if !themeWarned {
				themeWarned = true
				fmt.Fprintf(os.Stderr, "warning: the theme setting %q is not light, dark or auto; using auto (skd config set theme auto fixes it)\n", mode)
			}
			mode = "auto"
		}
	}
	// The menu runs commands in-process; ask the terminal once.
	if mode == themeDone {
		return nil
	}
	switch mode {
	case "", "auto":
		p := darkPalette
		quiet := flags.json || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" || !stdoutIsTerminal()
		if !quiet {
			if light, ok := detectBackground(termbg.Timeout); ok && light {
				p = lightPalette
			}
		}
		usePalette(p)
	case "dark":
		usePalette(darkPalette)
	case "light":
		usePalette(lightPalette)
	default:
		return fmt.Errorf("%s %q: use light, dark or auto", src, mode)
	}
	themeDone = mode
	return nil
}

// themeDone is the mode setupTheme last settled.
var themeDone = "-"

// themeWarned is set once a bad theme setting was reported.
var themeWarned bool

// detectBackground and stdoutIsTerminal are replaceable in tests.
var (
	detectBackground = termbg.Detect
	stdoutIsTerminal = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
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
	t.Focused.FocusedButton = t.Focused.FocusedButton.Background(cAccent).Foreground(pal.Ink).Bold(true)
	t.Focused.BlurredButton = t.Focused.BlurredButton.Background(pal.Line).Foreground(cText)
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
