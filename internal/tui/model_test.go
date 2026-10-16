package tui

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/targets"
)

var testTheme = Theme{Accent: "#A78BFA", Cyan: "#67E8F9", Muted: "#B4B8CA", Faint: "#9599AE", Line: "#3F4257", OK: "#4ADE80",
	Warn: "#FBBF24", Err: "#F87171", Text: "#E5E7EB", Ink: "#1A1B26"}

func vendored(repo, name, cat string, agents ...string) Skill {
	return Skill{Name: name, LinkName: name, Kind: engine.KindVendor, Origin: repo, Category: cat, Installed: true,
		Agents: agents, Description: "the " + name + " skill"}
}

func upstreamOnly(repo, name, cat string) Skill {
	return Skill{Name: name, LinkName: name, Kind: engine.KindVendor, Origin: repo, Category: cat}
}

func sampleSnapshot() *Snapshot {
	matt := Pack{Repo: "mattpocock/skills", Alias: "matt", Commit: "49dd158aabbccdd", When: "3 days ago", License: "MIT",
		Upstream: true, Selected: 2,
		Skills: []Skill{
			vendored("mattpocock/skills", "grill-me", "productivity", "agents", "claude"),
			vendored("mattpocock/skills", "tdd", "engineering", "agents"),
			upstreamOnly("mattpocock/skills", "code-review", "engineering"),
			upstreamOnly("mattpocock/skills", "pr", "engineering"),
		}}
	matt.Total = len(matt.Skills)
	for i := 0; i < 40; i++ {
		matt.Skills = append(matt.Skills, upstreamOnly("mattpocock/skills", fmt.Sprintf("extra-%02d", i), "misc"))
	}
	matt.Total = len(matt.Skills)
	obs := Pack{Repo: "kepano/obsidian-skills", Commit: "1234567", License: "MIT", Upstream: true, Selected: 1,
		Skills: []Skill{vendored("kepano/obsidian-skills", "obsidian-markdown", "", "agents")}, Total: 1}
	empty := Pack{Repo: "nobody/empty", NoLicense: true}
	return &Snapshot{Root: "/tmp/agent-skills", Name: "agent-skills", Branch: "main", Remote: true, Tracked: true,
		Defaults: []string{"agents"},
		Agents:   []targets.Target{{Name: "agents", Path: "/h/.agents/skills"}, {Name: "claude", Path: "/h/.claude/skills"}},
		Packs:    []Pack{matt, obs, empty},
		Mine:     []Skill{{Name: "subagent", LinkName: "subagent", Kind: engine.KindSkill, Installed: true, Agents: []string{"agents"}, Description: "Spawn helpers."}},
		Private:  []Skill{{Name: "html-document", LinkName: "html-document", Kind: engine.KindPrivate, Installed: true}},
		Apps:     []Skill{{Name: "surge", LinkName: "surge", Kind: engine.KindApp, Origin: "surge", Installed: true, Agents: []string{"claude"}}},
	}
}

// fakeOps records calls and returns canned answers.
type fakeOps struct {
	mu     sync.Mutex
	calls  []string
	snap   *Snapshot
	scan   *ScanResult
	trash  []engine.TrashEntry
	browse *Browse
	// check is what Check returns; checkErr fails it.
	check    *app.Outdated
	checkErr error
}

func (f *fakeOps) rec(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}
func (f *fakeOps) Load() (*Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap, nil
}
func (f *fakeOps) SetTargets(s *Skill, repo string, a []string) (string, error) {
	name := repo
	if s != nil {
		name = s.LinkName
	}
	f.rec("targets " + name + " " + strings.Join(a, ","))
	return "ok", nil
}
func (f *fakeOps) Browse(src string) (*Browse, error) {
	f.rec("browse " + src)
	if f.browse != nil {
		return f.browse, nil
	}
	return &Browse{Repo: src, Commit: "abcdef0", Have: map[string]bool{}}, nil
}
func (f *fakeOps) Enable(s Skill) (string, error) {
	f.rec("targets " + s.LinkName + " " + strings.Join(f.snap.Defaults, ","))
	return "ok", nil
}
func (f *fakeOps) Disable(s Skill) (string, error) {
	f.rec("targets " + s.LinkName + " ")
	return "ok", nil
}
func (f *fakeOps) Add(src string, r AddRequest) (string, error) {
	extra := ""
	if r.Alias != "" || len(r.Agents) > 0 {
		extra = fmt.Sprintf(" alias=%s agents=%s", r.Alias, strings.Join(r.Agents, ","))
	}
	f.rec(fmt.Sprintf("add %s %s %v%s", src, strings.Join(r.Names, ","), r.All, extra))
	return "ok", nil
}
func (f *fakeOps) Update(r []string) ([]*engine.SyncReport, string, error) {
	f.rec("update " + strings.Join(r, ","))
	return []*engine.SyncReport{{Repo: "mattpocock/skills", OldCommit: "1111111", NewCommit: "2222222",
		Changed: []string{"tdd"}, UpstreamNew: []string{"fresh"}}}, "updated", nil
}
func (f *fakeOps) Check() (*app.Outdated, error) {
	f.rec("check")
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.checkErr != nil {
		return nil, f.checkErr
	}
	if f.check == nil {
		return &app.Outdated{}, nil
	}
	return f.check, nil
}
func (f *fakeOps) Fork(s Skill, p bool, as string) (string, error) {
	if as != "" {
		as = " as " + as
	}
	f.rec(fmt.Sprintf("fork %s %v%s", s.LinkName, p, as))
	return "ok", nil
}
func (f *fakeOps) Trash(s Skill) (string, error)  { f.rec("trash " + s.LinkName); return "ok", nil }
func (f *fakeOps) Purge(r string) (string, error) { f.rec("purge " + r); return "ok", nil }
func (f *fakeOps) Apply() (string, error)         { f.rec("apply"); return "ok", nil }
func (f *fakeOps) Sync() (string, error)          { f.rec("sync"); return "ok", nil }
func (f *fakeOps) New(n string, p bool) (string, error) {
	f.rec(fmt.Sprintf("new %s %v", n, p))
	kind := engine.KindSkill
	if p {
		kind = engine.KindPrivate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *f.snap
	sk := Skill{Name: n, LinkName: n, Kind: kind, Installed: true, Agents: cp.Defaults}
	if p {
		cp.Private = append(append([]Skill{}, cp.Private...), sk)
	} else {
		cp.Mine = append(append([]Skill{}, cp.Mine...), sk)
	}
	f.snap = &cp
	return "ok", nil
}
func (f *fakeOps) Scan() (*ScanResult, error) { f.rec("scan"); return f.scan, nil }
func (f *fakeOps) Adopt(props []app.Proposal) (string, error) {
	var names []string
	for _, p := range props {
		names = append(names, p.Item.Target+"/"+p.Item.Name)
	}
	f.rec("adopt " + strings.Join(names, ","))
	return "ok", nil
}
func (f *fakeOps) TrashList() ([]engine.TrashEntry, error) { f.rec("trash-list"); return f.trash, nil }
func (f *fakeOps) Restore(id string) (string, error)       { f.rec("restore " + id); return "ok", nil }
func (f *fakeOps) EmptyTrash() (string, error)             { f.rec("empty"); return "ok", nil }

func newModel(t *testing.T, s *Snapshot, w, h int) (Model, *fakeOps) {
	t.Helper()
	f := &fakeOps{snap: s}
	m := New(f, s, testTheme, "1.2.3")
	nm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return nm.(Model), f
}

func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		nm, c := m.Update(msg)
		m, cmd = nm.(Model), c
	}
	return m, cmd
}

// drain runs cmd (and nested batches) and feeds a finished job back into
// the model, as the bubbletea runtime would.
func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	var msgs []tea.Msg
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			for _, cc := range msg {
				run(cc)
			}
		default:
			msgs = append(msgs, msg)
		}
	}
	run(cmd)
	for _, msg := range msgs {
		if d, ok := msg.(doneMsg); ok {
			nm, _ := m.Update(d)
			m = nm.(Model)
		}
	}
	return m
}

func plain(m Model) []string {
	return strings.Split(ansi.Strip(m.View()), "\n")
}

func TestViewLayout(t *testing.T) {
	lipgloss.SetColorProfile(0) // TrueColor: make sure styling never changes the cell count
	for _, size := range [][2]int{{120, 36}, {90, 24}, {200, 60}} {
		w, h := size[0], size[1]
		m, _ := newModel(t, sampleSnapshot(), w, h)
		lines := plain(m)
		if len(lines) != h {
			t.Fatalf("%dx%d: %d lines", w, h, len(lines))
		}
		for i, l := range lines {
			if got := ansi.StringWidth(l); got != w {
				t.Errorf("%dx%d line %d is %d wide: %q", w, h, i, got, l)
			}
		}
		all := strings.Join(lines, "\n")
		for _, want := range []string{
			"◆ skilldots", "v1.2.3", "agent-skills", "main", "synced", // header
			"PACKS", "MINE", "PRIVATE", "APPS", "mattpocock/skills", "subagent", "html-document", "surge", // left
			"49dd158", "3 days ago", "MIT", "grill-me", "productivity", "agents claude", "not installed", // right
			"a add", "q quit", // footer
		} {
			if !strings.Contains(all, want) {
				t.Errorf("%dx%d: view lacks %q\n%s", w, h, want, all)
			}
		}
		if !strings.HasPrefix(lines[0], " ◆ skilldots") {
			t.Errorf("header first: %q", lines[0])
		}
		if !strings.Contains(lines[h-1], "a add") {
			t.Errorf("key bar last: %q", lines[h-1])
		}
		if !strings.Contains(all, "of 44") {
			t.Errorf("%dx%d: long list has no scroll position", w, h)
		}
	}
}

func TestViewEdgeCases(t *testing.T) {
	cases := map[string]struct {
		snap func() *Snapshot
		want []string
	}{
		"empty library": {func() *Snapshot {
			return &Snapshot{Root: "/tmp/lib", Name: "lib", Defaults: []string{"agents"}}
		}, []string{"none yet", "Your library is empty", "no remote"}},
		"only private": {func() *Snapshot {
			return &Snapshot{Root: "/tmp/lib", Name: "lib", Private: []Skill{{Name: "p1", LinkName: "p1", Kind: engine.KindPrivate, Installed: true, Description: "mine only"}}}
		}, []string{"PRIVATE", "p1", "private · this machine only", "mine only"}},
		"pack without vendored skills": {func() *Snapshot {
			return &Snapshot{Root: "/tmp/lib", Name: "lib", Packs: []Pack{{Repo: "a/b", NoLicense: true}}}
		}, []string{"a/b", "0 of 0", "not fetched", "no license", "no skills vendored yet"}},
		"name conflict": {func() *Snapshot {
			msg := `"x" is provided by vendor:a/b and skills/`
			return &Snapshot{Root: "/tmp/lib", Name: "lib", Warnings: []string{msg + "; neither is linked", "unknown target"},
				Mine: []Skill{{Name: "x", LinkName: "x", Kind: engine.KindSkill, Installed: true, Conflict: msg, Agents: []string{"agents"}}}}
		}, []string{"2 problems", "provided by", "conflict", "not linked"}},
	}
	for name, c := range cases {
		for _, size := range [][2]int{{120, 36}, {90, 24}, {60, 12}} {
			m, _ := newModel(t, c.snap(), size[0], size[1])
			lines := plain(m)
			all := strings.Join(lines, "\n")
			for _, want := range c.want {
				if !strings.Contains(all, want) {
					t.Errorf("%s %v: lacks %q\n%s", name, size, want, all)
				}
			}
			for i, l := range lines {
				if ansi.StringWidth(l) != size[0] {
					t.Errorf("%s %v: line %d width %d", name, size, i, ansi.StringWidth(l))
				}
			}
			// Keys must not panic on sparse data.
			press(t, m, "down", "up", "tab", "right", "j", "k", "space", "e", "f", "d", "esc", "/", "z", "esc", "?", "x")
		}
	}
	m, _ := newModel(t, sampleSnapshot(), 50, 10)
	if !strings.Contains(m.View(), "window too small") {
		t.Errorf("tiny window: %q", m.View())
	}
}

func TestResize(t *testing.T) {
	m, _ := newModel(t, sampleSnapshot(), 120, 36)
	m, _ = press(t, m, "tab", "G") // cursor to the end of the long list
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	m = nm.(Model)
	lines := plain(m)
	if len(lines) != 20 || ansi.StringWidth(lines[0]) != 90 {
		t.Fatalf("not re-laid out: %d lines, %d wide", len(lines), ansi.StringWidth(lines[0]))
	}
	if !strings.Contains(strings.Join(lines, "\n"), "extra-39") {
		t.Errorf("cursor row scrolled off after resize")
	}
}

func TestNavigation(t *testing.T) {
	m, _ := newModel(t, sampleSnapshot(), 120, 36)
	if p := m.currentPack(); p == nil || p.Repo != "mattpocock/skills" {
		t.Fatalf("starts on first pack, got %+v", p)
	}
	m, _ = press(t, m, "down")
	if p := m.currentPack(); p == nil || p.Repo != "kepano/obsidian-skills" {
		t.Fatalf("down: %+v", p)
	}
	m, _ = press(t, m, "j", "j") // past the last pack, over the blank and header, onto MINE
	if r := m.currentLeft(); r == nil || r.kind != rowSkill || r.skill.Name != "subagent" {
		t.Fatalf("j skips headers: %+v", r)
	}
	m, _ = press(t, m, "k", "k", "k")
	if p := m.currentPack(); p == nil || p.Repo != "mattpocock/skills" {
		t.Fatalf("back up: %+v", p)
	}
	m, _ = press(t, m, "up")
	if p := m.currentPack(); p == nil || p.Repo != "mattpocock/skills" {
		t.Fatalf("up at top stays: %+v", p)
	}

	// Columns.
	m, _ = press(t, m, "tab")
	if m.focus != focusRight {
		t.Fatal("tab focuses the right column")
	}
	m, _ = press(t, m, "down")
	if s, _ := m.target(); s == nil || s.Name != "tdd" {
		t.Fatalf("right cursor: %+v", s)
	}
	m, _ = press(t, m, "left")
	if m.focus != focusLeft {
		t.Fatal("← focuses the left column")
	}
	m, _ = press(t, m, "right")
	if m.focus != focusRight {
		t.Fatal("→ focuses the right column")
	}
	m, _ = press(t, m, "tab")
	if m.focus != focusLeft {
		t.Fatal("tab toggles back")
	}
	// The empty pack has nothing to focus on the right.
	m, _ = press(t, m, "down", "down", "tab")
	if m.focus != focusLeft {
		t.Fatal("cannot focus an empty right column")
	}
}

func TestFilter(t *testing.T) {
	m, _ := newModel(t, sampleSnapshot(), 120, 36)
	m, _ = press(t, m, "/", "o", "b", "s")
	if !m.filtering || m.filter[focusLeft] != "obs" {
		t.Fatalf("filter typing: %v %q", m.filtering, m.filter[focusLeft])
	}
	rows := m.leftRows()
	var picks []string
	for _, r := range rows {
		if r.kind == rowPack {
			picks = append(picks, m.snap.Packs[r.pack].Repo)
		}
	}
	if len(picks) != 1 || picks[0] != "kepano/obsidian-skills" {
		t.Fatalf("filtered packs: %v", picks)
	}
	if p := m.currentPack(); p == nil || p.Repo != "kepano/obsidian-skills" {
		t.Fatalf("cursor moves to first match: %+v", p)
	}
	if !strings.Contains(ansi.Strip(m.View()), "/ obs") {
		t.Error("filter line not shown")
	}
	m, _ = press(t, m, "enter")
	if m.filtering || m.filter[focusLeft] != "obs" {
		t.Fatal("enter keeps the filter")
	}
	m, _ = press(t, m, "esc")
	if m.filter[focusLeft] != "" || m.quitting {
		t.Fatal("esc clears the filter first")
	}
	if p := m.currentPack(); p == nil || p.Repo != "kepano/obsidian-skills" {
		t.Fatalf("clearing the filter keeps the selection: %+v", p)
	}
	// Right column filter.
	m, _ = press(t, m, "up", "up", "tab", "/", "r", "e", "v")
	if got := m.rightRows(); len(got) != 1 || got[0].Name != "code-review" {
		t.Fatalf("right filter: %+v", got)
	}
	m, _ = press(t, m, "esc")
	if m.filter[focusRight] != "" || len(m.rightRows()) != 44 {
		t.Fatal("esc while typing clears the right filter")
	}
}

func TestQuit(t *testing.T) {
	for _, k := range []string{"q", "esc"} {
		m, _ := newModel(t, sampleSnapshot(), 120, 36)
		m, cmd := press(t, m, k)
		if cmd == nil {
			t.Fatalf("%s: no command", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok || !m.quitting {
			t.Fatalf("%s does not quit", k)
		}
		if m.View() != "" {
			t.Fatalf("%s: view not cleared on quit", k)
		}
	}
	// Busy: q waits.
	m, _ := newModel(t, sampleSnapshot(), 120, 36)
	m.busy = "updating"
	m, cmd := press(t, m, "q")
	if m.quitting || cmd != nil {
		t.Fatal("q while busy must not quit")
	}
}

func TestActions(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 36)

	// space on an enabled skill disables it.
	m, cmd := press(t, m, "tab", "space")
	if m.busy == "" {
		t.Fatal("space starts a job")
	}
	if !strings.Contains(ansi.Strip(m.View()), "disabling grill-me") {
		t.Error("spinner label missing")
	}
	m = drain(t, m, cmd)
	// space on an upstream-only skill installs it.
	m, cmd = press(t, m, "down", "down", "space")
	m = drain(t, m, cmd)
	// e: pick agents, toggle claude on, confirm.
	m, _ = press(t, m, "up", "e")
	if m.ov == nil || m.ov.kind != ovPick || len(m.ov.items) != 2 {
		t.Fatalf("agents overlay: %+v", m.ov)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Agents for tdd") {
		t.Error("agents overlay not drawn")
	}
	m, _ = press(t, m, "down", "space")
	m, cmd = press(t, m, "enter")
	m = drain(t, m, cmd)
	// d on a vendored skill asks, then trashes.
	m, _ = press(t, m, "d")
	if m.ov == nil || m.ov.kind != ovConfirm {
		t.Fatal("d asks first")
	}
	m, _ = press(t, m, "n")
	if m.ov != nil {
		t.Fatal("n cancels")
	}
	m, _ = press(t, m, "d")
	m, cmd = press(t, m, "y")
	m = drain(t, m, cmd)
	// f, enter keeps the name, p forks privately.
	m, _ = press(t, m, "f", "enter")
	m, cmd = press(t, m, "p")
	m = drain(t, m, cmd)
	// u updates this pack; the report shows in the right column.
	m, cmd = press(t, m, "u")
	m = drain(t, m, cmd)
	v := ansi.Strip(m.View())
	for _, want := range []string{"1111111 → 2222222", "changed", "tdd", "upstream new", "fresh", "✓ updated"} {
		if !strings.Contains(v, want) {
			t.Errorf("update report lacks %q", want)
		}
	}
	// d on the pack (left column) purges after confirmation.
	m, cmd = press(t, m, "tab", "d", "enter")
	m = drain(t, m, cmd)
	m, cmd = press(t, m, "s")
	if m.ov == nil {
		t.Fatal("s asks first")
	}
	_ = cmd
	m, cmd = press(t, m, "y")
	m = drain(t, m, cmd)
	// a: type a repository, browse, confirm with follow-all.
	m, _ = press(t, m, "a")
	m, _ = press(t, m, "x", "/", "y")
	m, cmd = press(t, m, "enter")
	m = drain(t, m, cmd)
	if m.ov == nil || m.ov.kind != ovPick {
		t.Fatal("browse opens the picker")
	}
	m, _ = press(t, m, "*", "enter", "enter") // all; no alias
	if m.ov == nil || !strings.HasPrefix(m.ov.title, "Agents for") {
		t.Fatalf("add asks for agents last: %+v", m.ov)
	}
	m, cmd = press(t, m, "enter")
	m = drain(t, m, cmd)

	want := []string{
		"targets grill-me ",
		"add mattpocock/skills code-review false",
		"targets tdd agents,claude",
		"trash tdd",
		"fork tdd true",
		"update mattpocock/skills",
		"purge mattpocock/skills",
		"sync",
		"browse x/y",
		"add x/y  true",
	}
	if strings.Join(f.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(f.calls, "\n"), strings.Join(want, "\n"))
	}
	_ = m
}

// TestViewShortTerminals renders every height from the minimum to where
// the description lines appear, with the cursor on a skill that has a
// description, plus the narrowest supported widths.
func TestViewShortTerminals(t *testing.T) {
	lipgloss.SetColorProfile(0)
	sizes := [][2]int{{90, 12}, {60, 12}}
	for h := minHeight; h <= 19; h++ {
		sizes = append(sizes, [2]int{120, h})
	}
	for _, size := range sizes {
		w, h := size[0], size[1]
		for _, keys := range [][]string{nil, {"tab"}, {"tab", "down"}, {"j", "j"}, {"tab", "?"}, {"tab", "e"}, {"tab", "d"}, {"tab", "a"}} {
			m, _ := newModel(t, sampleSnapshot(), w, h)
			m, _ = press(t, m, keys...)
			if s, _ := m.target(); len(keys) > 0 && keys[0] == "tab" && (s == nil || s.Description == "") {
				t.Fatalf("%dx%d %v: cursor not on a described skill: %+v", w, h, keys, s)
			}
			lines := plain(m)
			if len(lines) != h {
				t.Fatalf("%dx%d %v: %d lines", w, h, keys, len(lines))
			}
			for i, l := range lines {
				if got := ansi.StringWidth(l); got != w {
					t.Errorf("%dx%d %v: line %d is %d wide: %q", w, h, keys, i, got, l)
				}
			}
		}
	}
}

// TestViewStripsControlCharacters feeds terminal escapes through every
// text path that comes from outside skd and checks the raw output only
// carries the dashboard's own colour codes.
func TestViewStripsControlCharacters(t *testing.T) {
	lipgloss.SetColorProfile(0)
	const evil = "\x1b]0;pwned\x07\x1b[2J\x1b[H\x9b6n\x00\x7f\x08\r\n\t"
	s := sampleSnapshot()
	p := &s.Packs[0]
	p.Skills[0].Description = "說明 " + evil + " 🚀 done"
	p.Skills[0].Category = "cat" + evil
	p.Skills[0].Name = "grill-me" + evil // the "upstream" name differs from the link name
	p.Skills[2].Description = evil
	s.Mine[0].Description = "mine " + evil
	s.Mine[0].Conflict = "clash " + evil
	s.Warnings = []string{"warn " + evil + " 日本語"}
	sgr := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	check := func(name string, m Model) {
		t.Helper()
		v := m.View()
		rest := sgr.ReplaceAllString(v, "")
		for i := 0; i < len(rest); i++ {
			if c := rest[i]; (c < 0x20 && c != '\n') || c == 0x7f {
				t.Fatalf("%s: control byte %#x at %d in %q", name, c, i, rest[max(0, i-20):min(len(rest), i+20)])
			}
		}
		if !utf8.ValidString(rest) || strings.ContainsRune(rest, '\u009b') {
			t.Fatalf("%s: invalid UTF-8 or C1 control in output", name)
		}
		for i, l := range strings.Split(ansi.Strip(v), "\n") {
			if ansi.StringWidth(l) != m.w {
				t.Errorf("%s: line %d is %d wide: %q", name, i, ansi.StringWidth(l), l)
			}
		}
	}
	for _, size := range [][2]int{{120, 36}, {90, 24}, {60, 12}} {
		m, _ := newModel(t, s, size[0], size[1])
		check("pack", m)
		mm, _ := press(t, m, "tab")
		check("skill in pack", mm)
		mm, _ = press(t, m, "j", "j", "j")
		check("own skill", mm)
		// Errors and warnings from the engine land in the status line.
		nm, _ := m.Update(doneMsg{result{err: fmt.Errorf("fetch failed %s", evil)}})
		check("error status", nm.(Model))
		nm, _ = m.Update(doneMsg{result{err: warning("careful " + evil)}})
		check("warning status", nm.(Model))
		nm, _ = m.Update(doneMsg{result{msg: "ok " + evil}})
		check("ok status", nm.(Model))
		// A fetched repository in the browse picker.
		nm, _ = m.Update(doneMsg{result{src: "x/y", browse: &Browse{Repo: "x/y", Commit: "abc", Have: map[string]bool{},
			Skills: []pack.Skill{{Name: "evil", Category: "c" + evil, Description: evil + "描述"}}}}})
		check("browse", nm.(Model))
	}
	if got := clean("a\x1b[2Jb\tc\u009bd\xffe 中文 👍🏽"); got != "a[2Jb cde 中文 👍🏽" {
		t.Errorf("clean: %q", got)
	}
}
