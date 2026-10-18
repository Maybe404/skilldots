package cli

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/targets"
)

func targetsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "targets",
		Short: i18n.T("List known agent targets and whether they exist here"),
		Long: i18n.T("List the built-in agent targets and the custom ones of skills.toml.\n\n" +
			"A custom target is used only after a person approves it on this machine\n" +
			"(skd targets trust <name>, at a terminal), and never when its directory is\n" +
			"outside your home directory, is your home directory, is or holds a\n" +
			"sensitive place (~/.ssh, ~/.aws, ~/Library/LaunchAgents, ~/.vscode, …) or the\n" +
			"library, is inside a built-in target's directory, or is, is inside or holds\n" +
			"another custom target's directory."),
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
				Active bool
				// Paused is an agent skills.toml lists in targets.disabled:
				// skd links nothing there until it is resumed.
				Paused     bool
				Approvable bool   `json:",omitempty"`
				Problem    string `json:",omitempty"`
			}
			var rows []row
			for _, t := range c.reg.All() {
				rows = append(rows, row{Name: t.Name, Path: t.Path, Mode: t.Mode, Present: fsutil.Exists(t.Path) || fsutil.Exists(filepath.Dir(t.Path)),
					Default: def[t.Name], Builtin: t.Builtin, Active: true, Paused: slices.Contains(c.lib.Manifest.Targets.Disabled, t.Name)})
			}
			for _, w := range c.reg.Withheld() {
				p := w.Real
				if p == "" {
					p = w.Path
				}
				rows = append(rows, row{Name: w.Name, Path: p, Mode: w.Mode, Default: def[w.Name], Paused: slices.Contains(c.lib.Manifest.Targets.Disabled, w.Name),
					Approvable: w.Approvable, Problem: w.Problem})
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
					d = accent(i18n.T("default"))
				}
				if r.Paused {
					flag, d = dim("‖"), dim(i18n.T("paused"))
				}
				if !r.Active {
					flag, d = warn("!"), warn(i18n.T("not used"))
					notes = append(notes, i18n.Sprintf("%s custom target %s is not used: %s", warn("!"), txt(r.Name), txt(r.Problem)))
				}
				t.Row(flag, txt(r.Name), d, dim(txt(r.Path)))
			}
			c.printf("%s\n", t.Render())
			for _, n := range notes {
				c.printf("%s\n", n)
			}
			if len(c.lib.Manifest.Targets.Disabled) > 0 {
				c.printf("%s\n", dim(i18n.T("a paused agent gets no links; its skills keep it selected (skd targets resume <agent>)")))
			}
			return nil
		},
	}
	cmd.AddCommand(trustCmd(), untrustCmd(), pauseCmd(), resumeCmd(), defaultTargetsCmd())
	return cmd
}

// pausedNote is what a header adds about paused agents ("" when none).
func pausedNote(paused []string) string {
	if len(paused) == 0 {
		return ""
	}
	return warn(i18n.Sprintf(" · paused: %s", strings.Join(txts(paused), ", ")))
}

func pauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause <agent>...",
		Short: i18n.T("Pause agents: unlink every skill from them, keeping which skills use them"),
		Long: i18n.T("Pause agents: skd takes out the links it made in their skill directories and\n" +
			"links nothing there until they are resumed. Every skill keeps the agents it\n" +
			"was given, so `skd targets resume` puts back exactly what was there. The\n" +
			"pause is kept in skills.toml (targets.disabled), so it reaches every machine\n" +
			"that uses the library. Nothing is deleted: what skd did not make stays."),
		Args: rangeArgs(1, 64),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			out, err := c.PauseAgents(args, true)
			if err != nil {
				return err
			}
			c.printf("%s %s\n", ok("✓"), i18n.Sprintf("paused %s", strings.Join(txts(args), ", ")))
			printApply(c, out)
			c.jsonOut(out)
			return nil
		},
	}
}

func resumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <agent>...",
		Short: i18n.T("Resume paused agents: link their skills again"),
		Args:  rangeArgs(1, 64),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			for _, a := range args {
				if !c.Paused(a) {
					return i18n.Errorf("%s is not paused (see `skd targets`)", a)
				}
			}
			out, err := c.PauseAgents(args, false)
			if err != nil {
				return err
			}
			c.printf("%s %s\n", ok("✓"), i18n.Sprintf("resumed %s", strings.Join(txts(args), ", ")))
			printApply(c, out)
			c.jsonOut(out)
			return nil
		},
	}
}

func defaultTargetsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default <agent>...",
		Short: i18n.T("Set the default agents: those a skill without agents of its own is linked into"),
		Long: i18n.T("Set targets.default of skills.toml: the agents a skill, pack or app without\n" +
			"agents of its own is linked into. Skills given their own agents keep them.\n" +
			"skd applies at once: skills leave the agents taken out of the list."),
		Args: rangeArgs(1, 64),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			out, err := c.SetDefaultAgents(args)
			if err != nil {
				return err
			}
			c.printf("%s %s\n", ok("✓"), i18n.Sprintf("default agents: %s", strings.Join(txts(args), ", ")))
			printApply(c, out)
			c.jsonOut(out)
			return nil
		},
	}
}

// personAtTerminal reports whether a person answers skd's questions:
// a terminal on stdin and stdout, and neither --yes nor --json, which
// say that nobody will.
func personAtTerminal() bool { return isTerminal() && !flags.yes && !flags.json }

func errNeedsPerson(w targets.Withheld) error {
	return i18n.Errorf("approving custom target %q lets skd write into %s; only a person at a terminal can do that (--yes and --json do not): ask the user to run `skd targets trust %s` in a terminal", w.Name, w.Real, w.Name)
}

func trustCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trust <name>",
		Short: i18n.T("Approve a custom target of skills.toml on this machine (a person at a terminal only)"),
		Long: i18n.T("Show where a custom target of skills.toml leads and, when you agree, let skd\n" +
			"write skills there on this machine. The approval names the target and its\n" +
			"directory (symbolic links resolved) and its mode (link or copy); when\n" +
			"skills.toml later points the target elsewhere or changes its mode it needs\n" +
			"approving again. A directory not named skills is approved only by typing\n" +
			"the target's name. Approvals are kept beside the\n" +
			"settings file, never in a library. Only a person at a terminal can approve:\n" +
			"without one, and with --yes or --json, skd refuses."),
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			name := args[0]
			if _, ok := c.lib.Manifest.Targets.Custom[name]; !ok {
				return i18n.Errorf("skills.toml has no custom target %q (see `skd targets`)", name)
			}
			w, held := c.reg.WithheldTarget(name)
			if !held {
				t, _ := c.reg.Get(name)
				c.printf("%s custom target %s is approved already: %s\n", ok("✓"), txt(name), txt(t.Path))
				c.jsonOut(map[string]any{"name": name, "path": t.Path, "approved": true})
				return nil
			}
			if !w.Approvable {
				return i18n.Errorf("custom target %q cannot be approved: %s", name, w.Problem)
			}
			if !personAtTerminal() {
				return errNeedsPerson(w)
			}
			yes, err := approveTarget(c.reg, w)
			if err != nil {
				return err
			}
			if !yes {
				return i18n.New("not approved; nothing changed")
			}
			c.printf("%s approved custom target %s → %s %s\n", ok("✓"), txt(name), txt(w.Real), dim(i18n.T("(skd apply links skills there)")))
			return nil
		},
	}
}

func untrustCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "untrust <name>",
		Short: i18n.T("Withdraw the approval of a custom target on this machine"),
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := targets.Revoke(args[0])
			if err != nil {
				return err
			}
			if n == 0 {
				say("%s\n", dim(i18n.Sprintf("custom target %s was not approved", txt(args[0]))))
			} else {
				say("%s custom target %s is no longer approved %s\n", ok("✓"), txt(args[0]),
					dim(i18n.T("(skd stops using it; what it linked there stays until you take the target out of skills.toml and run skd apply)")))
			}
			jsonOut(map[string]any{"name": args[0], "revoked": n})
			return nil
		},
	}
}

// approveTarget shows a person where the custom target w leads and asks
// whether skd may write there; on yes the approval is recorded. For a
// directory not named skills (targets.Unusual) the person is warned and
// must type the target's name: a y is not enough.
func approveTarget(reg *targets.Registry, w targets.Withheld) (bool, error) {
	lines := []string{
		bold(i18n.Sprintf("Custom target %s", txt(w.Name))),
		pad(i18n.T("skills.toml path"), 18) + txt(w.Path),
		pad(i18n.T("directory"), 18) + txt(w.Real),
		pad(i18n.T("mode"), 18) + txt(w.Mode),
		"",
		dim(map[bool]string{true: i18n.T("skd will copy skills into this directory, put back trash entries"), false: i18n.T("skd will link skills into this directory, put back trash entries")}[w.Mode == "copy"]),
		dim(i18n.T("that came from it, and with --force move what is in their way into the")),
		dim(i18n.T("library trash. Approve only a directory you meant for skills.")),
	}
	unusual := targets.Unusual(w.Real)
	if unusual {
		lines = append(lines, "",
			warn(i18n.T("! This directory is not named skills. Whatever reads or runs files from")),
			warn(i18n.T("  it (an editor, a shell, a service) would load what skd puts there.")))
	}
	say("%s\n", card(lines...))
	if unusual {
		var typed string
		err := ask(huh.NewInput().Title(i18n.Sprintf("Type %s to let skd write skills into %s (anything else refuses)", txt(w.Name), txt(w.Real))).Value(&typed))
		if err != nil || typed != w.Name {
			return false, nil
		}
		return true, reg.Approve(w, typed)
	}
	var yes bool
	err := ask(huh.NewConfirm().Title(i18n.Sprintf("Let skd write skills into %s?", txt(w.Real))).Affirmative(i18n.T("Approve")).Negative(i18n.T("No")).Value(&yes))
	if err != nil || !yes {
		return false, nil
	}
	return true, reg.Approve(w, "")
}

// reviewCustomTargets is what clone says about the cloned library's
// custom targets before it applies: each one a person may approve is
// asked about at a terminal, and skipped (with how to approve it later)
// without one. It reports whether any was approved.
func reviewCustomTargets(reg *targets.Registry) (approved bool, err error) {
	ws := reg.Withheld()
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
			yes, err := approveTarget(reg, w)
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
		say("  %s\n", dim(i18n.Sprintf("a person approves one at a terminal with `skd targets trust <name>` (%s), then runs `skd apply`", strings.Join(txts(later), ", "))))
	}
	return approved, nil
}
