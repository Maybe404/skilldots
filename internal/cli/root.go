// Package cli wires the skd command tree.
package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/targets"
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
	out io.Writer
}

func (c *ctx) resolve() ([]engine.Entry, error) { return engine.Resolve(c.lib, c.reg) }

// printf writes human-readable progress. Under --json it goes to stderr so
// stdout stays parseable.
func (c *ctx) printf(format string, a ...any) {
	if flags.json {
		fmt.Fprintf(os.Stderr, format, a...)
		return
	}
	fmt.Fprintf(c.out, format, a...)
}

// jsonOut prints v as JSON when --json is set and reports whether it did.
func (c *ctx) jsonOut(v any) bool {
	if !flags.json {
		return false
	}
	enc := json.NewEncoder(c.out)
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
	return &ctx{lib: lib, reg: reg, out: os.Stdout}, nil
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
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

func isTerminal() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// Root builds the command tree.
func Root() *cobra.Command {
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
			return runMenu()
		},
	}
	root.PersistentFlags().StringVar(&flags.library, "library", "", "library directory (default: $"+library.EnvLibrary+", nearest skills.toml, the one set by `skd use`, or ~/.skilldots)")
	root.PersistentFlags().BoolVar(&flags.json, "json", false, "machine-readable output")
	root.PersistentFlags().BoolVarP(&flags.yes, "yes", "y", false, "answer yes to prompts")
	root.AddCommand(initCmd(), addCmd(), listCmd(), applyCmd(), updateCmd(), enableCmd(), disableCmd(),
		removeCmd(), purgeCmd(), forkCmd(), pathCmd(), targetsCmd(), newCmd(), cloneCmd(), syncCmd(), useCmd(), showCmd())
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
