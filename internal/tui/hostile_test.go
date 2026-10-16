package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
