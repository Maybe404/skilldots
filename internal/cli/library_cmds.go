package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

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
				for _, line := range []string{"/private/", "/.trash/"} {
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
			p := &manifest.Pack{Repo: SelfRepo, Use: []string{SelfSkill}}
			c.lib.Manifest.Packs = append(c.lib.Manifest.Packs, p)
			if _, err := engine.Sync(c.lib, p); err != nil {
				c.lib.Manifest.Packs = c.lib.Manifest.Packs[:len(c.lib.Manifest.Packs)-1]
				say("  %s could not fetch the %s skill (%v); run `skd add %s --skill %s` later\n", warn("!"), SelfSkill, err, SelfRepo, SelfSkill)
				result["skillError"] = err.Error()
				jsonOut(result)
				return nil
			}
			if err := c.lib.Save(); err != nil {
				return err
			}
			say("  %s\n", dim("installed the "+SelfSkill+" skill so agents on this machine know how to use skd"))
			out, err := doApply(c, false, false, nil)
			if err != nil {
				return err
			}
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
	c.printf("%s\n\n", cyan(plural(len(m.Packs), "pack"))+dim(" · ")+cyan(plural(len(entries), "skill"))+dim(" · ")+cyan(fmt.Sprintf("%d linked", linked))+dim(" · agents: "+strings.Join(m.Targets.Default, ", ")))

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
		cat := e.Category
		if cat == "" {
			cat = dim("·")
		}
		t.Row(name, cat, targetList(e), dim(truncate(e.Description, 48)))
	}
	c.printf("%s\n", t.Render())
}

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

// applyOutcome is the machine-readable result of an apply.
type applyOutcome struct {
	Applied bool            `json:"applied"`
	DryRun  bool            `json:"dryRun,omitempty"`
	Changes []engine.Change `json:"changes"`
	Kept    int             `json:"kept"`
	Backups []string        `json:"backups,omitempty"`
}

// runApply applies and, under --json, prints the outcome as the command's
// only JSON document. Commands that report more call doApply instead.
func runApply(c *ctx, dry, force bool) error {
	out, err := doApply(c, dry, force, nil)
	if err != nil {
		return err
	}
	c.jsonOut(out)
	return nil
}

// doApply plans and applies, printing human-readable progress. force
// forces every entry; forceKeys ("<target>/<name>") forces only those.
func doApply(c *ctx, dry, force bool, forceKeys map[string]bool) (*applyOutcome, error) {
	entries, err := c.resolve()
	if err != nil {
		return nil, err
	}
	plan := engine.MakePlanForce(c.lib, entries, func(key string) bool { return force || forceKeys[key] })
	counts := plan.Counts()
	out := &applyOutcome{DryRun: dry, Changes: plan.ChangeList(), Kept: counts["keep"]}
	if !flags.json {
		for _, op := range plan.Ops {
			if op.Action == "keep" {
				continue
			}
			mark := map[string]string{"create": ok("+"), "replace": accent("↻"), "remove": bad("−"), "skip": warn("!"), "forget": dim("?")}[op.Action]
			line := fmt.Sprintf("  %s %-10s %s", mark, dim(op.Target.Name), op.Name)
			if op.Reason != "" {
				line += "  " + dim(op.Reason)
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
		return out, nil
	}
	summary := fmt.Sprintf("+%d ↻%d −%d · %d kept · %d skipped", counts["create"], counts["replace"], counts["remove"], counts["keep"], counts["skip"])
	if dry {
		c.printf("%s\n", dim("dry run · "+summary))
		return out, nil
	}
	res, err := engine.Apply(c.lib, plan)
	if saveErr := c.lib.Save(); saveErr != nil && err == nil {
		err = saveErr
	}
	for _, b := range res.Backups {
		c.printf("  %s %s\n", dim("moved aside"), b)
	}
	if err != nil {
		return nil, err
	}
	out.Applied, out.Backups = true, res.Backups
	c.printf("%s %s\n", ok("✓"), dim("applied · "+summary))
	return out, nil
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
			base := c.lib.SkillsDir()
			if private {
				base = c.lib.PrivateDir()
			}
			dir := filepath.Join(base, args[0])
			if fsutil.Lexists(dir) {
				return fmt.Errorf("%s already exists", dir)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			body := fmt.Sprintf("---\nname: %s\ndescription: What this skill does and when an agent should use it.\n---\n\n# %s\n\nInstructions for the agent go here.\n", args[0], args[0])
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
				return err
			}
			c.printf("%s created %s\n", ok("✓"), dir)
			out, err := doApply(c, false, false, nil)
			if err != nil {
				return err
			}
			c.jsonOut(map[string]any{"created": args[0], "path": dir, "apply": out})
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
			items, err := engine.ListTrash(c.lib)
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
			for _, it := range items {
				t.Row(it.ID, dim(strings.Replace(it.Origin, home, "~", 1)), dim(it.When.Format("2006-01-02 15:04")))
			}
			c.printf("%s\n%s\n", t.Render(), dim("skd trash restore <id>   ·   skd trash empty"))
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
			dest, err := engine.Restore(c.lib, args[0])
			if err != nil {
				return err
			}
			c.printf("%s restored to %s\n", ok("✓"), dest)
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
			items, _ := engine.ListTrash(c.lib)
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
			n, err := engine.EmptyTrash(c.lib)
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
