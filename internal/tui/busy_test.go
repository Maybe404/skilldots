package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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
