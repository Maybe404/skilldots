// Package tui is the full-screen dashboard `skd` opens in a terminal: packs
// and your own skills on the left, the selected pack's skills on the right,
// actions applied in place. Reads go through engine, pack and library;
// writes repeat the CLI commands' steps on the same engine calls, and
// backups run `skd sync` itself.
package tui

import (
	"os"

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
	// git must never prompt on the terminal the dashboard owns: a
	// credential or passphrase prompt would draw over it and hang. Fail
	// instead; the error shows in the status line.
	os.Setenv("GIT_TERMINAL_PROMPT", "0")
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		os.Setenv("GIT_SSH_COMMAND", "ssh -o BatchMode=yes")
	}
	ops := &libOps{root: o.Root, self: o.Self}
	snap, err := ops.Load()
	if err != nil {
		return err
	}
	p := tea.NewProgram(New(ops, snap, o.Theme, o.Version), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
