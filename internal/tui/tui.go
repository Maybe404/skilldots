// Package tui is the full-screen dashboard `skd` opens in a terminal: packs
// and your own skills on the left, the selected pack's skills on the right,
// actions applied in place. Every write goes through internal/app, the
// same flows the CLI commands run.
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

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
	// From here on SIGINT and SIGTERM never kill skd outright: bubbletea
	// turns the first one into a quit, and below skd waits for the running
	// job so no git or `skd sync` is left behind on its own.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)

	m := New(ops, snap, o.Theme, o.Version)
	m.replay = replayKeys(o.Input)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	label := "the running operation"
	force := false
	if fm, ok := final.(Model); ok {
		if fm.quitAfter && fm.status != "" {
			// ctrl+c during a job: say how it ended.
			fmt.Println(statusText(fm.statusLvl, fm.status))
		}
		if fm.busy != "" {
			label = fm.busy
		}
		force = fm.forceQuit
	}
	stop := func() error { return stopDescendants(os.Getpid(), stopGrace) }
	if werr := waitJobs(m.jobs, label, sigs, force, stop); werr != nil {
		return werr
	}
	return err
}

// stopGrace is how long the job's processes get to exit after SIGTERM
// (git removes its lock files) before they are killed, and how long Run
// then waits for the job itself to notice.
const stopGrace = 2 * time.Second

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
		fmt.Fprintf(os.Stderr, "skd: waiting for %s to finish… (ctrl+c again to stop it now)\n", label)
		select {
		case <-idle:
			return nil
		case <-sigs:
		}
	}
	fmt.Fprintf(os.Stderr, "skd: stopping %s…\n", label)
	if err := stop(); err != nil {
		fmt.Fprintf(os.Stderr, "skd: could not stop its processes: %v\n", err)
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
