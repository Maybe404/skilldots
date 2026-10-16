// Package cli wires the skd command tree.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/targets"
	"github.com/Maybe404/skilldots/internal/termbg"
	"github.com/Maybe404/skilldots/internal/tui"
)

// Version is set at build time.
var Version = "dev"

var flags struct {
	library string
	json    bool
	yes     bool
	theme   string
}

// ctx is everything a command needs once the library is open.
type ctx struct {
	*app.Session
	lib *library.Library
	reg *targets.Registry
}

// resolve computes the library's entries. Per-entry problems are printed
// once each as warnings on stderr (never stdout, which may carry JSON).
func (c *ctx) resolve() ([]engine.Entry, error) { return c.Resolve() }

// printf writes human-readable progress. Under --json it goes to stderr so
// stdout stays parseable.
func (c *ctx) printf(format string, a ...any) { say(format, a...) }

// say is printf for code that has no ctx yet (init, clone, use).
func say(format string, a ...any) {
	if flags.json {
		fmt.Fprintf(os.Stderr, format, a...)
		return
	}
	fmt.Fprintf(os.Stdout, format, a...)
}

// logLine prints the progress messages of the shared app flows.
func logLine(l app.Level, msg string) {
	switch l {
	case app.Problem:
		fmt.Fprintf(os.Stderr, "%s %s\n", warn("warning:"), msg)
	case app.Done:
		say("%s %s\n", ok("✓"), msg)
	case app.Warn:
		say("%s %s\n", warn("!"), msg)
	case app.Fail:
		say("%s %s\n", bad("✗"), msg)
	case app.Detail:
		say("  %s\n", dim(msg))
	default:
		say("%s\n", dim(msg))
	}
}

// jsonOut prints v as JSON when --json is set and reports whether it did.
// Every command prints at most one JSON document on stdout.
func (c *ctx) jsonOut(v any) bool { return jsonOut(v) }

func jsonOut(v any) bool {
	if !flags.json {
		return false
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
	return true
}

func open() (*ctx, error) {
	root, err := library.Locate(flags.library)
	if err != nil {
		return nil, err
	}
	s, err := app.Open(root)
	if err != nil {
		return nil, err
	}
	s.Log = logLine
	return &ctx{Session: s, lib: s.Lib, reg: s.Reg}, nil
}

// confirm asks a yes/no question unless --yes was given.
func confirm(prompt string) bool {
	if flags.yes {
		return true
	}
	if !isTerminal() {
		fmt.Fprintln(os.Stderr, "refusing to proceed without a terminal; pass --yes")
		return false
	}
	var yes bool
	if err := ask(huh.NewConfirm().Title(prompt).Affirmative("Yes").Negative("No").Value(&yes)); err != nil {
		return false
	}
	return yes
}

func isTerminal() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// Root builds the command tree.
func Root() *cobra.Command {
	var simple bool
	root := &cobra.Command{
		Use:           "skd",
		Short:         "skilldots — declarative skill management for AI agents",
		Long:          "skd keeps every skill your agents use in one git-backed library and links them into each agent's skill directory.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return setupTheme(flags.theme)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isTerminal() {
				return errNoTTY
			}
			// A library named explicitly must exist: falling through to
			// the setup menu would offer to create one somewhere else.
			if flags.library != "" {
				if _, err := library.Locate(flags.library); err != nil {
					return err
				}
			}
			if lib, ok := dashboardLibrary(simple); ok {
				return runDashboard(lib)
			}
			return runMenu()
		},
	}
	root.Flags().BoolVar(&simple, "simple", false, "use the question-and-answer menu instead of the full-screen dashboard")
	root.PersistentFlags().StringVar(&flags.library, "library", "", "library directory (default: $"+library.EnvLibrary+", nearest skills.toml, the one set by `skd use`, or ~/.skilldots)")
	root.PersistentFlags().BoolVar(&flags.json, "json", false, "machine-readable output")
	root.PersistentFlags().BoolVarP(&flags.yes, "yes", "y", false, "answer yes to prompts")
	root.PersistentFlags().StringVar(&flags.theme, "theme", "", "colours: light, dark or auto (default auto: ask the terminal; also $"+EnvTheme+")")
	root.AddCommand(initCmd(), addCmd(), listCmd(), applyCmd(), updateCmd(), enableCmd(), disableCmd(),
		removeCmd(), purgeCmd(), forkCmd(), pathCmd(), targetsCmd(), newCmd(), cloneCmd(), syncCmd(), useCmd(), showCmd(), scanCmd(), adoptCmd(), trashCmd())
	return root
}

// Main runs the CLI and returns an exit code.
func Main() int {
	if err := Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// dashboardLibrary reports whether `skd` should open the full-screen
// dashboard, and on which library: stdout must be a terminal at least
// tui.MinWidth columns wide and able to draw it (not TERM=dumb), and a
// library must exist (setup stays in the menu).
func dashboardLibrary(simple bool) (string, bool) {
	if simple || os.Getenv("TERM") == "dumb" || !term.IsTerminal(int(os.Stdout.Fd())) {
		return "", false
	}
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err != nil || w < tui.MinWidth {
		return "", false
	}
	root, err := library.Locate(flags.library)
	if err != nil {
		return "", false
	}
	return root, true
}

func runDashboard(root string) error {
	return tui.Run(tui.Options{Root: root, Version: Version, Theme: dashboardTheme(pal), Input: termbg.Unread()})
}

// dashboardTheme is p as the dashboard's palette.
func dashboardTheme(p palette) tui.Theme {
	return tui.Theme{Accent: p.Accent, Cyan: p.Cyan, Muted: p.Muted, Faint: p.Faint, OK: p.OK, Warn: p.Warn, Err: p.Err, Text: p.Text,
		Line: p.Line, Ink: p.Ink, Light: p.Light}
}
