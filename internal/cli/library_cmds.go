package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// SelfRepo ships the skill that teaches agents to use skd.
const (
	SelfRepo  = "Maybe404/skilldots"
	SelfSkill = "skilldots"
)

const gitignore = `.DS_Store
# Machine-local skills: a separate git repository that is never pushed with this one.
/private/
# Where skd parks anything it takes out of the way. Empty with: skd trash empty
/.trash/
# Temporary files of a save that was interrupted; the next save removes them.
.*.tmp-*
`

func initCmd() *cobra.Command {
	var from string
	var defaults []string
	var noSkill bool
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Create a library (default ~/.skilldots)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := library.DefaultRoot()
			if len(args) == 1 {
				root = args[0]
			} else if from != "" {
				root = from
			}
			root, _ = filepath.Abs(root)
			if fsutil.Exists(filepath.Join(root, manifest.FileName)) {
				return fmt.Errorf("%s already has a %s", root, manifest.FileName)
			}
			// init records the library in the settings file at the end:
			// fail before creating anything when it cannot.
			if _, err := config.Load(); err != nil {
				return fmt.Errorf("%w; init records the library there, so fix or remove it first (nothing was created)", err)
			}
			for _, d := range []string{"", library.DirSkills, library.DirVendor, library.DirPrivate} {
				if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
					return err
				}
			}
			if len(defaults) == 0 {
				defaults = detectDefaults()
			}
			m := manifest.New(defaults)
			if err := m.Save(filepath.Join(root, manifest.FileName)); err != nil {
				return err
			}
			l := &lock.Lock{Version: 1, Packs: map[string]*lock.Pack{}, Links: map[string]*lock.Link{}}
			if err := l.Save(filepath.Join(root, lock.FileName)); err != nil {
				return err
			}
			gi := filepath.Join(root, ".gitignore")
			if !fsutil.Exists(gi) {
				os.WriteFile(gi, []byte(gitignore), 0o644)
			} else {
				b, _ := os.ReadFile(gi)
				for _, line := range []string{"/private/", "/.trash/", ".*.tmp-*"} {
					if !strings.Contains(string(b), line) {
						b = append(b, []byte("\n"+line+"\n")...)
					}
				}
				os.WriteFile(gi, b, 0o644)
			}
			if err := library.EnsureGit(root); err != nil {
				return err
			}
			priv := filepath.Join(root, library.DirPrivate)
			if err := library.EnsureGit(priv); err != nil {
				return err
			}
			if err := library.Remember(root); err != nil {
				return err
			}
			say("%s initialised library at %s\n  %s\n", ok("✓"), root, dim("default targets: "+strings.Join(defaults, ", ")+"  (skd enable <skill> --agent claude to add more)"))
			result := map[string]any{"library": root, "defaults": defaults, "skill": false}
			if noSkill {
				jsonOut(result)
				return nil
			}
			// Teach the machine's agents how to drive skd. Best effort: offline
			// init still succeeds. Open the new library explicitly: the
			// current directory may sit inside another one.
			flags.library = root
			c, err := open()
			if err != nil {
				return err
			}
			rep, out, err := c.Subscribe(&manifest.Pack{Repo: SelfRepo, Use: []string{SelfSkill}})
			if rep == nil && err != nil {
				say("  %s could not fetch the %s skill (%s); run `skd add %s --skill %s` later\n", warn("!"), SelfSkill, errText(err), SelfRepo, SelfSkill)
				result["skillError"] = err.Error()
				jsonOut(result)
				return nil
			}
			if err != nil {
				return err
			}
			say("  %s\n", dim("installed the "+SelfSkill+" skill so agents on this machine know how to use skd"))
			printApply(c, out)
			result["skill"], result["apply"] = true, out
			jsonOut(result)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "turn an existing directory into the library")
	cmd.Flags().StringSliceVar(&defaults, "targets", nil, "default targets (default: agents)")
	cmd.Flags().BoolVar(&noSkill, "no-skill", false, "do not install the skilldots skill for agents")
	return cmd
}

// detectDefaults returns the cross-agent directory only; nearly every agent
// reads ~/.agents/skills, and users opt specific agents in with --agent.
func detectDefaults() []string { return []string{"agents"} }

func listCmd() *cobra.Command {
	var all, flat bool
	var agent string
	cmd := &cobra.Command{
		Use:   "list [pack]",
		Short: "Show packs and skills (one line per pack unless expanded)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			entries, err := c.resolve()
			if err != nil {
				return err
			}
			if c.jsonOut(entries) {
				return nil
			}
			if agent != "" {
				return listByAgent(c, entries, agent)
			}
			if len(args) == 1 {
				p := c.lib.Manifest.FindPack(args[0])
				if p == nil {
					return fmt.Errorf("no pack %q", args[0])
				}
				printPack(c, p, entries, true)
				return nil
			}
			if flat {
				printEntries(c, entries)
				return nil
			}
			printOverview(c, entries, all)
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "expand every pack")
	cmd.Flags().BoolVar(&flat, "flat", false, "one line per skill")
	cmd.Flags().StringVar(&agent, "agent", "", "show what one agent target receives")
	return cmd
}

func printOverview(c *ctx, entries []engine.Entry, expand bool) {
	m := c.lib.Manifest
	linked := 0
	for _, e := range entries {
		if len(e.Targets) > 0 {
			linked++
		}
	}
	home, _ := os.UserHomeDir()
	c.printf("%s  %s\n", brand(), dim(strings.Replace(c.lib.Root, home, "~", 1)))
	c.printf("%s\n", cyan(plural(len(m.Packs), "pack"))+dim(" · ")+cyan(plural(len(entries), "skill"))+dim(" · ")+cyan(fmt.Sprintf("%d linked", linked))+dim(" · agents: "+strings.Join(m.Targets.Default, ", ")))
	rec := c.RecordedOutdated()
	if line := updatesLine(rec, time.Now()); line != "" {
		c.printf("%s\n", line)
	}
	c.printf("\n")

	if len(m.Packs) > 0 {
		t := newTable("PACK", "SKILLS", "COMMIT", "TARGETS")
		for _, p := range m.Packs {
			lp := c.lib.Lock.Packs[p.Repo]
			total, mine := 0, packEntries(entries, p.Repo)
			if lp != nil {
				total = len(lp.Skills)
			}
			commit := dim("—")
			if lp != nil && lp.Commit != "" {
				commit = dim(lp.Commit[:7])
			}
			label := p.Repo
			if p.Alias != "" {
				label += dim(" (" + p.Alias + ")")
			}
			if rec != nil {
				if st := rec.Find(p.Repo); st != nil && st.HasUpdates() {
					label += " " + accent("↑")
				}
			}
			sel := fmt.Sprintf("%d of %d", len(mine), total)
			if p.UsesAll() {
				sel = fmt.Sprintf("all %d", total)
			}
			t.Row(label, sel, commit, packTargets(c, p))
		}
		c.printf("%s\n", t.Render())
		if expand {
			for _, p := range m.Packs {
				c.printf("\n%s\n", accent(p.DisplayName()))
				printEntries(c, packEntries(entries, p.Repo))
			}
		}
	}
	printGroup(c, entries, engine.KindSkill, "skills/", "yours, published with the library")
	printGroup(c, entries, engine.KindPrivate, "private/", "this machine only")
	printGroup(c, entries, engine.KindApp, "apps", "shipped with installed applications")
}

// updatesLine summarises the last recorded upstream check (skd outdated,
// or the dashboard's) for the overview and the menu: "" when there is
// none. It never touches the network.
func updatesLine(rec *app.Outdated, now time.Time) string {
	if rec == nil {
		return ""
	}
	when := "checked " + app.Ago(rec.Checked, now)
	if rec.Stale(now) {
		when += " · stale, skd outdated checks again"
	}
	var line string
	switch n := rec.Updates(); n {
	case 0:
		line = ok("✓") + " " + dim("no updates · "+when)
	case 1:
		line = accent("↑ 1 update available") + dim(" · "+when+" · skd update")
	default:
		line = accent(fmt.Sprintf("↑ %d updates available", n)) + dim(" · "+when+" · skd update")
	}
	if n := len(rec.Failed); n > 0 {
		line += dim(" · ") + warn(fmt.Sprintf("%d could not be checked", n))
	}
	return line
}

func packEntries(entries []engine.Entry, repo string) []engine.Entry {
	var out []engine.Entry
	for _, e := range entries {
		if e.Kind == engine.KindVendor && e.Origin == repo {
			out = append(out, e)
		}
	}
	return out
}

func packTargets(c *ctx, p *manifest.Pack) string {
	if p.Targets == nil {
		return dim("default")
	}
	return joinOr(*p.Targets, "off")
}

func printPack(c *ctx, p *manifest.Pack, entries []engine.Entry, expand bool) {
	lp := c.lib.Lock.Packs[p.Repo]
	commit := "—"
	if lp != nil && lp.Commit != "" {
		commit = lp.Commit[:7]
	}
	c.printf("%s %s\n", bold(p.Repo), dim("@ "+commit))
	printEntries(c, packEntries(entries, p.Repo))
}

func printEntries(c *ctx, entries []engine.Entry) {
	t := newTable("SKILL", "CATEGORY", "TARGETS", "DESCRIPTION")
	for _, e := range entries {
		name := e.LinkName
		if e.LinkName != e.Name {
			name += dim(" ← " + e.Name)
		}
		if e.Missing {
			name += " " + warn("missing")
		}
		if e.Conflict != "" {
			name += " " + warn("conflict")
		}
		if e.ClaudeDisabled {
			name += " " + warn("off in Claude Code")
		}
		cat := e.Category
		if cat == "" {
			cat = dim("·")
		}
		t.Row(name, cat, targetList(e), dim(truncate(e.Description, 48)))
	}
	c.printf("%s\n", t.Render())
	for _, e := range entries {
		if e.ClaudeDisabled {
			c.printf("%s\n", dim(ClaudeOffNote))
			break
		}
	}
}

// ClaudeOffNote explains "off in Claude Code" under a list that shows it.
const ClaudeOffNote = "off in Claude Code: turned off in Claude Code's own skill settings; skd still links it. " +
	"To keep it out of Claude entirely, remove the claude target: skd disable <skill> --agent claude (e in the dashboard)."

func printGroup(c *ctx, entries []engine.Entry, kind engine.Kind, title, note string) {
	var mine []engine.Entry
	for _, e := range entries {
		if e.Kind == kind {
			mine = append(mine, e)
		}
	}
	if len(mine) == 0 {
		return
	}
	c.printf("\n%s %s\n", accent(title), dim(note))
	printEntries(c, mine)
}

func targetList(e engine.Entry) string {
	if len(e.Targets) == 0 {
		return dim("off")
	}
	names := make([]string, len(e.Targets))
	for i, t := range e.Targets {
		names[i] = t.Name
	}
	return strings.Join(names, ", ")
}

func listByAgent(c *ctx, entries []engine.Entry, agent string) error {
	t, err := c.reg.Get(agent)
	if err != nil {
		return err
	}
	c.printf("%s %s\n", bold(t.Name), dim(t.Path))
	tb := newTable("SKILL", "FROM")
	for _, e := range entries {
		for _, et := range e.Targets {
			if et.Name == t.Name {
				tb.Row(e.LinkName, dim(engine.Describe(e)))
			}
		}
	}
	c.printf("%s\n", tb.Render())
	return nil
}

func applyCmd() *cobra.Command {
	var dry, force bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Make agent directories match the manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			return runApply(c, dry, force)
		},
	}
	cmd.Flags().BoolVarP(&dry, "dry-run", "n", false, "show the plan without changing anything")
	cmd.Flags().BoolVar(&force, "force", false, "replace unmanaged or modified entries (they are moved to the library trash, not deleted)")
	return cmd
}

// runApply applies and, under --json, prints the outcome as the command's
// only JSON document.
func runApply(c *ctx, dry, force bool) error {
	out, err := c.Apply(app.ApplyOptions{DryRun: dry, Force: force})
	if out != nil {
		printApply(c, out)
	}
	if err != nil {
		return err
	}
	c.jsonOut(out)
	return nil
}

// printApply prints what an apply did (or, for a dry run, would do).
func printApply(c *ctx, out *app.ApplyOutcome) {
	plan := out.Plan
	counts := plan.Counts()
	if !flags.json {
		for _, op := range plan.Ops {
			if op.Action == "keep" {
				continue
			}
			mark := map[string]string{"create": ok("+"), "replace": accent("↻"), "remove": bad("−"), "skip": warn("!"), "forget": dim("?")}[op.Action]
			line := fmt.Sprintf("  %s %-10s %s", mark, dim(txt(op.Target.Name)), txt(op.Name))
			if op.Reason != "" {
				line += "  " + dim(txt(op.Reason))
			}
			c.printf("%s\n", line)
		}
	}
	if !plan.Changes() {
		msg := fmt.Sprintf("up to date, %d linked", counts["keep"])
		if counts["skip"] > 0 {
			msg += fmt.Sprintf(", %d skipped", counts["skip"])
		}
		c.printf("%s %s\n", ok("✓"), dim(msg))
		return
	}
	summary := fmt.Sprintf("+%d ↻%d −%d · %d kept · %d skipped", counts["create"], counts["replace"], counts["remove"], counts["keep"], counts["skip"])
	if out.DryRun {
		c.printf("%s\n", dim("dry run · "+summary))
		return
	}
	for _, b := range out.Backups {
		c.printf("  %s %s\n", dim("moved aside"), txt(b))
	}
	if out.Applied {
		c.printf("%s %s\n", ok("✓"), dim("applied · "+summary))
	}
}

func targetsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "targets",
		Short: "List known agent targets and whether they exist here",
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
			}
			var rows []row
			for _, t := range c.reg.All() {
				rows = append(rows, row{t.Name, t.Path, t.Mode, fsutil.Exists(t.Path) || fsutil.Exists(filepath.Dir(t.Path)), def[t.Name], t.Builtin})
			}
			if c.jsonOut(rows) {
				return nil
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].Present != rows[j].Present {
					return rows[i].Present
				}
				return rows[i].Name < rows[j].Name
			})
			t := newTable("", "TARGET", "", "PATH")
			for _, r := range rows {
				flag, d := dim("·"), ""
				if r.Present {
					flag = ok("●")
				}
				if r.Default {
					d = accent("default")
				}
				t.Row(flag, r.Name, d, dim(r.Path))
			}
			c.printf("%s\n", t.Render())
			return nil
		},
	}
}

func newCmd() *cobra.Command {
	var private bool
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a skill of your own in skills/ (or private/)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			out, err := c.New(args[0], private)
			if err != nil {
				return err
			}
			printApply(c, out.Apply)
			c.jsonOut(map[string]any{"created": out.Name, "path": out.Path, "apply": out.Apply})
			return nil
		},
	}
	cmd.Flags().BoolVar(&private, "private", false, "create under private/ (never pushed)")
	return cmd
}

func useCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <dir>",
		Short: "Point skd at an existing library directory from now on",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			if !fsutil.Exists(filepath.Join(root, manifest.FileName)) {
				return fmt.Errorf("%s has no %s", root, manifest.FileName)
			}
			if err := library.Remember(root); err != nil {
				return err
			}
			say("%s skd now uses %s %s\n", ok("✓"), root, dim("("+library.ConfigPath()+")"))
			jsonOut(map[string]any{"library": root, "config": library.ConfigPath()})
			return nil
		},
	}
}

func trashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trash",
		Short: "What skd has set aside: list, restore, or empty (empty is for a person at a terminal)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			items, err := c.Trash()
			if err != nil {
				return err
			}
			if c.jsonOut(items) {
				return nil
			}
			if len(items) == 0 {
				c.printf("%s\n", dim("trash is empty"))
				return nil
			}
			home, _ := os.UserHomeDir()
			t := newTable("ID", "FROM", "WHEN")
			var refused []string
			for _, it := range items {
				id := txt(it.ID)
				if !it.Restorable {
					refused = append(refused, fmt.Sprintf("%s %s cannot be restored: %s", warn("!"), id, txt(it.Problem)))
					id += " " + warn("!")
				}
				t.Row(id, dim(txt(strings.Replace(it.Origin, home, "~", 1))), dim(it.When.Format("2006-01-02 15:04")))
			}
			c.printf("%s\n", t.Render())
			for _, r := range refused {
				c.printf("%s\n", r)
			}
			c.printf("%s\n", dim("skd trash restore <id>   ·   skd trash empty"))
			return nil
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "restore <id>",
		Short: "Put a parked directory back where it came from",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			dest, err := c.Restore(args[0])
			if err != nil {
				return err
			}
			c.printf("%s restored to %s\n", ok("✓"), txt(dest))
			c.jsonOut(map[string]any{"id": args[0], "restored": dest})
			return nil
		},
	}, &cobra.Command{
		Use:   "empty",
		Short: "Permanently delete everything in the trash (interactive only; --yes is ignored)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			if !isTerminal() {
				return fmt.Errorf("skd trash empty is irreversible and only runs interactively; ask the user to run it")
			}
			items, _ := c.Trash()
			if len(items) == 0 {
				c.printf("%s\n", dim("trash is empty"))
				c.jsonOut(map[string]any{"deleted": 0})
				return nil
			}
			c.printf("%s %s\n", bad("permanently delete"), plural(len(items), "parked directory"))
			var sure bool
			if err := ask(huh.NewConfirm().Title(fmt.Sprintf("Delete %d item(s) forever?", len(items))).Affirmative("Delete").Negative("Keep").Value(&sure)); err != nil || !sure {
				return fmt.Errorf("cancelled")
			}
			n, err := c.EmptyTrash()
			if err != nil {
				return err
			}
			c.printf("%s deleted %d\n", ok("✓"), n)
			c.jsonOut(map[string]any{"deleted": n})
			return nil
		},
	})
	return cmd
}
