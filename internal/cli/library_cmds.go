package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/targets"
)

// SelfRepo ships the skill that teaches agents to use skd.
const (
	SelfRepo  = app.SelfRepo
	SelfSkill = app.SelfSkill
)

func initCmd() *cobra.Command {
	var from string
	var defaults []string
	var noSkill bool
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: i18n.T("Create a library (default ~/.skilldots)"),
		Args:  maximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := library.DefaultRoot()
			if len(args) == 1 {
				root = args[0]
			} else if from != "" {
				root = from
			}
			root, _ = filepath.Abs(root)
			if len(defaults) == 0 {
				defaults = app.DefaultTargets()
			}
			if err := app.InitLibrary(root, defaults); err != nil {
				return err
			}
			say("%s initialised library at %s\n  %s\n", ok("✓"), root, dim(i18n.Sprintf("default targets: %s  (skd enable <skill> --agent <name> adds another agent)", strings.Join(defaults, ", "))))
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
			rep, out, err := c.InstallSelfSkill()
			if rep == nil && err != nil {
				say("  %s could not install the %s skill (%s); run `skd add %s --skill %s --agent '*'` later\n", warn("!"), SelfSkill, errText(err), SelfRepo, SelfSkill)
				result["skillError"] = err.Error()
				jsonOut(result)
				return nil
			}
			if err != nil {
				return err
			}
			say("  %s\n", dim(i18n.Sprintf("installed the %s skill so agents on this machine know how to use skd", SelfSkill)))
			printApply(c, out)
			result["skill"], result["apply"] = true, out
			jsonOut(result)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", i18n.T("turn an existing directory into the library"))
	cmd.Flags().StringSliceVar(&defaults, "targets", nil, i18n.T("default targets (default: agents)"))
	cmd.Flags().BoolVar(&noSkill, "no-skill", false, i18n.T("do not install the skilldots skill for agents"))
	return cmd
}

func listCmd() *cobra.Command {
	var all, flat bool
	var agent string
	cmd := &cobra.Command{
		Use:   "list [pack]",
		Short: i18n.T("Show packs and skills (one line per pack unless expanded)"),
		Args:  maximumNArgs(1),
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
					// A skill's name, given where a pack is expected.
					if e, err := c.Lookup(entries, args[0]); err == nil && e != nil {
						if e.Kind == engine.KindVendor {
							return i18n.Errorf("no pack %q; %s is a skill of %s: `skd list %s` shows it with its pack, `skd path %s` where its files are", args[0], e.LinkName, e.Origin, e.Origin, args[0])
						}
						return i18n.Errorf("no pack %q; %s is a skill (%s): `skd list --all` shows it, `skd path %s` where its files are", args[0], e.LinkName, engine.Describe(*e), args[0])
					}
					return i18n.Errorf("no pack %q (`skd list` lists the packs; `skd list --all` every skill)", args[0])
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
	cmd.Flags().BoolVar(&all, "all", false, i18n.T("expand every pack"))
	cmd.Flags().BoolVar(&flat, "flat", false, i18n.T("one line per skill"))
	cmd.Flags().StringVar(&agent, "agent", "", i18n.T("show what one agent target receives"))
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
	c.printf("%s  %s\n", brand(), dim(tildePath(c.lib.Root)))
	c.printf("%s\n", cyan(plural(len(m.Packs), "pack"))+dim(" · ")+cyan(plural(len(entries), "skill"))+dim(" · ")+cyan(i18n.Sprintf("%d in agents", linked))+dim(i18n.Sprintf(" · agents: %s", strings.Join(m.Targets.Default, ", ")))+pausedNote(m.Targets.Disabled))
	rec := c.RecordedOutdated()
	if line := updatesLine(rec, time.Now()); line != "" {
		c.printf("%s\n", line)
	}
	if line := shelvedLine(entries); line != "" {
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
			sel := i18n.Sprintf("%d of %d", len(mine), total)
			if p.UsesAll() {
				sel = i18n.Sprintf("all %d", total)
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

// shelvedLine names the skills whose update is shelved, and what
// resolves them; "" when none is.
func shelvedLine(entries []engine.Entry) string {
	var names []string
	for _, e := range entries {
		if e.Kind == engine.KindVendor && (e.Shelved != "" || e.Resolving) {
			names = append(names, txt(e.LinkName))
		}
	}
	if len(names) == 0 {
		return ""
	}
	return warn(i18n.Sprintf("! %d shelved with conflicts: %s", len(names), strings.Join(names, ", "))) + dim(i18n.T(" · skd resolve <skill>"))
}

// updatesLine summarises the last recorded upstream check (skd outdated,
// or the dashboard's) for the overview and the menu: "" when there is
// none. It never touches the network.
func updatesLine(rec *app.Outdated, now time.Time) string {
	if rec == nil {
		return ""
	}
	when := i18n.Sprintf("checked %s", app.Ago(rec.Checked, now))
	if rec.Stale(now) {
		when += i18n.T(" · stale, skd outdated checks again")
	}
	var line string
	switch n := rec.Updates(); n {
	case 0:
		line = ok("✓") + " " + dim(i18n.Sprintf("no updates · %s", when))
	case 1:
		line = accent(i18n.T("↑ 1 update available")) + dim(i18n.Sprintf(" · %s · skd update", when))
	default:
		line = accent(i18n.Sprintf("↑ %d updates available", n)) + dim(i18n.Sprintf(" · %s · skd update", when))
	}
	if n := len(rec.Failed); n > 0 {
		line += dim(" · ") + warn(i18n.Sprintf("%d could not be checked", n))
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
		return dim(i18n.T("default"))
	}
	if slices.Contains(*p.Targets, targets.AllInstalled) {
		return i18n.Sprintf("%s (every installed agent)", joinOr(engine.ExpandTargets(*p.Targets, c.Lib.Manifest.Targets.Default, c.Reg), "none"))
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
			name += " " + warn(i18n.T("missing"))
		}
		if e.Conflict != "" {
			name += " " + warn(i18n.T("conflict"))
		}
		switch {
		case e.Resolving:
			name += " " + accent(i18n.T("resolving"))
		case e.Shelved != "":
			name += " " + warn(i18n.T("shelved"))
		}
		if e.ClaudeDisabled {
			name += " " + warn(i18n.T("off in Claude Code"))
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
			c.printf("%s\n", dim(i18n.T(ClaudeOffNote)))
			break
		}
	}
}

// ClaudeOffNote explains "off in Claude Code" under a list that shows it.
var ClaudeOffNote = i18n.M("off in Claude Code: turned off in Claude Code's own skill settings; skd still links it. " +
	"To keep it out of Claude entirely, remove the claude target: skd disable <skill> --agent claude (e in the dashboard).")

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
	c.printf("\n%s %s\n", accent(title), dim(i18n.T(note)))
	printEntries(c, mine)
}

func targetList(e engine.Entry) string {
	if len(e.Targets) == 0 {
		return dim(i18n.T("off"))
	}
	names := make([]string, len(e.Targets))
	for i, t := range e.Targets {
		names[i] = t.Name
	}
	out := strings.Join(names, ", ")
	if len(e.ViaShared) > 0 {
		// Agents that read ~/.agents/skills get it there.
		out += dim(" " + i18n.Sprintf("(+ %s through agents)", strings.Join(e.ViaShared, ", ")))
	}
	return out
}

func listByAgent(c *ctx, entries []engine.Entry, agent string) error {
	t, err := c.reg.Get(agent)
	if err != nil {
		return err
	}
	c.printf("%s %s\n", bold(t.Name), dim(tildePath(t.Path)))
	tb := newTable("SKILL", "FROM")
	for _, e := range entries {
		for _, et := range e.Targets {
			if et.Name == t.Name {
				tb.Row(e.LinkName, dim(txt(app.Origin(e))))
			}
		}
		if slices.Contains(e.ViaShared, t.Name) {
			tb.Row(e.LinkName, dim(txt(app.Origin(e))+" "+i18n.T("(through agents)")))
		}
	}
	c.printf("%s\n", tb.Render())
	return nil
}

func applyCmd() *cobra.Command {
	var dry, force bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: i18n.T("Make agent directories match the manifest"),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			return runApply(c, dry, force)
		},
	}
	cmd.Flags().BoolVarP(&dry, "dry-run", "n", false, i18n.T("show the plan without changing anything"))
	cmd.Flags().BoolVar(&force, "force", false, i18n.T("replace unmanaged or modified entries (they are moved to the library trash, not deleted)"))
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
	return refusedError(out)
}

// refusedError is the error `skd apply` exits with when some planned
// change was not made (out.Refused; under --json listed as "refused" in
// the document already printed): the agent directories do not match the
// manifest yet.
func refusedError(out *app.ApplyOutcome) error {
	if n := len(out.Refused); n > 0 {
		return i18n.Errorf("%d planned change(s) not made (listed above); the agent directories do not match skills.toml yet", n)
	}
	return nil
}

// printApply prints what an apply did (or, for a dry run, would do).
func printApply(c *ctx, out *app.ApplyOutcome) {
	if !flags.json {
		printOps(c, out.Plan)
	}
	printApplySummary(c, out)
}

// printOps lists the changes of plan, one line each: a link made (+),
// taken out (−), replaced (relink), a record forgotten, or a skip (!).
func printOps(c *ctx, plan engine.Plan) {
	marks := map[string]string{"create": "+", "replace": i18n.T("relink"), "remove": "−", "skip": "!", "forget": i18n.T("forget")}
	styles := map[string]func(string) string{"create": ok, "replace": accent, "remove": bad, "skip": warn, "forget": dim}
	markW, targetW := 1, 10
	for _, op := range plan.Ops {
		if op.Action == "keep" || marks[op.Action] == "" {
			continue
		}
		markW = max(markW, i18n.Width(marks[op.Action]))
		targetW = max(targetW, i18n.Width(txt(op.Target.Name)))
	}
	for _, op := range plan.Ops {
		if op.Action == "keep" || marks[op.Action] == "" {
			continue
		}
		line := fmt.Sprintf("  %s %s %s", styles[op.Action](pad(marks[op.Action], markW)), dim(pad(txt(op.Target.Name), targetW)), txt(op.Name))
		if op.Reason != "" {
			line += "  " + dim(txt(op.Reason))
		}
		c.printf("%s\n", line)
	}
}

// printApplySummary prints what printApply says after the changes: agents
// not installed here, what was moved aside or refused, and the totals.
func printApplySummary(c *ctx, out *app.ApplyOutcome) {
	plan := out.Plan
	counts := plan.Counts()
	for _, a := range sortedKeys(out.Absent) {
		c.printf("  %s %s\n", dim("·"), dim(i18n.Sprintf("%s is not installed on this machine: %s not linked there", txt(a), plural(len(out.Absent[a]), "skill"))))
	}
	if !plan.Changes() {
		msg := i18n.Sprintf("up to date, %d links", counts["keep"])
		if counts["skip"] > 0 {
			msg += i18n.Sprintf(", %d skipped", counts["skip"])
		}
		c.printf("%s %s\n", ok("✓"), dim(msg))
		return
	}
	summary := out.ChangeSummary() + " · " + i18n.Sprintf("kept %d", counts["keep"])
	if counts["skip"] > 0 {
		summary += " · " + i18n.Sprintf("skipped %d", counts["skip"])
	}
	if out.DryRun {
		c.printf("%s\n", dim(i18n.Sprintf("dry run · %s", summary)))
		return
	}
	for _, b := range out.Backups {
		c.printf("  %s %s\n", dim(i18n.T("moved aside")), txt(tildeIn(b)))
	}
	for _, r := range out.Refused {
		c.printf("  %s %s\n", warn(i18n.T("! skipped")), txt(tildeIn(r)))
	}
	if out.Applied && len(out.Refused) > 0 {
		c.printf("%s %s\n", warn("!"), dim(i18n.Sprintf("applied · %s · %d not changed", summary, len(out.Refused))))
	} else if out.Applied {
		c.printf("%s %s\n", ok("✓"), dim(i18n.Sprintf("applied · %s", summary)))
	}
}

func newCmd() *cobra.Command {
	var private bool
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: i18n.T("Scaffold a skill of your own in skills/ (or private/)"),
		Args:  exactArgs(1),
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
	cmd.Flags().BoolVar(&private, "private", false, i18n.T("create under private/ (never pushed)"))
	return cmd
}

func useCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <dir>",
		Short: i18n.T("Point skd at an existing library directory from now on"),
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			if !fsutil.Exists(filepath.Join(root, manifest.FileName)) {
				return i18n.Errorf("%s has no %s", root, manifest.FileName)
			}
			if err := app.UseLibrary(root); err != nil {
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
		Short: i18n.T("What skd has set aside: list, restore, or empty (empty is for a person at a terminal)"),
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
				c.printf("%s\n", dim(i18n.T("trash is empty")))
				return nil
			}
			t := newTable("ID", "SKILL", "WHY", "WAS AT", "WHEN")
			var refused []string
			for _, it := range items {
				id := txt(it.ID)
				if !it.Restorable {
					refused = append(refused, i18n.Sprintf("%s %s cannot be restored: %s", warn("!"), id, txt(it.Problem)))
					id += " " + warn("!")
				}
				t.Row(id, strings.Join(txts(it.Skills), ", "), dim(trashReason(it)), dim(txt(tildePath(it.Origin))), dim(it.When.Format("2006-01-02 15:04")))
			}
			c.printf("%s\n", t.Render())
			if len(refused) > 0 {
				c.printf("%s\n", dim(i18n.T("! marks an entry skd does not restore; why:")))
			}
			for _, r := range refused {
				c.printf("%s\n", r)
			}
			c.printf("%s\n", dim(i18n.T("skd trash restore <id|skill>   ·   skd trash empty")))
			return nil
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "restore <id|skill>",
		Short: i18n.T("Put a parked directory back where it came from, selected and linked again"),
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			out, err := c.RestoreEntry(args[0])
			if out != nil && out.Apply != nil {
				printApply(c, out.Apply)
			}
			if err != nil {
				return err
			}
			c.printf("%s restored to %s\n", ok("✓"), txt(tildePath(out.Path)))
			if out.Selected {
				c.printf("  %s\n", dim(i18n.Sprintf("selected again in skills.toml: %s", strings.Join(txts(out.Skills), ", "))))
			}
			if out.Hint != "" {
				c.printf("%s %s\n", warn("!"), txt(out.Hint))
			}
			c.jsonOut(out)
			return nil
		},
	}, &cobra.Command{
		Use:   "empty",
		Short: i18n.T("Permanently delete everything in the trash (interactive only; --yes is ignored)"),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			if !isTerminal() {
				return i18n.Errorf("skd trash empty is irreversible and only runs interactively; ask the user to run it")
			}
			items, _ := c.Trash()
			if len(items) == 0 {
				c.printf("%s\n", dim(i18n.T("trash is empty")))
				c.jsonOut(map[string]any{"deleted": 0})
				return nil
			}
			c.printf("%s %s\n", bad(i18n.T("permanently delete")), plural(len(items), "parked directory"))
			var sure bool
			if err := ask(huh.NewConfirm().Title(i18n.Sprintf("Delete %d item(s) forever?", len(items))).Affirmative(i18n.T("Delete")).Negative(i18n.T("Keep")).Value(&sure)); err != nil || !sure {
				return i18n.Errorf("cancelled")
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

// trashReason says why a trash entry was set aside.
func trashReason(it engine.TrashEntry) string {
	if r := app.TrashReason(it); r != "" {
		return r
	}
	return "·"
}
