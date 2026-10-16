package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Maybe404/skilldots/internal/app"
)

// settle runs cmd as the bubbletea runtime would and feeds finished jobs
// and upstream checks back into the model.
func settle(t *testing.T, m Model, cmd tea.Cmd) Model {
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
		switch msg.(type) {
		case doneMsg, checkMsg, startCheckMsg:
			nm, c := m.Update(msg)
			m = nm.(Model)
			if _, ok := msg.(startCheckMsg); ok {
				m = settle(t, m, c)
			}
		}
	}
	return m
}

func screen(m Model) string { return strings.Join(plain(m), "\n") }

func line(m Model, i int) string { return plain(m)[i] }

// rightRow is the line of the right column showing skill name.
func rightRow(t *testing.T, m Model, name string) string {
	t.Helper()
	for _, l := range plain(m) {
		if i := strings.Index(l, "│"); i >= 0 && (strings.Contains(l[i:], "○ "+name+" ") || strings.Contains(l[i:], "◉ "+name+" ")) {
			return l[i:]
		}
	}
	t.Fatalf("no row for %s:\n%s", name, screen(m))
	return ""
}

func mattOutdated() *app.Outdated {
	return &app.Outdated{Packs: []app.PackStatus{
		{Repo: "mattpocock/skills", Commit: "49dd158aabbccdd", Latest: "fedcba9876", Changed: []string{"tdd"},
			New: []string{"pr"}, Gone: []string{"grill-me"}},
		{Repo: "kepano/obsidian-skills", Commit: "1234567", Latest: "1234567"},
	}}
}

func TestUpstreamCheckAtStart(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 40)
	f.check = mattOutdated()

	// Init starts the check; while it runs the dashboard is not busy.
	nm, cmd := m.Update(m.Init()())
	m = nm.(Model)
	if m.check != checkRunning || m.busy != "" {
		t.Fatalf("check = %v, busy = %q", m.check, m.busy)
	}
	if st := line(m, m.h-2); !strings.Contains(st, "checking upstream…") {
		t.Fatalf("status while checking: %q", st)
	}
	m, _ = press(t, m, "down")
	if m.left.cur != 2 {
		t.Fatal("keys wait for the check")
	}
	m, _ = press(t, m, "up")
	m = settle(t, m, cmd)

	if h := line(m, 0); !strings.Contains(h, "↑ 1 pack has updates") {
		t.Fatalf("header: %q", h)
	}
	if l := line(m, 2); !strings.Contains(l, "mattpocock/skills") || !strings.Contains(l, "↑") {
		t.Fatalf("pack row has no ↑: %q", l)
	}
	if l := line(m, 3); strings.Contains(l, "↑") {
		t.Fatalf("pack without updates marked: %q", l)
	}
	if !strings.Contains(screen(m), "↑ upstream fedcba9 · 1 changed · 1 new · 1 gone") {
		t.Fatalf("pack header lacks the upstream summary:\n%s", screen(m))
	}
	for name, want := range map[string]string{"tdd": "↑ changed", "pr": "new upstream", "grill-me": "gone upstream"} {
		if r := rightRow(t, m, name); !strings.Contains(r, want) {
			t.Errorf("%s row: %q, want %q", name, r, want)
		}
	}
	if r := rightRow(t, m, "code-review"); strings.Contains(r, "new upstream") || !strings.Contains(r, "not installed") {
		t.Errorf("code-review was upstream before: %q", r)
	}

	// U updates exactly the packs with updates; then the count is zero
	// and the status line lists what changed.
	m, cmd = press(t, m, "U")
	if m.busy != "updating 1 pack" {
		t.Fatalf("busy = %q", m.busy)
	}
	m = settle(t, m, cmd)
	if !slices.Contains(f.calls, "update mattpocock/skills") {
		t.Fatalf("calls: %v", f.calls)
	}
	if h := line(m, 0); !strings.Contains(h, "✓ no updates") || strings.Contains(h, "↑ 1") {
		t.Fatalf("header after U: %q", h)
	}
	if strings.Contains(line(m, 2), "↑") {
		t.Fatalf("pack still marked after U: %q", line(m, 2))
	}
	if r := rightRow(t, m, "tdd"); strings.Contains(r, "changed") {
		t.Fatalf("tdd still marked: %q", r)
	}
	if !strings.Contains(m.status, "updated") {
		t.Fatalf("status after U: %q", m.status)
	}

	// Nothing left: U says so instead of fetching everything.
	n := len(f.calls)
	m, _ = press(t, m, "U")
	if m.busy != "" || len(f.calls) != n || !strings.Contains(m.status, "no upstream updates") {
		t.Fatalf("U with nothing to update: busy %q, calls %v, status %q", m.busy, f.calls[n:], m.status)
	}

	// u keeps updating the pack under the cursor.
	m, cmd = press(t, m, "down", "u")
	m = settle(t, m, cmd)
	if f.calls[len(f.calls)-1] != "update kepano/obsidian-skills" {
		t.Fatalf("u: %v", f.calls)
	}
}

// Before a check has finished, U updates every pack as it always did.
func TestUpdateAllBeforeCheck(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 40)
	m, cmd := press(t, m, "U")
	settle(t, m, cmd)
	if !slices.Contains(f.calls, "update ") {
		t.Fatalf("calls: %v", f.calls)
	}
}

func TestUpstreamCheckOffline(t *testing.T) {
	for _, c := range []struct {
		name  string
		check *app.Outdated
		err   error
		why   string
	}{
		{"every pack failed", &app.Outdated{Failed: []app.Failure{
			{Repo: "mattpocock/skills", Error: "fatal: unable to access: Could not resolve host: github.com"},
			{Repo: "kepano/obsidian-skills", Error: "fatal: unable to access: Could not resolve host: github.com"}}},
			nil, "Could not resolve host"},
		{"check could not run", nil, errors.New("skills.toml: bad toml"), "bad toml"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, f := newModel(t, sampleSnapshot(), 120, 40)
			f.check, f.checkErr = c.check, c.err
			nm, cmd := m.Update(m.Init()())
			m = settle(t, nm.(Model), cmd)
			if h := line(m, 0); !strings.Contains(h, "check failed") {
				t.Fatalf("header: %q", h)
			}
			if st := line(m, m.h-2); !strings.Contains(st, "upstream check failed") || !strings.Contains(st, c.why) {
				t.Fatalf("status: %q", st)
			}
			// The dashboard works as before.
			m, cmd = press(t, m, "tab", "space")
			m = settle(t, m, cmd)
			if !slices.Contains(f.calls, "targets grill-me ") {
				t.Fatalf("toggle after a failed check: %v", f.calls)
			}
			// c checks again; U falls back to every pack.
			f.check, f.checkErr = mattOutdated(), nil
			m, cmd = press(t, m, "c")
			if m.check != checkRunning {
				t.Fatal("c does not check again")
			}
			m = settle(t, m, cmd)
			if h := line(m, 0); !strings.Contains(h, "↑ 1 pack has updates") {
				t.Fatalf("header after c: %q", h)
			}
		})
	}
}

// Some packs checked, one not: the rest is shown, the failure named.
func TestUpstreamCheckPartlyFailed(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 40)
	f.check = mattOutdated()
	f.check.Failed = []app.Failure{{Repo: "nobody/empty", Error: "repository not found"}}
	nm, cmd := m.Update(m.Init()())
	m = settle(t, nm.(Model), cmd)
	if h := line(m, 0); !strings.Contains(h, "↑ 1 pack has updates · 1 check failed") {
		t.Fatalf("header: %q", h)
	}
	if !strings.Contains(m.status, "could not check nobody/empty: repository not found") {
		t.Fatalf("status: %q", m.status)
	}
}

// A check that finishes after an update of the same pack does not bring
// back what the update already took in.
func TestStaleCheckAfterUpdate(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 40)
	f.check = mattOutdated()
	moved := sampleSnapshot()
	moved.Packs[0].Commit = "2222222"
	f.snap = moved
	nm, cmd := m.Update(m.Init()())
	m = settle(t, nm.(Model), cmd)
	if h := line(m, 0); !strings.Contains(h, "✓ no updates") {
		t.Fatalf("header: %q", h)
	}
}

func TestHeaderFitsNarrowTerminals(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 90, 30)
	f.check = mattOutdated()
	f.check.Failed = []app.Failure{{Repo: "nobody/empty", Error: "x"}}
	s := sampleSnapshot()
	s.Pending, s.Dirty, s.Tracked, s.Ahead = 3, true, true, 2
	f.snap = s
	nm, cmd := m.Update(m.Init()())
	m = settle(t, nm.(Model), cmd)
	h := line(m, 0)
	if !strings.Contains(h, "↑ 1 to update") || !strings.Contains(h, "2 unpushed") || ansi.StringWidth(h) != 90 {
		t.Fatalf("header at 90 columns: %q", h)
	}
}

// A pack whose only news is a skill moved upstream has no update: no ↑,
// U leaves it alone; the skill says moved.
func TestMovedOnly(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 40)
	f.check = &app.Outdated{Packs: []app.PackStatus{
		{Repo: "mattpocock/skills", Commit: "49dd158aabbccdd", Latest: "fedcba9876", Moved: []string{"tdd"}}}}
	nm, cmd := m.Update(m.Init()())
	m = settle(t, nm.(Model), cmd)
	if h := line(m, 0); !strings.Contains(h, "✓ no updates") || strings.Contains(line(m, 2), "↑") {
		t.Fatalf("moved only shows an update:\n%s", screen(m))
	}
	if r := rightRow(t, m, "tdd"); !strings.Contains(r, "moved") || strings.Contains(r, "changed") {
		t.Fatalf("tdd row: %q", r)
	}
	n := len(f.calls)
	m, _ = press(t, m, "U")
	if m.busy != "" || len(f.calls) != n {
		t.Fatalf("U updated a pack with nothing but a move: %v", f.calls[n:])
	}
}
