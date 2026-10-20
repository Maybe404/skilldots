// Package cli wires the skd command tree.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/safetext"
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
	lang    string
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
		i18n.Fprintf(stderr, format, a...)
		return
	}
	i18n.Fprintf(stdout, format, a...)
}

// txt and errText are text from outside skd (upstream names and
// directories, file names, library files, git's output) as it may be
// printed: no control characters or bidi controls (safetext). Every
// warning and error goes through them; so does anything else that
// carries such text.
func txt(s string) string      { return safetext.Clean(s, 0) }
func errText(err error) string { return safetext.Lines(err.Error()) }
func txts(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = txt(v)
	}
	return out
}

// logLine prints the progress messages of the shared app flows.
func logLine(l app.Level, msg string) {
	msg = safetext.Lines(msg)
	switch l {
	case app.Problem:
		fmt.Fprintln(stderr, tag(warn, i18n.T("warning:"), msg))
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
	writeJSON(os.Stdout, v)
	return true
}

// writeJSON writes v as indented JSON. Beyond what encoding/json escapes
// (U+0000 to U+001F, U+2028, U+2029), DEL, the C1 controls and the bidi
// controls are written as \u escapes too (safetext.JSON): the document
// decodes to the same values, and `skd … --json` shown on a terminal
// (piped to less, printed by an agent) cannot reorder or drive it.
func writeJSON(w io.Writer, v any) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := w.Write(safetext.JSON(b.Bytes()))
	return err
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
	for _, n := range s.Notices() {
		logLine(app.Problem, n)
	}
	return &ctx{Session: s, lib: s.Lib, reg: s.Reg}, nil
}

// confirm asks a yes/no question unless --yes was given.
func confirm(prompt string) bool {
	if flags.yes {
		return true
	}
	if !isTerminal() {
		fmt.Fprintln(stderr, i18n.T("refusing to proceed without a terminal; pass --yes"))
		return false
	}
	var yes bool
	if err := ask(huh.NewConfirm().Title(i18n.T(prompt)).Affirmative(i18n.T("Yes")).Negative(i18n.T("No")).Value(&yes)); err != nil {
		return false
	}
	return yes
}

// isTerminal reports whether skd can ask a person: stdin and stdout are
// both terminals. With stdout piped (skd update | cat) a prompt would be
// drawn where nobody sees it and wait forever, so such runs behave as
// without a terminal: they do what --yes does, or refuse.
func isTerminal() bool { return stdinIsTerminal() && stdoutIsTerminal() }

// Root builds the command tree.
func Root() *cobra.Command {
	var simple bool
	root := &cobra.Command{
		Use:           "skd",
		Short:         i18n.T("skilldots — declarative skill management for AI agents"),
		Long:          i18n.T("skd keeps every skill your agents use in one git-backed library and links them into each agent's skill directory."),
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := setupLang(); err != nil {
				return err
			}
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
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.Args = rootArgs
	root.SetFlagErrorFunc(flagError)
	root.Flags().BoolVar(&simple, "simple", false, i18n.T("use the question-and-answer menu instead of the full-screen dashboard"))
	root.PersistentFlags().StringVar(&flags.library, "library", "", i18n.Sprintf("the library's `dir` (default: $%s, nearest skills.toml, the one set by skd use, or ~/.skilldots)", library.EnvLibrary))
	root.PersistentFlags().BoolVar(&flags.json, "json", false, i18n.T("machine-readable output"))
	root.PersistentFlags().BoolVarP(&flags.yes, "yes", "y", false, i18n.T("answer yes to prompts"))
	root.PersistentFlags().StringVar(&flags.lang, "lang", "", i18n.Sprintf("language of skd's own text: en, zh or auto (default auto: the system's; also $%s, then the lang setting)", i18n.Env))
	root.PersistentFlags().StringVar(&flags.theme, "theme", "", i18n.Sprintf("colours: light, dark or auto (default auto: ask the terminal; also $%s)", EnvTheme))
	root.AddCommand(initCmd(), addCmd(), listCmd(), applyCmd(), updateCmd(), outdatedCmd(), enableCmd(), disableCmd(),
		removeCmd(), purgeCmd(), forkCmd(), pathCmd(), targetsCmd(), newCmd(), cloneCmd(), syncCmd(), useCmd(), showCmd(), scanCmd(), adoptCmd(), trashCmd(), configCmd(), historyCmd(), editCmd())
	localizeCobra(root)
	return root
}

// Main runs the CLI and returns an exit code.
func Main() int {
	// What the screen writers hold of a character split between writes
	// is printed before exit (as U+FFFD: it will not be completed now).
	defer flushOutput()
	// The language is chosen before the command tree is built: its help
	// texts are made with it. setupLang checks the flag again once cobra
	// has parsed it.
	applyLang(scanArgs(os.Args[1:]))
	err := Root().Execute()
	if w := settingsWarning(err); w != "" {
		fmt.Fprintln(stderr, w)
	}
	if err != nil {
		fmt.Fprintln(stderr, label(i18n.T("error:"), errText(err)))
		return 1
	}
	return 0
}

// settingsWarning is the one line that says the settings file could not
// be read and its settings were not used, after a command that ran
// without them; "" when the file is fine or when err already says it
// (finding the library, init, config).
func settingsWarning(err error) string {
	var bad *config.InvalidError
	if settingsErr == nil || errors.As(err, &bad) {
		return ""
	}
	return label(i18n.T("warning:"), i18n.Sprintf("%s; none of its settings (editor, theme, …) were used", errText(settingsErr)))
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
