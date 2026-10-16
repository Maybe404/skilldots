package tui

import (
	"errors"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeProgram stands in for the dashboard: Run returns at once (the user
// quit) or, when block is set, only once Kill is called.
type fakeProgram struct {
	model  Model
	block  bool
	killed chan struct{}
	once   sync.Once
}

func (p *fakeProgram) Run() (tea.Model, error) {
	if !p.block {
		return p.model, nil
	}
	<-p.killed
	return p.model, tea.ErrProgramKilled
}

func (p *fakeProgram) Kill() { p.once.Do(func() { close(p.killed) }) }

// dashboard is a model with a background check and a job running: the
// check ends when cancelled, the job when stop is called.
func dashboard(t *testing.T) (m Model, checkDone <-chan struct{}, stop func() error, stopped <-chan struct{}) {
	t.Helper()
	m = Model{checks: &checkControl{}, jobs: newTracker()}
	ctx := m.checks.begin()
	cd := make(chan struct{})
	go func() {
		defer m.checks.wg.Done()
		<-ctx.Done()
		close(cd)
	}()
	if !m.jobs.begin() {
		t.Fatal("job refused")
	}
	st := make(chan struct{})
	var once sync.Once
	stop = func() error {
		once.Do(func() {
			close(st)
			go m.jobs.end()
		})
		return nil
	}
	return m, cd, stop, st
}

func closed(c <-chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

// A hangup closes the dashboard, cancels the background check, stops the
// running job without waiting for it, and skd exits non-zero.
func TestHangupStopsEverything(t *testing.T) {
	m, checkDone, stop, stopped := dashboard(t)
	p := &fakeProgram{model: m, block: true, killed: make(chan struct{})}
	hup := make(chan os.Signal, 1)
	errc := make(chan error, 1)
	go func() { errc <- runDashboard(p, m, make(chan os.Signal, 1), hup, stop) }()
	hup <- syscall.SIGHUP
	var err error
	select {
	case err = <-errc:
	case <-time.After(10 * time.Second):
		t.Fatal("still running 10s after the hangup")
	}
	if err == nil {
		t.Fatal("hangup: no error, skd would exit 0")
	}
	if !closed(p.killed) {
		t.Fatal("dashboard not closed")
	}
	if !closed(checkDone) {
		t.Fatal("background check not cancelled")
	}
	if !closed(stopped) {
		t.Fatal("running job waited for instead of stopped")
	}
}

// With nothing running, a hangup still ends in an error.
func TestHangupIdleExitsNonZero(t *testing.T) {
	m := Model{checks: &checkControl{}, jobs: newTracker()}
	p := &fakeProgram{model: m, block: true, killed: make(chan struct{})}
	hup := make(chan os.Signal, 1)
	hup <- syscall.SIGHUP
	stop := func() error { return errors.New("stop called with nothing running") }
	errc := make(chan error, 1)
	go func() { errc <- runDashboard(p, m, make(chan os.Signal, 1), hup, stop) }()
	var err error
	select {
	case err = <-errc:
	case <-time.After(10 * time.Second):
		t.Fatal("still running 10s after the hangup")
	}
	if err == nil || !strings.Contains(err.Error(), "hung up") {
		t.Fatalf("idle hangup: %v", err)
	}
}

// Without a hangup, quitting waits for the job (it is not stopped) and
// returns no error once it ends.
func TestQuitWithoutHangupWaitsForJob(t *testing.T) {
	m, checkDone, stop, stopped := dashboard(t)
	p := &fakeProgram{model: m}
	go func() {
		time.Sleep(200 * time.Millisecond)
		m.jobs.end()
	}()
	if err := runDashboard(p, m, make(chan os.Signal, 1), make(chan os.Signal, 1), stop); err != nil {
		t.Fatalf("quit: %v", err)
	}
	if !closed(checkDone) {
		t.Fatal("background check not cancelled on quit")
	}
	if closed(stopped) {
		t.Fatal("job stopped on an ordinary quit")
	}
}
