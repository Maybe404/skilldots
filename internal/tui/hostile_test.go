package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/hostile"
	"github.com/Maybe404/skilldots/internal/safetext"
)

// TestHostileLibraryDrawsClean is the dashboard's part of the regression
// gate (the commands' is TestHostileLibraryPrintsClean): against a real
// library where everything read from outside carries escape sequences,
// BEL, C1 and bidi controls, no view prints any of them, colour off, so
// none of the output is the dashboard's own. It checks view, before the
// last line of defence in View.
func TestHostileLibraryDrawsClean(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(prev)
	home, _ := sandbox(t)
	git := func(dir string, args ...string) { run(t, dir, args...) }
	up := filepath.Join(home, "up")
	hostile.Upstream(up, git)
	root := newLibrary(t, home)
	o := &libOps{root: root}
	if _, err := o.Add(up, AddRequest{All: true}); err != nil {
		t.Fatal(err)
	}
	hostile.Change(up, git)
	agents := filepath.Join(home, ".agents", "skills")
	lures := hostile.Extras(root, agents, home)
	hostile.Settings(config.Path())
	os.Remove(filepath.Join(agents, "review"))
	write(t, filepath.Join(agents, "review", "SKILL.md"), skillMD("review", hostile.Evil))

	snap, err := o.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(o, snap, testTheme, "1.2.3")
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(Model)
	views := 0
	check := func(what string) {
		t.Helper()
		v := m.view()
		views++
		if m.quitting || strings.TrimSpace(v) == "" {
			t.Fatalf("%s: the dashboard closed", what)
		}
		if strings.ContainsFunc(strings.ReplaceAll(v, "\n", ""), safetext.Unsafe) || strings.Contains(v, "��") {
			t.Errorf("%s draws control characters:\n%q", what, v)
		}
	}
	// The hostile text is drawn (cleaned), not merely absent.
	const shown = "]0;pwned[2J]52;c;cHduZWQ=31mx"
	drawn := func(what string) {
		t.Helper()
		if !strings.Contains(m.view(), shown) {
			t.Errorf("%s does not show the hostile text:\n%s", what, m.view())
		}
	}
	key := func(what string, keys ...string) {
		t.Helper()
		var cmd tea.Cmd
		m, cmd = press(t, m, keys...)
		m = settle(t, m, cmd)
		check(what + " " + strings.Join(keys, " "))
	}
	// closeAll closes whatever dialog is open; esc with none would quit.
	closeAll := func(what string) {
		t.Helper()
		for i := 0; m.ov != nil && i < 4; i++ {
			key(what, "esc")
		}
		if m.ov != nil {
			t.Fatalf("%s: a dialog does not close", what)
		}
	}
	check("start")
	key("upstream check", "c")
	for i := 0; i < 6; i++ {
		key("left column", "j")
		key("left column, a skill", "tab")
		for j := 0; j < 6; j++ {
			key("right column", "j")
		}
		key("agents", "e")
		closeAll("close")
		key("back", "tab")
	}
	key("top", "g")
	key("update plan", "u")
	closeAll("close")
	key("apply", "A")
	key("trash", "t")
	drawn("trash")
	for i := 0; i < 4; i++ {
		key("trash", "j")
	}
	key("restore", "enter")
	closeAll("close")
	// Restore every entry the trash lists: the lures stay where they are.
	for i := 0; ; i++ {
		items, err := o.TrashList()
		if err != nil {
			t.Fatal(err)
		}
		if i >= len(items) {
			break
		}
		key("trash", "t")
		for j := 0; j < i; j++ {
			key("trash", "j")
		}
		key("restore "+items[i].ID, "r")
		closeAll("close")
	}
	for _, p := range lures.Never {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("restoring created %q", p)
		}
	}
	for _, p := range lures.Kept {
		if _, err := os.Lstat(filepath.Join(p, "SKILL.md")); err != nil {
			t.Errorf("restoring moved %q", p)
		}
	}
	key("agent targets", "T")
	key("approve", "a")
	closeAll("close")
	key("settings", ",")
	drawn("settings")
	key("editor", "enter")
	closeAll("close")
	key("history", "h")
	drawn("history")
	key("history record", "enter")
	closeAll("close")
	key("scan", "S")
	drawn("scan")
	for i := 0; i < 4; i++ {
		key("scan", "j")
	}
	closeAll("close")
	key("help", "?")
	closeAll("close")
	key("add", "a")
	m = typeText(t, m, up)
	key("browse", "enter")
	drawn("browse")
	closeAll("close")
	key("a pack's skill", "tab")
	key("edit", "o")
	closeAll("close")
	key("delete", "d")
	closeAll("close")
	key("reload", "r")
	if views < 50 {
		t.Fatalf("only %d views drawn", views)
	}
}

// View is the last line of defence: what the drawing code lets through
// (here a bidi control typed into the filter, which the text input keeps)
// does not reach the terminal; skd's colours do.
func TestViewGuard(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)
	m, _ := newModel(t, sampleSnapshot(), 120, 36)
	m, _ = press(t, m, "/")
	m.finput.SetValue("tdd\u202eevil")
	if !strings.ContainsRune(m.view(), '\u202e') {
		t.Skip("the text input dropped the bidi control itself")
	}
	v := m.View()
	if strings.ContainsRune(v, '\u202e') || !strings.Contains(v, "\x1b[") || !strings.Contains(v, "tddevil") {
		t.Errorf("View: %q", v)
	}
}

// The last line of defence keeps everything the dashboard draws with:
// each view comes through View unchanged (the text inputs' cursor is
// drawn reversed, the surfaces with backgrounds). The one exception is
// a span whose colours the terminal's palette makes the same for text
// and background (the light theme on a 256- or 16-colour terminal): it
// would be invisible, so it is drawn in the terminal's own colours.
func TestViewGuardKeepsTheDashboardsColours(t *testing.T) {
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)
	light := Theme{Accent: "#6D28D9", Cyan: "#0E7490", Muted: "#4A4F62", Faint: "#62677B", Line: "#A3A7B8",
		OK: "#15803D", Warn: "#B45309", Err: "#B91C1C", Text: "#1E2030", Ink: "#FFFFFF", Light: true}
	same := regexp.MustCompile(`\x1b\[(?:1;)?(?:38;5;(\d+);48;5;(\d+)|(9\d);(10\d)|(3\d);(4\d))m`)
	invisible := func(seq []string) bool {
		for i := 1; i+1 < len(seq); i += 2 {
			if seq[i] == "" {
				continue
			}
			a, _ := strconv.Atoi(seq[i])
			b, _ := strconv.Atoi(seq[i+1])
			return a == b || i > 1 && b-a == 10
		}
		return false
	}
	for _, p := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256, termenv.ANSI} {
		lipgloss.SetColorProfile(p)
		for _, th := range []Theme{testTheme, light} {
			m, _ := newModel(t, sampleSnapshot(), 120, 36)
			m.theme = th
			for _, keys := range [][]string{nil, {"j"}, {"tab"}, {"j"}, {"?"}, {"esc"}, {"/"}, {"t"}, {"d"}, {"esc", "esc"}, {","}, {"enter"}} {
				m, _ = press(t, m, keys...)
				v, w := m.View(), m.view()
				if v == w {
					continue
				}
				// Only the invisible spans may differ.
				for _, seq := range same.FindAllStringSubmatch(w, -1) {
					if invisible(seq) {
						w = strings.ReplaceAll(w, seq[0], "")
					}
				}
				if v != w || ansi.Strip(v) != ansi.Strip(m.view()) {
					t.Fatalf("profile %v, light %v, after %v: View dropped what the dashboard drew:\nview %q\nView %q", p, th.Light, keys, w, v)
				}
			}
		}
	}
}
