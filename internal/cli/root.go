// Package cli wires the skd command tree.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/targets"
	"github.com/Maybe404/skilldots/internal/tui"
)

// Version is set at build time.
var Version = "dev"

var flags struct {
	library string
	json    bool
	yes     bool
}

// ctx is everything a command needs once the library is open.
type ctx struct {
	lib *library.Library
	reg *targets.Registry
	// warned remembers warnings already shown, so a command that resolves
	// several times prints each one once.
	warned map[string]bool
}

// resolve computes the library's entries. Per-entry problems are printed
// as warnings on stderr (never stdout, which may carry JSON).
func (c *ctx) resolve() ([]engine.Entry, error) {
	entries, warnings, err := engine.Resolve(c.lib, c.reg)
	for _, w := range warnings {
		if c.warned == nil {
			c.warned = map[string]bool{}
		}
		if !c.warned[w] {
			c.warned[w] = true
			fmt.Fprintf(os.Stderr, "%s %s\n", warn("warning:"), w)
		}
	}
	return entries, err
}

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
	lib, err := library.Open(root)
	if err != nil {
		return nil, err
	}
	custom := map[string]struct{ Path, Mode string }{}
	for n, c := range lib.Manifest.Targets.Custom {
		custom[n] = struct{ Path, Mode string }{c.Path, c.Mode}
	}
	reg, err := targets.NewRegistry(custom)
	if err != nil {
		return nil, err
	}
	engine.SetRegistry(reg)
	return &ctx{lib: lib, reg: reg}, nil
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isTerminal() {
				return errNoTTY
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
// tui.MinWidth columns wide and a library must exist (setup stays in the
// menu).
func dashboardLibrary(simple bool) (string, bool) {
	if simple || !term.IsTerminal(int(os.Stdout.Fd())) {
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
	self, _ := os.Executable()
	return tui.Run(tui.Options{Root: root, Version: Version, Self: self, Theme: tui.Theme{
		Accent: cAccent, Cyan: cCyan, Muted: cMuted, Faint: cFaint, OK: cOK, Warn: cWarn, Err: cErr, Text: cText,
		Ink: lipgloss.Color("#1A1B26"), // the button ink in theme()
	}})
}
