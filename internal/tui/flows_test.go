package tui

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/scan"
)

func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m, _ = press(t, m, string(r))
	}
	return m
}

func lastCall(f *fakeOps) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return ""
	}
	return f.calls[len(f.calls)-1]
}

func TestAddFlowCategoryAliasAgents(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 36)
	f.browse = &Browse{Repo: "acme/x", Commit: "abcdef0123", Have: map[string]bool{"c": true}, Skills: []pack.Skill{
		{Name: "a", Category: "eng"}, {Name: "b", Category: "eng"}, {Name: "c", Category: "prod"}, {Name: "d", Category: "prod"}}}
	m, _ = press(t, m, "a")
	m = typeText(t, m, "acme/x")
	m, cmd := press(t, m, "enter")
	m = drain(t, m, cmd)
	if m.ov == nil || m.ov.kind != ovPick || !m.ov.items[m.ov.cur].header {
		t.Fatalf("picker should open on the first category heading: %+v", m.ov)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"eng", "prod", "installed", "2 skills"} {
		if !strings.Contains(v, want) {
			t.Errorf("picker lacks %q", want)
		}
	}
	// space on "eng" ticks a and b (skd add --category eng).
	m, _ = press(t, m, "space", "enter")
	if m.ov == nil || m.ov.title != "Alias" {
		t.Fatalf("alias step: %+v", m.ov)
	}
	// A bad alias keeps the step open and says why, on the status line too.
	m = typeText(t, m, "a:b")
	m, _ = press(t, m, "enter")
	if m.ov == nil || m.ov.err == "" || m.statusLvl != lvErr {
		t.Fatalf("bad alias accepted: %+v status %q", m.ov, m.status)
	}
	if !strings.Contains(ansi.Strip(m.View()), "✗ alias") {
		t.Error("error not shown in the dialog")
	}
	m, _ = press(t, m, "backspace", "backspace", "enter")
	if m.ov == nil || !strings.HasPrefix(m.ov.title, "Agents for acme/x") {
		t.Fatalf("agents step: %+v", m.ov)
	}
	m, cmd = press(t, m, "down", "space", "enter")
	m = drain(t, m, cmd)
	if got := lastCall(f); got != "add acme/x a,b false alias=a agents=claude" {
		t.Fatalf("add call = %q", got)
	}
	if m.ov != nil {
		t.Fatal("dialog still open")
	}
}

func TestAddFlowDefaults(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 36)
	f.browse = &Browse{Repo: "acme/x", Commit: "abcdef0", Have: map[string]bool{}, Skills: []pack.Skill{{Name: "a"}, {Name: "b"}}}
	m, _ = press(t, m, "a")
	m = typeText(t, m, "acme/x")
	m, cmd := press(t, m, "enter")
	m = drain(t, m, cmd)
	// No categories: no headings; tick b; enter through alias and agents.
	if m.ov.items[0].header {
		t.Fatal("heading without categories")
	}
	m, _ = press(t, m, "down", "space", "enter", "enter")
	m, cmd = press(t, m, "enter")
	drain(t, m, cmd)
	if got := lastCall(f); got != "add acme/x b false" {
		t.Fatalf("add call = %q", got)
	}
	// Nothing ticked on a new repository: closes without a job.
	m, _ = newModel(t, sampleSnapshot(), 120, 36)
	m.ops = f
	m, _ = press(t, m, "a")
	m = typeText(t, m, "acme/x")
	m, cmd = press(t, m, "enter")
	m = drain(t, m, cmd)
	m, cmd = press(t, m, "enter")
	if m.ov != nil || cmd != nil {
		t.Fatal("empty selection should just close")
	}
}

func TestNewFlow(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 36)
	m, _ = press(t, m, "n")
	if m.ov == nil || m.ov.kind != ovInput {
		t.Fatal("n opens the name dialog")
	}
	m, _ = press(t, m, "enter")
	if m.ov == nil || m.ov.err == "" {
		t.Fatal("empty name accepted")
	}
	m = typeText(t, m, "../x")
	m, _ = press(t, m, "enter")
	if m.ov == nil || m.ov.err == "" || m.statusLvl != lvErr {
		t.Fatal("invalid name accepted")
	}
	for range 4 {
		m, _ = press(t, m, "backspace")
	}
	m = typeText(t, m, "notes")
	m, _ = press(t, m, "enter")
	if m.ov == nil || m.ov.kind != ovConfirm {
		t.Fatal("asks where")
	}
	m, cmd := press(t, m, "p")
	m = drain(t, m, cmd)
	if got := lastCall(f); got != "new notes true" {
		t.Fatalf("call = %q", got)
	}
	r := m.currentLeft()
	if r == nil || r.kind != rowSkill || r.skill.LinkName != "notes" || r.skill.Kind != engine.KindPrivate {
		t.Fatalf("cursor not on the new skill: %+v", r)
	}
	if !strings.Contains(ansi.Strip(m.View()), "● notes") {
		t.Error("new skill not shown as linked in the left column")
	}
}

func TestForkFlowRename(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 36)
	m, _ = press(t, m, "tab", "down", "f") // tdd
	if m.ov == nil || m.ov.input.Value() != "tdd" {
		t.Fatalf("fork dialog not prefilled: %+v", m.ov)
	}
	m, _ = press(t, m, "backspace", "backspace", "backspace")
	m = typeText(t, m, "bad/name")
	m, _ = press(t, m, "enter")
	if m.ov == nil || m.ov.err == "" {
		t.Fatal("invalid fork name accepted")
	}
	for range 8 {
		m, _ = press(t, m, "backspace")
	}
	m = typeText(t, m, "my-tdd")
	m, _ = press(t, m, "enter")
	m, cmd := press(t, m, "y")
	drain(t, m, cmd)
	if got := lastCall(f); got != "fork tdd false as my-tdd" {
		t.Fatalf("call = %q", got)
	}
	// Clearing the name keeps the original.
	m, f = newModel(t, sampleSnapshot(), 120, 36)
	m, _ = press(t, m, "tab", "down", "f", "backspace", "backspace", "backspace", "enter")
	m, cmd = press(t, m, "enter")
	drain(t, m, cmd)
	if got := lastCall(f); got != "fork tdd false" {
		t.Fatalf("call = %q", got)
	}
}

func scanFixture() *ScanResult {
	it := func(target, name string, st scan.State) scan.Item {
		return scan.Item{Target: target, Name: name, State: st, Path: "/h/." + target + "/skills/" + name}
	}
	return &ScanResult{
		Report: scan.Report{Items: []scan.Item{it("agents", "x", scan.Copy), it("agents", "dup", scan.Copy), it("claude", "x", scan.Copy),
			it("claude", "gone", scan.Broken), it("agents", "ok", scan.Managed)}},
		Proposals: []app.Proposal{
			{Item: it("agents", "dup", scan.Copy), Kind: "skip", Label: "resolve by hand"},
			{Item: it("claude", "gone", scan.Broken), Kind: "drop", Label: "broken link; remove"},
			{Item: it("agents", "x", scan.Copy), Kind: "own", Label: "copy into skills/ as your own"},
			{Item: it("claude", "x", scan.Copy), Kind: "enable", Label: "enable for claude", Via: "own"},
		},
	}
}

func TestScanAdoptFlow(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 36)
	f.scan = scanFixture()
	m, cmd := press(t, m, "S")
	m = drain(t, m, cmd)
	if m.ov == nil || m.ov.kind != ovPick {
		t.Fatal("scan opens the adopt picker")
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"5 entries · 1 managed · 3 unmanaged · 1 broken", "agents", "claude", "copy", "broken", "resolve by hand"} {
		if !strings.Contains(v, want) {
			t.Errorf("scan view lacks %q", want)
		}
	}
	// The skip row cannot be ticked; untick the broken link and adopt the rest.
	for i, it := range m.ov.items {
		if it.label == "dup" && (!it.locked || it.on) {
			t.Fatalf("skip proposal selectable: %+v", it)
		}
		if it.label == "gone" {
			m.ov.cur = i
		}
	}
	m, _ = press(t, m, "space")
	m, cmd = press(t, m, "enter")
	drain(t, m, cmd)
	if got := lastCall(f); got != "adopt agents/x,claude/x" {
		t.Fatalf("call = %q", got)
	}
	// Nothing to adopt: a status line, no dialog.
	m, f = newModel(t, sampleSnapshot(), 120, 36)
	f.scan = &ScanResult{Report: scan.Report{Items: []scan.Item{{Target: "agents", Name: "ok", State: scan.Managed}}}}
	m, cmd = press(t, m, "S")
	m = drain(t, m, cmd)
	if m.ov != nil || !strings.Contains(m.status, "nothing to adopt") {
		t.Fatalf("status %q", m.status)
	}
}

func TestTrashFlow(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 36)
	when := time.Date(2026, 10, 16, 9, 30, 0, 0, time.Local)
	f.trash = []engine.TrashEntry{
		{ID: "20261016-093000/tdd", Name: "tdd", Origin: "/lib/vendor/acme/skills/eng/tdd", When: when, Restorable: true},
		{ID: "20261016-093000/old", Name: "old", Origin: "/h/.claude/skills/old", When: when, Restorable: true},
		{ID: "20261016-093000/lure", Name: "lure", Origin: "/h/Library/LaunchAgents/x", When: when,
			Problem: `where it came from ("/h/Library/LaunchAgents/x") is neither in an agent's skill directory nor in the library's vendor/`},
	}
	m, cmd := press(t, m, "t")
	m = drain(t, m, cmd)
	if m.ov == nil || m.ov.kind != ovList {
		t.Fatal("t opens the trash")
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Trash · 3 items", "tdd", "cannot be restored · from /h/Library/LaunchAgents/x", "2026-10-16 09:30", "/lib/vendor/acme/skills/eng/tdd", "r/enter restore", "x empty trash"} {
		if !strings.Contains(v, want) {
			t.Errorf("trash view lacks %q", want)
		}
	}
	m, cmd = press(t, m, "down", "enter")
	m = drain(t, m, cmd)
	if got := lastCall(f); got != "restore 20261016-093000/old" {
		t.Fatalf("call = %q", got)
	}
	// An entry skd would not restore says why and runs nothing.
	m, cmd = press(t, m, "t")
	m = drain(t, m, cmd)
	m, cmd = press(t, m, "down", "down", "enter")
	if cmd != nil || m.busy != "" || lastCall(f) != "trash-list" {
		t.Fatalf("restoring an entry skd refuses started %q", lastCall(f))
	}
	if v := ansi.Strip(m.View()); m.ov == nil || !strings.Contains(v, "Cannot restore lure") || !strings.Contains(v, "LaunchAgents") {
		t.Fatalf("no reason shown:\n%s", v)
	}
	m, _ = press(t, m, "esc")
	// Emptying asks twice; enter never confirms the second question.
	m, cmd = press(t, m, "t")
	m = drain(t, m, cmd)
	m, _ = press(t, m, "x")
	if m.ov == nil || m.ov.kind != ovConfirm {
		t.Fatal("x asks first")
	}
	m, _ = press(t, m, "y")
	if m.ov == nil || !strings.Contains(m.ov.title, "forever") {
		t.Fatalf("second question: %+v", m.ov)
	}
	m, cmd = press(t, m, "enter", "y")
	if m.ov == nil || cmd != nil || lastCall(f) == "empty" {
		t.Fatal("enter or y emptied the trash")
	}
	m, _ = press(t, m, "esc")
	if m.ov != nil || lastCall(f) == "empty" {
		t.Fatal("esc must cancel")
	}
	m, cmd = press(t, m, "t")
	m = drain(t, m, cmd)
	m, _ = press(t, m, "x", "y")
	m, cmd = press(t, m, "D")
	drain(t, m, cmd)
	if got := lastCall(f); got != "empty" {
		t.Fatalf("call = %q", got)
	}
	// Empty trash: a status line, no dialog.
	m, f = newModel(t, sampleSnapshot(), 120, 36)
	m, cmd = press(t, m, "t")
	m = drain(t, m, cmd)
	if m.ov != nil || m.status != "trash is empty" {
		t.Fatalf("status %q", m.status)
	}
}

func TestBusyRefusesNewKeys(t *testing.T) {
	for _, k := range []string{"n", "t", "S", "a", "f"} {
		m, f := newModel(t, sampleSnapshot(), 120, 36)
		m, _ = press(t, m, "tab", "down")
		m.busy = "updating"
		m, cmd := press(t, m, k)
		if m.ov != nil || cmd != nil || m.busyNote == "" || len(f.calls) != 0 {
			t.Errorf("%s while busy: ov %v cmd %v note %q calls %v", k, m.ov, cmd != nil, m.busyNote, f.calls)
		}
		if !strings.Contains(ansi.Strip(m.View()), "wait until it finishes") {
			t.Errorf("%s: refusal not on the status line", k)
		}
	}
}

func TestHelpListsEveryKey(t *testing.T) {
	m, _ := newModel(t, sampleSnapshot(), 120, 60)
	m, _ = press(t, m, "?")
	text := strings.Join(m.ov.lines, "\n")
	for _, k := range []string{"j k", "pgup", "g G", "tab", "enter", "/", "space", "a ", "u / U", "e ", "f ", "d ", "n ", "S ", "t ", "s ", "A ", "r ", "c ", ",", "?", "q esc", "ctrl+c"} {
		if !strings.Contains(text, k) {
			t.Errorf("help lacks %q", k)
		}
	}
	// Short terminals scroll the help instead of cutting it.
	m, _ = newModel(t, sampleSnapshot(), 90, 14)
	m, _ = press(t, m, "?")
	if !strings.Contains(ansi.Strip(m.View()), "scroll") {
		t.Error("short help does not offer scrolling")
	}
	m, _ = press(t, m, "down", "down")
	if m.ov == nil || m.ov.off != 2 {
		t.Fatal("down scrolls the help")
	}
	m, _ = press(t, m, "x")
	if m.ov != nil {
		t.Fatal("other keys close help")
	}
}

// TestOverlaysOnSmallTerminals opens every dialog at the smallest sizes
// and checks nothing panics and no line is wider than the screen.
func TestOverlaysOnSmallTerminals(t *testing.T) {
	lipgloss.SetColorProfile(0)
	open := map[string]func(m Model, f *fakeOps) Model{
		"new":  func(m Model, f *fakeOps) Model { m, _ = press(t, m, "n"); return m },
		"fork": func(m Model, f *fakeOps) Model { m, _ = press(t, m, "tab", "f"); return m },
		"help": func(m Model, f *fakeOps) Model { m, _ = press(t, m, "?"); return m },
		"scan": func(m Model, f *fakeOps) Model {
			f.scan = scanFixture()
			m, c := press(t, m, "S")
			return drain(t, m, c)
		},
		"trash": func(m Model, f *fakeOps) Model {
			f.trash = []engine.TrashEntry{{ID: "b/x", Name: strings.Repeat("long-name-", 12), Origin: "/very/" + strings.Repeat("deep/", 30)}}
			m, c := press(t, m, "t")
			return drain(t, m, c)
		},
		"plan": func(m Model, f *fakeOps) Model {
			m, c := press(t, m, "U")
			return drain(t, m, c)
		},
		"plan one": func(m Model, f *fakeOps) Model {
			m, c := press(t, m, "u")
			return drain(t, m, c)
		},
		"history": func(m Model, f *fakeOps) Model {
			f.history = []app.UpdateRecord{{Repo: strings.Repeat("long/", 30), From: "1", To: "2", Changed: []string{strings.Repeat("x", 90)},
				Commits: []app.UpstreamCommit{{Hash: "abc", Subject: strings.Repeat("subject ", 30)}}}}
			m, c := press(t, m, "h")
			m = drain(t, m, c)
			m, _ = press(t, m, "enter")
			return m
		},
		"settings": func(m Model, f *fakeOps) Model {
			m, c := press(t, m, ",")
			m = drain(t, m, c)
			m, _ = press(t, m, "enter")
			return m
		},
		"edit": func(m Model, f *fakeOps) Model { m, _ = press(t, m, "tab", "o"); return m },
		"add": func(m Model, f *fakeOps) Model {
			f.browse = &Browse{Repo: "acme/x", Have: map[string]bool{}, Skills: []pack.Skill{{Name: "a", Category: "c"}}}
			m, _ = press(t, m, "a", "x", "enter")
			m, c := press(t, m, "enter")
			m = drain(t, m, c)
			m, _ = press(t, m, "enter", "enter")
			return m
		},
	}
	for _, size := range [][2]int{{60, 12}, {61, 13}, {90, 12}, {40, 8}, {200, 50}} {
		for name, fn := range open {
			m, f := newModel(t, sampleSnapshot(), size[0], size[1])
			m = fn(m, f)
			for i, l := range strings.Split(m.View(), "\n") {
				if w := ansi.StringWidth(l); w > size[0] {
					t.Errorf("%s at %dx%d: line %d is %d wide", name, size[0], size[1], i, w)
				}
			}
		}
	}
}

func TestInputShowsPlaceholderAndHintsOnShortScreens(t *testing.T) {
	lipgloss.SetColorProfile(0)
	m, _ := newModel(t, sampleSnapshot(), 120, 36)
	m, _ = press(t, m, "n")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "my-skill") {
		t.Error("placeholder cut to its first letter")
	}
	m, _ = newModel(t, sampleSnapshot(), 60, 12)
	m, _ = press(t, m, "n")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "esc cancel") {
		t.Errorf("key hints dropped on a short screen:\n%s", v)
	}
}

func TestSettingsFlow(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 40)
	m, cmd := press(t, m, ",")
	m = drain(t, m, cmd)
	if m.ov == nil || m.ov.title != "Settings" {
		t.Fatalf("no settings page: %+v", m.ov)
	}
	// The third row is the theme (after editor and editor-kind); a value
	// it does not accept is refused in the dialog, a good one is saved.
	m, _ = press(t, m, "down", "down", "enter")
	if m.ov == nil || m.ov.title != "Set theme" {
		t.Fatalf("enter on theme: %+v", m.ov)
	}
	m, _ = press(t, m, "b", "l", "u", "e", "enter")
	if m.ov == nil || !strings.Contains(m.ov.err, "use auto, light, dark") {
		t.Fatalf("blue accepted: %+v", m.ov)
	}
	for range 4 {
		m, _ = press(t, m, "backspace")
	}
	m, cmd = press(t, m, "d", "a", "r", "k", "enter")
	m = drain(t, m, cmd)
	if !slices.Contains(f.calls, "set theme=dark") {
		t.Fatalf("calls: %v", f.calls)
	}
	// library is changed by skd use, not here.
	m, cmd = press(t, m, ",")
	m = drain(t, m, cmd)
	m, _ = press(t, m, "G", "enter")
	if m.ov == nil || !strings.Contains(strings.Join(m.ov.lines, " "), "skd use") {
		t.Fatalf("library row: %+v", m.ov)
	}
}

// A pasted install command or link fills the picker with what it names.
func TestAddFlowPreselects(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 36)
	f.browse = &Browse{Repo: "acme/x", Commit: "abcdef0", Have: map[string]bool{}, Preselect: map[string]bool{"b": true},
		Skills: []pack.Skill{{Name: "a"}, {Name: "b"}}}
	m, _ = press(t, m, "a")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "npx skills add") || !strings.Contains(v, "skills.sh") {
		t.Fatalf("the input does not say what it accepts:\n%s", v)
	}
	// Text that is no source is refused in the dialog, before any fetch.
	m = typeText(t, m, "npx skills remove x/y")
	m, cmd := press(t, m, "enter")
	if m.ov == nil || m.ov.err == "" || cmd != nil {
		t.Fatalf("bad command accepted: %+v", m.ov)
	}
	for range len("npx skills remove x/y") {
		m, _ = press(t, m, "backspace")
	}
	m = typeText(t, m, "npx skills add acme/x --skill b")
	m, cmd = press(t, m, "enter")
	m = drain(t, m, cmd)
	if lastCall(f) != "browse npx skills add acme/x --skill b" {
		t.Fatalf("calls: %v", f.calls)
	}
	if m.ov == nil || m.ov.items[0].on || !m.ov.items[1].on {
		t.Fatalf("b is not ticked: %+v", m.ov.items)
	}
	if !strings.Contains(m.status, "1 ticked from what you pasted") {
		t.Fatalf("status: %q", m.status)
	}
	m, _ = press(t, m, "enter", "enter")
	m, cmd = press(t, m, "enter")
	drain(t, m, cmd)
	if got := lastCall(f); got != "add npx skills add acme/x --skill b b false" {
		t.Fatalf("add call = %q", got)
	}
}

func TestHistoryView(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 40)
	m, cmd := press(t, m, "h")
	m = drain(t, m, cmd)
	if m.ov != nil || !strings.Contains(m.status, "no updates recorded yet") {
		t.Fatalf("empty history: %+v %q", m.ov, m.status)
	}
	f.history = []app.UpdateRecord{
		{Repo: "acme/x", From: "1111111aaa", To: "2222222bbb", Time: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
			Changed: []string{"tdd"}, Gone: []string{"old"}, Commits: []app.UpstreamCommit{{Hash: "abcdef123", Subject: "tdd: clearer\x1b[31m steps"}}, MoreCommits: true},
		{Repo: "acme/y", To: "3333333", Added: []string{"a"}, Time: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)},
	}
	m, cmd = press(t, m, "h")
	m = drain(t, m, cmd)
	if m.ov == nil || !strings.HasPrefix(m.ov.title, "Update history · 2 updates") {
		t.Fatalf("history view: %+v", m.ov)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"acme/x", "1111111→2222222", "1 changed · 1 gone", "acme/y", "1 added"} {
		if !strings.Contains(v, want) {
			t.Errorf("history lacks %q:\n%s", want, v)
		}
	}
	m, _ = press(t, m, "enter")
	text := strings.Join(m.ov.lines, "\n")
	for _, want := range []string{"1111111 → 2222222", "changed       tdd", "upstream gone old", "abcdef1 tdd: clearer", "… more"} {
		if !strings.Contains(text, want) {
			t.Errorf("details lack %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b") {
		t.Error("a control character from upstream reached the screen")
	}
	// h no longer moves to the left column; ← does.
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, "right")
	m, _ = press(t, m, "left")
	if m.focus != focusLeft {
		t.Fatal("← does not switch to the left column")
	}
}

func TestEditFlow(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 40)
	// Your own skill opens straight away.
	m, _ = press(t, m, "G")
	for m.currentLeft() == nil || m.currentLeft().skill.LinkName != "subagent" {
		m, _ = press(t, m, "up")
	}
	m, cmd := press(t, m, "o")
	m = drain(t, m, cmd)
	if lastCall(f) != "edit subagent fork=false private=false force=false" {
		t.Fatalf("calls: %v", f.calls)
	}
	// A vendored one asks: fork, fork privately, or open anyway.
	m, _ = newModel(t, sampleSnapshot(), 120, 40)
	m.ops = f
	m, _ = press(t, m, "right", "o")
	if m.ov == nil || m.ov.kind != ovConfirm || !strings.Contains(strings.Join(m.ov.lines, " "), "skd update replaces its files") {
		t.Fatalf("vendored: %+v", m.ov)
	}
	for key, want := range map[string]string{
		"f": "edit grill-me fork=true private=false force=false",
		"p": "edit grill-me fork=true private=true force=false",
		"o": "edit grill-me fork=false private=false force=true",
	} {
		if m.ov == nil {
			m, _ = press(t, m, "o")
		}
		m, cmd = press(t, m, key)
		m = drain(t, m, cmd)
		if got := lastCall(f); got != want {
			t.Errorf("%s: %q, want %q", key, got, want)
		}
	}
	n := len(f.calls)
	m, _ = press(t, m, "o", "esc")
	if len(f.calls) != n {
		t.Fatal("esc opened it")
	}
	// A terminal editor gets the screen: the job ends in an exec command.
	f.foreground = exec.Command("true")
	m, _ = newModel(t, sampleSnapshot(), 120, 40)
	m.ops = f
	m, _ = press(t, m, "right", "o")
	m, cmd = press(t, m, "o")
	var msgs []tea.Msg
	for _, c := range cmd().(tea.BatchMsg) {
		if c != nil {
			msgs = append(msgs, c())
		}
	}
	for _, msg := range msgs {
		if d, ok := msg.(doneMsg); ok {
			_, ec := m.Update(d)
			if ec == nil {
				t.Fatal("no command to hand over the terminal")
			}
		}
	}
}

// On a short terminal a dialog drops its description and then its body,
// never the row that says which keys do what: o on a vendored skill and u
// on a pack both take keys whose labels must stay on screen.
func TestDialogsKeepTheirKeysOnSmallScreens(t *testing.T) {
	lipgloss.SetColorProfile(0)
	cases := map[string]struct {
		open func(m Model) Model
		want []string
	}{
		"o": {func(m Model) Model { m, _ = press(t, m, "tab", "o"); return m },
			[]string{"f/enter fork into skills/ and open", "p fork into private/ and open", "o open anyway", "esc cancel"}},
		"u": {func(m Model) Model { m, c := press(t, m, "u"); return drain(t, m, c) },
			[]string{"y/enter update", "esc cancel"}},
		"d": {func(m Model) Model { m, _ = press(t, m, "tab", "d"); return m }, []string{"esc cancel"}},
	}
	for _, size := range [][2]int{{90, 12}, {60, 12}, {61, 13}, {120, 14}} {
		for name, c := range cases {
			m, _ := newModel(t, sampleSnapshot(), size[0], size[1])
			m = c.open(m)
			if m.ov == nil {
				t.Fatalf("%s: no dialog", name)
			}
			v := ansi.Strip(m.View())
			for _, w := range c.want {
				if !strings.Contains(v, w) {
					t.Errorf("%s at %dx%d: %q not on screen:\n%s", name, size[0], size[1], w, v)
				}
			}
			if !strings.Contains(v, m.ov.title) && size[1] > 12 {
				t.Errorf("%s at %dx%d: title gone:\n%s", name, size[0], size[1], v)
			}
		}
	}
}
