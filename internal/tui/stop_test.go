package tui

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// A second ctrl+c while a job runs quits at once and tells Run to stop
// the job's processes.
func TestSecondCtrlCWhileBusyForcesQuit(t *testing.T) {
	m, _ := newModel(t, sampleSnapshot(), 120, 36)
	m, _ = press(t, m, "A")
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = nm.(Model)
	if cmd != nil || m.forceQuit {
		t.Fatal("the first ctrl+c must only wait")
	}
	if !strings.Contains(ansi.Strip(m.View()), "ctrl+c again to stop it now") {
		t.Errorf("no hint for the second ctrl+c:\n%s", ansi.Strip(m.View()))
	}
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = nm.(Model)
	if cmd == nil || !m.forceQuit || !m.quitting {
		t.Fatalf("second ctrl+c: cmd %v forceQuit %v", cmd != nil, m.forceQuit)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("not a quit command")
	}
}

func TestWaitJobs(t *testing.T) {
	type run struct {
		running, force bool
		signal         bool // a signal arrives while waiting
		wantStop, fail bool
	}
	for name, c := range map[string]run{
		"idle":                      {},
		"idle, forced":              {force: true},
		"job ends by itself":        {running: true},
		"signal while waiting":      {running: true, signal: true, wantStop: true, fail: true},
		"forced from the dashboard": {running: true, force: true, wantStop: true, fail: true},
	} {
		t.Run(name, func(t *testing.T) {
			tr := newTracker()
			sigs := make(chan os.Signal, 1)
			sigs <- syscall.SIGTERM // the one that closed the dashboard: drained
			stopped := false
			if c.running {
				tr.begin()
				if !c.signal && !c.force {
					go func() { time.Sleep(50 * time.Millisecond); tr.end() }()
				} else if c.signal {
					go func() { time.Sleep(50 * time.Millisecond); sigs <- syscall.SIGTERM }()
				}
			}
			stop := func() error { stopped = true; tr.end(); return nil }
			err := waitJobs(tr, "syncing", sigs, c.force, stop)
			if stopped != c.wantStop {
				t.Errorf("stop called %v, want %v", stopped, c.wantStop)
			}
			if (err != nil) != c.fail {
				t.Errorf("err %v, want failure %v", err, c.fail)
			}
			if err != nil && !strings.Contains(err.Error(), "stopped syncing") {
				t.Errorf("error: %v", err)
			}
		})
	}
	// Stopping fails: still exits non-zero, after the grace period.
	tr := newTracker()
	tr.begin()
	err := waitJobs(tr, "x", make(chan os.Signal), true, func() error { return errors.New("no ps") })
	if err == nil {
		t.Error("forced quit reported success")
	}
}
