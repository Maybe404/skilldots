package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/targets"
)

const gitignore = `.DS_Store
# Machine-local skills: a separate git repository that is never pushed with this one.
/private/
`

func initCmd() *cobra.Command {
	var from string
	var defaults []string
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
			} else if b, _ := os.ReadFile(gi); !strings.Contains(string(b), "/private/") {
				os.WriteFile(gi, append(b, []byte("\n/private/\n")...), 0o644)
			}
			if err := library.EnsureGit(root); err != nil {
				return err
			}
			priv := filepath.Join(root, library.DirPrivate)
			if err := library.EnsureGit(priv); err != nil {
				return err
			}
			fmt.Printf("initialised library at %s\n  default targets: %s\n", root, strings.Join(defaults, ", "))
			if root != library.DefaultRoot() {
				fmt.Printf("  export %s=%s   # or run skd from inside it\n", library.EnvLibrary, root)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "turn an existing directory into the library")
	cmd.Flags().StringSliceVar(&defaults, "targets", nil, "default targets (auto-detected when omitted)")
	return cmd
}

// detectDefaults picks "agents" plus whichever well-known agents are installed.
func detectDefaults() []string {
	reg, err := targets.NewRegistry(nil)
	if err != nil {
		return []string{"agents"}
	}
	out := []string{"agents"}
	for _, t := range reg.Present() {
		if t.Name != "agents" && (t.Name == "claude" || t.Name == "codex" || t.Name == "gemini" || t.Name == "cursor" || t.Name == "opencode") {
			out = append(out, t.Name)
		}
	}
	return out
}

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
				for _, e := range entries {
					printEntry(c, e)
				}
				return nil
			}
			m := c.lib.Manifest
			c.printf("library %s\n", c.lib.Root)
			for _, p := range m.Packs {
				printPack(c, p, entries, all)
			}
			printGroup(c, entries, engine.KindSkill, "skills/ (yours)")
			printGroup(c, entries, engine.KindPrivate, "private/ (this machine)")
			printGroup(c, entries, engine.KindApp, "apps")
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "expand every pack")
	cmd.Flags().BoolVar(&flat, "flat", false, "one line per skill")
	cmd.Flags().StringVar(&agent, "agent", "", "show what one agent target receives")
	return cmd
}

func printPack(c *ctx, p *manifest.Pack, entries []engine.Entry, expand bool) {
	lp := c.lib.Lock.Packs[p.Repo]
	total, enabled := 0, 0
	var mine []engine.Entry
	if lp != nil {
		total = len(lp.Skills)
	}
	for _, e := range entries {
		if e.Kind == engine.KindVendor && e.Origin == p.Repo {
			mine = append(mine, e)
			if len(e.Targets) > 0 {
				enabled++
			}
		}
	}
	commit := "—"
	if lp != nil && lp.Commit != "" {
		commit = lp.Commit[:7]
	}
	label := p.Repo
	if p.Alias != "" {
		label += " (" + p.Alias + ")"
	}
	sel := fmt.Sprintf("%d/%d", len(mine), total)
	if p.UsesAll() {
		sel = fmt.Sprintf("all %d", total)
	}
	c.printf("%-40s %-10s %s\n", label, sel, commit)
	if expand {
		for _, e := range mine {
			printEntry(c, e)
		}
	}
}

func printGroup(c *ctx, entries []engine.Entry, kind engine.Kind, title string) {
	var names []string
	for _, e := range entries {
		if e.Kind == kind {
			names = append(names, e.LinkName+targetSuffix(e))
		}
	}
	if len(names) == 0 {
		return
	}
	c.printf("%-40s %s\n", title, strings.Join(names, ", "))
}

func printEntry(c *ctx, e engine.Entry) {
	name := e.LinkName
	if e.LinkName != e.Name {
		name += " (" + e.Name + ")"
	}
	cat := e.Category
	if cat == "" {
		cat = "·"
	}
	status := ""
	if e.Missing {
		status = "  ⚠ missing"
	}
	c.printf("  %-30s %-14s → %s%s\n", name, cat, targetList(e), status)
}

func targetSuffix(e engine.Entry) string {
	if len(e.Targets) == 0 {
		return " (off)"
	}
	return ""
}

func targetList(e engine.Entry) string {
	if len(e.Targets) == 0 {
		return "(off)"
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
	c.printf("%s  %s\n", t.Name, t.Path)
	for _, e := range entries {
		for _, et := range e.Targets {
			if et.Name == t.Name {
				origin := string(e.Kind)
				if e.Origin != "" {
					origin = e.Origin
				}
				c.printf("  %-30s %s\n", e.LinkName, origin)
			}
		}
	}
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
	cmd.Flags().BoolVar(&force, "force", false, "replace unmanaged or modified entries")
	return cmd
}

func runApply(c *ctx, dry, force bool) error {
	entries, err := c.resolve()
	if err != nil {
		return err
	}
	plan := engine.MakePlan(c.lib, entries, force)
	if c.jsonOut(plan.Ops) {
		return nil
	}
	for _, op := range plan.Ops {
		if op.Action == "keep" {
			continue
		}
		mark := map[string]string{"create": "+", "replace": "↻", "remove": "-", "skip": "!"}[op.Action]
		line := fmt.Sprintf("%s %-10s %s", mark, op.Target.Name, op.Name)
		if op.Reason != "" {
			line += "   (" + op.Reason + ")"
		}
		c.printf("%s\n", line)
	}
	counts := plan.Counts()
	if !plan.Changes() {
		c.printf("up to date (%d linked)\n", counts["keep"])
		return nil
	}
	if dry {
		c.printf("dry run: +%d ↻%d -%d, %d kept, %d skipped\n", counts["create"], counts["replace"], counts["remove"], counts["keep"], counts["skip"])
		return nil
	}
	if err := engine.Apply(c.lib, plan); err != nil {
		return err
	}
	if err := c.lib.Save(); err != nil {
		return err
	}
	c.printf("applied: +%d ↻%d -%d, %d kept, %d skipped\n", counts["create"], counts["replace"], counts["remove"], counts["keep"], counts["skip"])
	return nil
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
			for _, r := range rows {
				flag := " "
				if r.Present {
					flag = "●"
				}
				d := ""
				if r.Default {
					d = "default"
				}
				c.printf("%s %-12s %-8s %s\n", flag, r.Name, d, r.Path)
			}
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
			c.printf("created %s\n", dir)
			return runApply(c, false, false)
		},
	}
	cmd.Flags().BoolVar(&private, "private", false, "create under private/ (never pushed)")
	return cmd
}
