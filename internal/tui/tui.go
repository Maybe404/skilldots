// Package tui is the full-screen dashboard `skd` opens in a terminal: packs
// and your own skills on the left, the selected pack's skills on the right,
// actions applied in place. Every write goes through internal/app, the
// same flows the CLI commands run.
package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Maybe404/skilldots/internal/safetext"
)

// termOut and termErr are what the dashboard prints on the terminal
// outside its screen (after it closed: how a job ended, what skd waits
// for). A job's label can name a trash entry or a pack, so this text is
// cleaned like the screen is (View): no escape sequence, control
// character or bidi control of its own reaches the terminal.
var (
	termOut io.Writer = cleanWriter{os.Stdout}
	termErr io.Writer = cleanWriter{os.Stderr}
)

// cleanWriter writes each Write (one whole message) cleaned by
// safetext.Screen, without colour: the messages have none.
type cleanWriter struct{ w io.Writer }

func (c cleanWriter) Write(p []byte) (int, error) {
	if _, err := io.WriteString(c.w, safetext.Screen(string(p), false)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// MinWidth is the narrowest terminal the dashboard is offered on; the CLI
// falls back to the question-and-answer menu below it.
const MinWidth = 90

// Options configures Run.
type Options struct {
	// Root is the library directory.
	Root    string
	Version string
	Theme   Theme
	// Input is what the user typed before the dashboard started that is
	// no longer in the terminal's input queue (read while skd asked the
	// terminal for its background); it is handled first.
	Input []byte
}

// Run opens the dashboard and blocks until the user quits. The terminal is
// restored on return.
func Run(o Options) error {
	quietGit(o.Root)
	ops := &libOps{root: o.Root}
	snap, err := ops.Load()
	if err != nil {
		return err
	}
	// From here on SIGINT, SIGTERM and SIGHUP never kill skd outright:
	// bubbletea turns the first SIGINT or SIGTERM into a quit, and below
	// skd waits for the running job so no git or `skd sync` is left behind
	// on its own.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	// A hangup (the terminal window closed, `tmux kill-session`) leaves
	// nobody to answer: the dashboard is closed and everything skd started
	// is stopped below rather than skd dying and leaving its git (in a
	// session of its own, out of the hangup's reach) to run on.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)

	m := New(ops, snap, o.Theme, o.Version)
	m.replay = replayKeys(o.Input)
	p := tea.NewProgram(m, tea.WithAltScreen())
	return runDashboard(p, m, sigs, hup, func() error { return stopDescendants(os.Getpid(), stopGrace) })
}

// program is what runDashboard needs of a bubbletea program.
type program interface {
	Run() (tea.Model, error)
	Kill()
}

// runDashboard runs p, the dashboard built from m, then winds down what it
// started: a background check is cancelled, a running job waited for (see
// waitJobs, which stop and sigs are for). A signal on hup closes the
// dashboard at once and stops the job instead of waiting for it, and the
// error says the terminal hung up.
func runDashboard(p program, m Model, sigs, hup <-chan os.Signal, stop func() error) error {
	ran := make(chan struct{})
	hungUp := make(chan bool, 1)
	go func() {
		select {
		case <-hup:
			hungUp <- true
			p.Kill()
		case <-ran:
			hungUp <- false
		}
	}()
	final, err := p.Run()
	close(ran)
	label := "the running operation"
	force := false
	if <-hungUp {
		err = errors.New("the terminal hung up")
		force = true
	}
	if fm, ok := final.(Model); ok {
		if fm.quitAfter && fm.status != "" {
			// ctrl+c during a job: say how it ended.
			fmt.Fprintln(termOut, statusText(fm.statusLvl, fm.status))
		}
		if fm.busy != "" {
			label = fm.busy
		}
		force = force || fm.forceQuit
	}
	// A background upstream check is only a check: stop it rather than
	// wait for a slow network, and wait just long enough for its git to
	// exit (it removes its lock files on the way out).
	if !m.checks.stop(checkGrace) {
		fmt.Fprintln(termErr, "skd: the upstream check did not stop in time; its git may still be running")
	}
	if werr := waitJobs(m.jobs, label, sigs, force, stop); werr != nil {
		return werr
	}
	return err
}

// stopGrace is how long the job's processes get to exit after SIGTERM
// (git removes its lock files) before they are killed, and how long Run
// then waits for the job itself to notice.
const stopGrace = 2 * time.Second

// checkGrace is how long quitting waits for a cancelled upstream check.
const checkGrace = 2 * time.Second

// quietGit keeps git from prompting on the terminal the dashboard owns: a
// credential or passphrase prompt would draw over it and hang. git fails
// instead and the error shows in the status line. ssh gets BatchMode only
// when the user has not chosen an ssh command of their own: GIT_SSH_COMMAND
// would override a core.sshCommand (a different key, a proxy, a wrapper).
func quietGit(root string) {
	os.Setenv("GIT_TERMINAL_PROMPT", "0")
	if os.Getenv("GIT_SSH_COMMAND") != "" || os.Getenv("GIT_SSH") != "" || gitSSHCommand(root) != "" {
		return
	}
	os.Setenv("GIT_SSH_COMMAND", "ssh -o BatchMode=yes")
}

// gitSSHCommand is core.sshCommand as git sees it in the library
// (system, global, include and repository config), or "".
func gitSSHCommand(root string) string {
	cmd := exec.Command("git", "config", "--get", "core.sshCommand")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// waitJobs blocks until no job runs, telling the user why. A ctrl+c on
// the terminal reaches git directly and usually ends the job sooner. A
// signal that arrives while waiting, or force (ctrl+c pressed twice in
// the dashboard), stops every process skd started instead: their process
// group is skd's own, so the tree is walked and signalled explicitly (see
// stopDescendants). The job is then abandoned half-way and waitJobs
// returns an error so skd exits non-zero.
func waitJobs(t *tracker, label string, sigs <-chan os.Signal, force bool, stop func() error) error {
drain: // signals delivered while the dashboard ran already made it quit
	for {
		select {
		case <-sigs:
		default:
			break drain
		}
	}
	idle := t.close()
	select {
	case <-idle:
		return nil
	default:
	}
	if !force {
		fmt.Fprintf(termErr, "skd: waiting for %s to finish… (ctrl+c again to stop it now)\n", label)
		select {
		case <-idle:
			return nil
		case <-sigs:
		}
	}
	fmt.Fprintf(termErr, "skd: stopping %s…\n", label)
	if err := stop(); err != nil {
		fmt.Fprintf(termErr, "skd: could not stop its processes: %v\n", err)
	}
	select {
	case <-idle:
	case <-time.After(stopGrace):
	}
	return fmt.Errorf("stopped %s before it finished; run `skd` again to check the library (`git status` in it shows what was left half-way)", label)
}

func statusText(l level, s string) string {
	icon := map[level]string{lvInfo: "·", lvOK: "✓", lvWarn: "!", lvErr: "✗"}[l]
	return icon + " " + clean(s)
}
