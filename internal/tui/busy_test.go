package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Maybe404/skilldots/internal/engine"
)

// doneOf runs a job command and returns its doneMsg.
func doneOf(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	var find func(c tea.Cmd) tea.Msg
	find = func(c tea.Cmd) tea.Msg {
		if c == nil {
			return nil
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			for _, cc := range msg {
				if d := find(cc); d != nil {
					return d
				}
			}
		case doneMsg:
			return msg
		}
		return nil
	}
	d := find(cmd)
	if d == nil {
		t.Fatal("command did not run a job")
	}
	return d
}

func TestCtrlCWhileBusyWaits(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 36)
	m, job := press(t, m, "A")
	if m.busy == "" {
		t.Fatal("A starts a job")
	}
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = nm.(Model)
	if cmd != nil || m.quitting || !m.quitAfter {
		t.Fatalf("ctrl+c while busy must not quit yet: cmd %v quitting %v", cmd != nil, m.quitting)
	}
	if !strings.Contains(ansi.Strip(m.View()), "quitting when it finishes") {
		t.Errorf("no notice:\n%s", ansi.Strip(m.View()))
	}
	// A refused key does not hide the pending quit.
	m, _ = press(t, m, "u")
	if !strings.Contains(ansi.Strip(m.View()), "quitting when it finishes") {
		t.Error("pending quit hidden by a refused key")
	}
	// The job ends: the dashboard quits and keeps its outcome for Run.
	nm, cmd = m.Update(doneOf(t, job))
	m = nm.(Model)
	if cmd == nil || !m.quitting {
		t.Fatal("does not quit once the job is done")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("not a quit command")
	}
	if m.status != "ok" || m.statusLvl != lvOK || len(f.calls) != 1 {
		t.Fatalf("outcome lost: %q %v %v", m.status, m.statusLvl, f.calls)
	}
	if got := statusText(m.statusLvl, m.status); got != "✓ ok" {
		t.Errorf("printed outcome: %q", got)
	}
}

// While a job runs, every key that would start another operation or open
// an action dialog is refused with a visible reason, and nothing is called.
func TestBusyRefusesOperationKeys(t *testing.T) {
	for _, start := range [][]string{{"tab"}, nil, {"j", "j", "j"}} { // a skill in a pack, a pack, an own skill
		for _, k := range []string{"space", "a", "A", "u", "U", "d", "e", "s", "f", "r", "o", "h", ","} {
			m, f := newModel(t, sampleSnapshot(), 120, 36)
			m, _ = press(t, m, start...)
			m.busy = "updating acme/skills"
			before := m.snap
			m, cmd := press(t, m, k)
			if cmd != nil {
				t.Errorf("%v %q: returned a command while busy", start, k)
			}
			if m.ov != nil {
				t.Errorf("%v %q: opened %v while busy", start, k, m.ov.kind)
			}
			if m.busy != "updating acme/skills" || m.snap != before || len(f.calls) != 0 {
				t.Errorf("%v %q: state changed while busy: busy %q calls %v", start, k, m.busy, f.calls)
			}
			if !strings.Contains(ansi.Strip(m.View()), "updating acme/skills… · wait until it finishes") {
				t.Errorf("%v %q: refusal not shown:\n%s", start, k, ansi.Strip(m.View()))
			}
		}
	}
	// Moving, filtering and help stay available.
	m, _ := newModel(t, sampleSnapshot(), 120, 36)
	m.busy = "x"
	m, _ = press(t, m, "down", "tab", "?")
	if m.ov == nil || m.ov.kind != ovHelp {
		t.Error("help unavailable while busy")
	}
}

func TestFinishOutcomes(t *testing.T) {
	fresh := sampleSnapshot()
	fresh.Packs = fresh.Packs[1:] // the job removed mattpocock/skills
	cases := []struct {
		name   string
		res    result
		status string
		lvl    level
		snap   *Snapshot
	}{
		{name: "message", res: result{msg: "vendored 2", snap: fresh}, status: "vendored 2", lvl: lvOK, snap: fresh},
		{name: "no message", res: result{}, status: "applying done", lvl: lvOK},
		{name: "error with partial message", res: result{err: errors.New("acme/x: fetch failed\n  fatal: repository not found"), msg: "updated 1 pack(s)", snap: fresh},
			status: "acme/x: fetch failed fatal: repository not found · updated 1 pack(s)", lvl: lvErr, snap: fresh},
		{name: "error without snapshot keeps the old one", res: result{err: errors.New("boom")}, status: "boom", lvl: lvErr},
		{name: "warning", res: result{err: warning("no remote configured · committed"), snap: fresh},
			status: "no remote configured · committed", lvl: lvWarn, snap: fresh},
		{name: "browse error", res: result{err: errors.New("x/y contains no SKILL.md"), src: "x/y"},
			status: "x/y contains no SKILL.md", lvl: lvErr},
	}
	for _, c := range cases {
		m, _ := newModel(t, sampleSnapshot(), 120, 36)
		orig := m.snap
		m.busy = "applying"
		nm, cmd := m.Update(doneMsg{c.res})
		m = nm.(Model)
		if cmd != nil || m.busy != "" || m.ov != nil {
			t.Errorf("%s: cmd %v busy %q overlay %v", c.name, cmd != nil, m.busy, m.ov)
		}
		if m.status != c.status || m.statusLvl != c.lvl {
			t.Errorf("%s: status %q (%v), want %q (%v)", c.name, m.status, m.statusLvl, c.status, c.lvl)
		}
		want := c.snap
		if want == nil {
			want = orig
		}
		if m.snap != want {
			t.Errorf("%s: wrong snapshot kept", c.name)
		}
		icon := map[level]string{lvOK: "✓ ", lvWarn: "! ", lvErr: "✗ "}[c.lvl]
		if v := ansi.Strip(m.View()); !strings.Contains(v, icon+firstLine(c.status)[:min(20, len(c.status))]) {
			t.Errorf("%s: status line not drawn with %q:\n%s", c.name, icon, v)
		}
	}
	// The cursor stays on the same pack when the list changes under it.
	m, _ := newModel(t, sampleSnapshot(), 120, 36)
	m, _ = press(t, m, "down") // kepano/obsidian-skills
	nm, _ := m.Update(doneMsg{result{snap: fresh}})
	if p := nm.(Model).currentPack(); p == nil || p.Repo != "kepano/obsidian-skills" {
		t.Errorf("cursor moved: %+v", p)
	}
	// Reports are merged by repository.
	nm, _ = m.Update(doneMsg{result{reports: []*engine.SyncReport{{Repo: "a/b", NewCommit: "1"}}}})
	if nm.(Model).reports["a/b"] == nil {
		t.Error("report dropped")
	}
}

func TestTracker(t *testing.T) {
	tr := newTracker()
	if !tr.begin() {
		t.Fatal("begin before close")
	}
	idle := tr.close()
	select {
	case <-idle:
		t.Fatal("idle while a job runs")
	default:
	}
	if tr.begin() {
		t.Fatal("a job started after close")
	}
	tr.end()
	select {
	case <-idle:
	case <-time.After(time.Second):
		t.Fatal("not idle after the last job ended")
	}
	<-newTracker().close() // nothing ran: closed at once
}

// A job whose command is only dispatched after the dashboard stopped
// must not start: Run has stopped waiting by then.
func TestJobAfterCloseDoesNotRun(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 36)
	m, cmd := press(t, m, "A")
	<-m.jobs.close()
	for _, c := range cmd().(tea.BatchMsg) {
		if msg := c(); msg != nil {
			if _, ok := msg.(doneMsg); ok {
				t.Fatal("job ran after close")
			}
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("ops called: %v", f.calls)
	}
}
