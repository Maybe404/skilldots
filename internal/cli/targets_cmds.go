package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/targets"
)

func targetsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "targets",
		Short: "List known agent targets and whether they exist here",
		Long: "List the built-in agent targets and the custom ones of skills.toml.\n\n" +
			"A custom target is used only after a person approves it on this machine\n" +
			"(skd targets trust <name>, at a terminal), and never when its directory is\n" +
			"outside your home directory, is your home directory, or is or holds a\n" +
			"sensitive place (~/.ssh, ~/.gnupg, ~/.aws, ~/Library/LaunchAgents, …).",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			def := map[string]bool{}
			for _, d := range c.lib.Manifest.Targets.Default {
				def[d] = true
			}
			type row struct {
				Name, Path, Mode string
				Present, Default bool
				Builtin          bool
				// Active is false for a custom target skd does not use;
				// Problem says why, and Approvable whether a person can
				// approve it (skd targets trust).
				Active     bool
				Approvable bool   `json:",omitempty"`
				Problem    string `json:",omitempty"`
			}
			var rows []row
			for _, t := range c.reg.All() {
				rows = append(rows, row{t.Name, t.Path, t.Mode, fsutil.Exists(t.Path) || fsutil.Exists(filepath.Dir(t.Path)), def[t.Name], t.Builtin, true, false, ""})
			}
			for _, w := range c.reg.Withheld() {
				p := w.Real
				if p == "" {
					p = w.Path
				}
				rows = append(rows, row{Name: w.Name, Path: p, Mode: w.Mode, Default: def[w.Name], Approvable: w.Approvable, Problem: w.Problem})
			}
			if c.jsonOut(rows) {
				return nil
			}
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].Active != rows[j].Active {
					return rows[i].Active
				}
				if rows[i].Present != rows[j].Present {
					return rows[i].Present
				}
				return rows[i].Name < rows[j].Name
			})
			t := newTable("", "TARGET", "", "PATH")
			var notes []string
			for _, r := range rows {
				flag, d := dim("·"), ""
				if r.Present {
					flag = ok("●")
				}
				if r.Default {
					d = accent("default")
				}
				if !r.Active {
					flag, d = warn("!"), warn("not used")
					notes = append(notes, fmt.Sprintf("%s custom target %s is not used: %s", warn("!"), txt(r.Name), txt(r.Problem)))
				}
				t.Row(flag, txt(r.Name), d, dim(txt(r.Path)))
			}
			c.printf("%s\n", t.Render())
			for _, n := range notes {
				c.printf("%s\n", n)
			}
			return nil
		},
	}
	cmd.AddCommand(trustCmd(), untrustCmd())
	return cmd
}

// personAtTerminal reports whether a person answers skd's questions:
// a terminal on stdin and stdout, and neither --yes nor --json, which
// say that nobody will.
func personAtTerminal() bool { return isTerminal() && !flags.yes && !flags.json }

func errNeedsPerson(w targets.Withheld) error {
	return fmt.Errorf("approving custom target %q lets skd write into %s; only a person at a terminal can do that (--yes and --json do not): ask the user to run `skd targets trust %s` in a terminal", w.Name, w.Real, w.Name)
}

func trustCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trust <name>",
		Short: "Approve a custom target of skills.toml on this machine (a person at a terminal only)",
		Long: "Show where a custom target of skills.toml leads and, when you agree, let skd\n" +
			"write skills there on this machine. The approval names the target and its\n" +
			"directory (symbolic links resolved); when skills.toml later points the\n" +
			"target elsewhere it needs approving again. Approvals are kept beside the\n" +
			"settings file, never in a library. Only a person at a terminal can approve:\n" +
			"without one, and with --yes or --json, skd refuses.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			name := args[0]
			if _, ok := c.lib.Manifest.Targets.Custom[name]; !ok {
				return fmt.Errorf("skills.toml has no custom target %q (see `skd targets`)", name)
			}
			w, held := c.reg.WithheldTarget(name)
			if !held {
				t, _ := c.reg.Get(name)
				c.printf("%s custom target %s is approved already: %s\n", ok("✓"), txt(name), txt(t.Path))
				c.jsonOut(map[string]any{"name": name, "path": t.Path, "approved": true})
				return nil
			}
			if !w.Approvable {
				return fmt.Errorf("custom target %q cannot be approved: %s", name, w.Problem)
			}
			if !personAtTerminal() {
				return errNeedsPerson(w)
			}
			yes, err := approveTarget(w)
			if err != nil {
				return err
			}
			if !yes {
				return errors.New("not approved; nothing changed")
			}
			c.printf("%s approved custom target %s → %s %s\n", ok("✓"), txt(name), txt(w.Real), dim("(skd apply links skills there)"))
			return nil
		},
	}
}

func untrustCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "untrust <name>",
		Short: "Withdraw the approval of a custom target on this machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := targets.Revoke(args[0])
			if err != nil {
				return err
			}
			if n == 0 {
				say("%s\n", dim(fmt.Sprintf("custom target %s was not approved", txt(args[0]))))
			} else {
				say("%s custom target %s is no longer approved %s\n", ok("✓"), txt(args[0]),
					dim("(skd stops using it; what it linked there stays until you take the target out of skills.toml and run skd apply)"))
			}
			jsonOut(map[string]any{"name": args[0], "revoked": n})
			return nil
		},
	}
}

// approveTarget shows a person where the custom target w leads and asks
// whether skd may write there; on yes the approval is recorded.
func approveTarget(w targets.Withheld) (bool, error) {
	say("%s\n", card(
		bold("Custom target "+txt(w.Name)),
		"skills.toml path  "+txt(w.Path),
		"directory         "+txt(w.Real),
		"mode              "+txt(w.Mode),
		"",
		dim("skd will "+map[bool]string{true: "copy", false: "link"}[w.Mode == "copy"]+" skills into this directory, put back trash entries"),
		dim("that came from it, and with --force move what is in their way into the"),
		dim("library trash. Approve only a directory you meant for skills."),
	))
	var yes bool
	err := ask(huh.NewConfirm().Title(fmt.Sprintf("Let skd write skills into %s?", txt(w.Real))).Affirmative("Approve").Negative("No").Value(&yes))
	if err != nil || !yes {
		return false, nil
	}
	return true, targets.Approve(w.Name, w.Path, w.Real)
}

// reviewCustomTargets is what clone says about the cloned library's
// custom targets before it applies: each one a person may approve is
// asked about at a terminal, and skipped (with how to approve it later)
// without one. It reports whether any was approved.
func reviewCustomTargets(ws []targets.Withheld) (approved bool, err error) {
	if len(ws) == 0 {
		return false, nil
	}
	say("%s this library defines %s: directories outside the agents' own skill directories that skd would write skills into\n",
		warn("!"), plural(len(ws), "custom target"))
	person := personAtTerminal()
	var later []string
	for _, w := range ws {
		where := txt(w.Path)
		if w.Real != "" && w.Real != w.Path {
			where += " → " + txt(w.Real)
		}
		switch {
		case !w.Approvable:
			say("  %s %s (%s) is not used: %s\n", warn("!"), txt(w.Name), where, txt(w.Problem))
		case !person:
			say("  %s %s (%s) is skipped: not approved on this machine\n", warn("!"), txt(w.Name), where)
			later = append(later, w.Name)
		default:
			yes, err := approveTarget(w)
			if err != nil {
				return approved, err
			}
			if yes {
				approved = true
				say("  %s approved %s\n", ok("✓"), txt(w.Name))
			} else {
				say("  %s %s is skipped\n", dim("·"), txt(w.Name))
				later = append(later, w.Name)
			}
		}
	}
	if len(later) > 0 {
		say("  %s\n", dim("a person approves one at a terminal with `skd targets trust <name>` ("+strings.Join(txts(later), ", ")+"), then runs `skd apply`"))
	}
	return approved, nil
}
