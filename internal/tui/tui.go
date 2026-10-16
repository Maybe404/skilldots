// Package tui is the full-screen dashboard `skd` opens in a terminal: packs
// and your own skills on the left, the selected pack's skills on the right,
// actions applied in place. Reads go through engine, pack and library;
// writes repeat the CLI commands' steps on the same engine calls, and
// backups run `skd sync` itself.
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

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
	// Self is the skd executable used for `skd sync`; empty means
	// os.Executable().
	Self string
}

// Run opens the dashboard and blocks until the user quits. The terminal is
// restored on return.
func Run(o Options) error {
	quietGit(o.Root)
	ops := &libOps{root: o.Root, self: o.Self}
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
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	label := "the running operation"
	if fm, ok := final.(Model); ok {
		if fm.quitAfter && fm.status != "" {
			// ctrl+c during a job: say how it ended.
			fmt.Println(statusText(fm.statusLvl, fm.status))
		}
		if fm.busy != "" {
			label = fm.busy
		}
	}
	waitJobs(m.jobs, label, sigs)
	return err
}

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

// waitJobs blocks until no job runs, telling the user why. Signals that
// arrive meanwhile only repeat the notice: a git process interrupted from
// here would be left running, while a ctrl+c on the terminal reaches it
// directly and makes the job end sooner.
func waitJobs(t *tracker, label string, sigs <-chan os.Signal) {
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
		return
	default:
	}
	fmt.Fprintf(os.Stderr, "skd: waiting for %s to finish…\n", label)
	for {
		select {
		case <-idle:
			return
		case <-sigs:
			fmt.Fprintf(os.Stderr, "skd: still waiting for %s; it is not safe to stop half-way\n", label)
		}
	}
}

func statusText(l level, s string) string {
	icon := map[level]string{lvInfo: "·", lvOK: "✓", lvWarn: "!", lvErr: "✗"}[l]
	return icon + " " + clean(s)
}
