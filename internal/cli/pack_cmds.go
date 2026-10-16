package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/pack"
)

func addCmd() *cobra.Command {
	var skills, categories, targetNames []string
	var all bool
	var alias string
	cmd := &cobra.Command{
		Use:   "add <owner/repo | git url>[@ref]",
		Short: "Subscribe to a repository and pick the skills to vendor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			src, err := c.ParseAdd(args[0], alias)
			if err != nil {
				return err
			}
			c.printf("%s %s\n", dim("fetching"), src.Repo)
			f, err := c.Fetch(src)
			if err != nil {
				return err
			}
			printRejected(c, src.Repo, f.Rejected)
			if len(f.Skills) == 0 {
				return fmt.Errorf("%s contains no SKILL.md", src.Repo)
			}
			sel := app.Selection{Skills: skills, Categories: categories, All: all, Alias: alias, Agents: targetNames}
			if !all && len(skills) == 0 && len(categories) == 0 {
				if c.jsonOut(f.Skills) {
					return nil
				}
				chosen, err := pickSkills(c, src.Repo, f)
				if err != nil {
					return err
				}
				if len(chosen) == 1 && chosen[0] == "*" {
					sel.All = true
				} else {
					sel.Skills = chosen
				}
			}
			out, err := c.Add(f, sel)
			if err != nil {
				return err
			}
			if out.Nothing {
				c.printf("%s\n", dim("nothing selected"))
				return nil
			}
			rep := out.Report
			printRenamed(c, rep)
			warnLicense(c, rep)
			c.printf("%s vendored %s from %s %s\n", ok("✓"), plural(len(rep.Added)+len(rep.Changed)+len(rep.Unchanged), "skill"), out.Repo, dim("@ "+short(rep.NewCommit)))
			printApply(c, out.Apply)
			c.jsonOut(map[string]any{"repo": out.Repo, "commit": rep.NewCommit, "skills": out.Linked,
				"added": nonNil(rep.Added), "changed": nonNil(rep.Changed), "unchanged": nonNil(rep.Unchanged),
				"noLicense": nonNil(rep.NoLicenseList), "renamed": rep.Renamed, "rejected": nonNil(rep.Rejected),
				"changes": out.Apply.Changes, "kept": out.Apply.Kept, "backups": out.Apply.Backups})
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&skills, "skill", "s", nil, "skill names to select (repeatable)")
	cmd.Flags().StringSliceVar(&categories, "category", nil, "select every skill in a category")
	cmd.Flags().BoolVar(&all, "all", false, "follow every skill, including ones added upstream later")
	cmd.Flags().StringVar(&alias, "alias", "", "short name for the pack")
	cmd.Flags().StringSliceVar(&targetNames, "agent", nil, "targets for this pack (default: manifest default)")
	return cmd
}

// pickSkills shows an interactive multi-select grouped by category.
// It returns ["*"] when the user chose to follow everything.
func pickSkills(c *ctx, repo string, f *app.Fetched) ([]string, error) {
	upstream := f.Skills
	if !isTerminal() {
		return nil, fmt.Errorf("no terminal: pass --skill, --category or --all")
	}
	var opts []huh.Option[string]
	var preselected []string
	for _, cat := range pack.Categories(upstream) {
		for _, s := range upstream {
			if s.Category != cat {
				continue
			}
			label := s.Name
			if cat != "" {
				label = dim(cat+"/") + s.Name
			}
			if s.Description != "" {
				label += "  " + dim(truncate(s.Description, 60))
			}
			opts = append(opts, huh.NewOption(label, s.Name))
			if f.Have[s.Name] {
				preselected = append(preselected, s.Name)
			}
		}
	}
	mode := "pick"
	chosen := preselected
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("%s — %d skills", repo, len(upstream))).
				Options(
					huh.NewOption("Pick skills", "pick"),
					huh.NewOption("Follow everything (new upstream skills are added automatically)", "all"),
				).
				Value(&mode),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select skills").
				Description("space to toggle · type to filter · enter to confirm").
				Options(opts...).
				Filterable(true).
				Height(18).
				Value(&chosen),
		).WithHideFunc(func() bool { return mode == "all" }),
	)
	form = form.WithTheme(theme())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, nil
		}
		return nil, err
	}
	if mode == "all" {
		return []string{"*"}, nil
	}
	return dedupe(chosen), nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// printRejected warns about upstream SKILL.md files that were not used.
func printRejected(c *ctx, repo string, rejected []pack.Rejected) {
	for _, r := range rejected {
		fmt.Fprintf(os.Stderr, "%s %s: %s\n", warn("!"), repo, r)
	}
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// printRenamed tells about skills linked under another name because the
// upstream name was already taken in the library.
func printRenamed(c *ctx, rep *engine.SyncReport) {
	for _, n := range sortedKeys(rep.Renamed) {
		fmt.Fprintf(os.Stderr, "%s %s: %q is already used in the library; linking it as %q (as in skills.toml)\n", warn("!"), rep.Repo, n, rep.Renamed[n])
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func warnLicense(c *ctx, rep *engine.SyncReport) {
	for _, w := range rep.Warnings {
		fmt.Fprintf(os.Stderr, "%s %s: %s\n", warn("!"), rep.Repo, w)
	}
	if rep.NoLicense {
		c.printf("%s %s: no license found for %s — check before redistributing\n", warn("!"), rep.Repo, strings.Join(rep.NoLicenseList, ", "))
	}
}

func updateCmd() *cobra.Command {
	var commit bool
	cmd := &cobra.Command{
		Use:   "update [pack...]",
		Short: "Fetch upstream changes for packs and re-vendor selected skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			// Whatever succeeded is saved and linked even if another pack failed.
			out, err := c.Update(args, func(repo string, rep *engine.SyncReport, err error) {
				if err != nil {
					c.printf("%s %s: %v\n", bad("✗"), repo, err)
					return
				}
				printReport(c, rep)
				printRenamed(c, rep)
				warnLicense(c, rep)
			})
			if out != nil && out.Apply != nil {
				printApply(c, out.Apply)
			}
			if err != nil {
				return err
			}
			committed := false
			if commit && out.Changed {
				if committed, err = c.Commit("chore(skills): update vendored skills", true); committed {
					c.printf("%s committed in %s\n", ok("✓"), c.lib.Root)
				}
				if err != nil {
					return err
				}
			}
			c.jsonOut(map[string]any{"packs": out.Reports, "failed": out.Failed, "apply": out.Apply, "committed": committed})
			if len(out.Failed) > 0 {
				return fmt.Errorf("%d pack(s) failed: %s", len(out.Failed), out.Failed[0].Repo)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&commit, "commit", false, "git commit and push the library when something changed")
	return cmd
}

func printReport(c *ctx, r *engine.SyncReport) {
	head := bold(r.Repo)
	switch {
	case r.OldCommit == "":
		head += dim(fmt.Sprintf("  new @ %s", short(r.NewCommit)))
	case r.OldCommit == r.NewCommit:
		head += dim(fmt.Sprintf("  %s  up to date", short(r.NewCommit)))
	default:
		head += fmt.Sprintf("  %s → %s", dim(short(r.OldCommit)), accent(short(r.NewCommit)))
	}
	c.printf("%s\n", head)
	row := func(label string, style func(string) string, names []string) {
		if len(names) > 0 {
			c.printf("  %s %s\n", style(fmt.Sprintf("%-13s", label)), strings.Join(names, ", "))
		}
	}
	row("changed", accent, r.Changed)
	row("added", ok, r.Added)
	row("dropped", bad, r.Dropped)
	row("moved", dim, r.Moved)
	if n := len(r.UpstreamNew); n > 0 {
		shown := r.UpstreamNew
		more := ""
		if n > 5 {
			shown, more = shown[:5], dim(fmt.Sprintf(" … %d more (skd add %s to browse)", n-5, r.Repo))
		}
		c.printf("  %s %s%s\n", dim(fmt.Sprintf("%-13s", "upstream new")), strings.Join(shown, ", "), more)
	}
	row("upstream gone", warn, r.UpstreamGone)
	for _, rj := range r.Rejected {
		c.printf("  %s %s\n", warn(fmt.Sprintf("%-13s", "rejected")), rj)
	}
	for _, b := range r.Backups {
		c.printf("  %s %s\n", dim(fmt.Sprintf("%-13s", "moved aside")), b)
	}
	if len(r.Changed) > 0 {
		c.printf("  %s\n", dim(fmt.Sprintf("see changes: git -C %s diff -- vendor/%s", c.lib.Root, r.Repo)))
	}
}

func short(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func enableCmd() *cobra.Command {
	var agents []string
	cmd := &cobra.Command{
		Use:   "enable <skill|pack>",
		Short: "Link a skill (or whole pack) into agents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			return targetsDone(c)(c.Enable(app.KeyRef(args[0]), agents))
		},
	}
	cmd.Flags().StringSliceVar(&agents, "agent", nil, "targets to add (default: manifest default)")
	return cmd
}

func disableCmd() *cobra.Command {
	var agents []string
	cmd := &cobra.Command{
		Use:   "disable <skill|pack>",
		Short: "Unlink a skill (or pack) from agents; the library keeps it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			return targetsDone(c)(c.Disable(app.KeyRef(args[0]), agents))
		},
	}
	cmd.Flags().StringSliceVar(&agents, "agent", nil, "targets to remove (default: all)")
	return cmd
}

// targetsDone reports a targets edit like `skd apply`.
func targetsDone(c *ctx) func(*app.TargetsOutcome, error) error {
	return func(out *app.TargetsOutcome, err error) error {
		if err != nil {
			return err
		}
		printApply(c, out.Apply)
		c.jsonOut(out.Apply)
		return nil
	}
}

func removeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <skill>",
		Short: "Deselect a vendored skill: unlink everywhere and drop it from vendor/",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			out, err := c.Remove(app.KeyRef(args[0]))
			if err != nil {
				return err
			}
			if out.Purged != nil {
				printPurge(c, out.Purged.Repo, out.Purged.Unlinked)
				printApply(c, out.Purged.Apply)
				c.jsonOut(map[string]any{"removed": out.Entry.LinkName, "purged": out.Purged})
				return nil
			}
			printApply(c, out.Apply)
			c.jsonOut(map[string]any{"removed": out.Entry.LinkName, "apply": out.Apply})
			return nil
		},
	}
	return cmd
}

func purgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "purge <pack>",
		Short: "Delete a pack entirely: manifest, lock, vendor/ and every link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			p := c.lib.Manifest.FindPack(args[0])
			if p == nil {
				return fmt.Errorf("no pack %q", args[0])
			}
			printPurge(c, p.Repo, c.PurgeLinks(p.Repo))
			if !confirm("proceed?") {
				return fmt.Errorf("cancelled")
			}
			out, err := c.Purge(p.Repo)
			if err != nil {
				return err
			}
			printApply(c, out.Apply)
			c.jsonOut(out)
			return nil
		},
	}
	return cmd
}

// printPurge lists what purging repo removes.
func printPurge(c *ctx, repo string, links []string) {
	c.printf("%s %s: unlink %s and move vendor/%s to the trash\n", warn("purge"), bold(repo), plural(len(links), "link"), repo)
	for _, l := range links {
		c.printf("  %s %s\n", bad("−"), l)
	}
}

func pathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path <skill>",
		Short: "Print where a skill's files live in the library",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			entries, err := c.resolve()
			if err != nil {
				return err
			}
			e, err := lookup(c, entries, args[0])
			if err != nil {
				return err
			}
			if e == nil {
				return fmt.Errorf("no skill %q", args[0])
			}
			if !c.jsonOut(map[string]any{"name": e.LinkName, "path": e.Dir}) {
				fmt.Println(e.Dir)
			}
			return nil
		},
	}
}

func forkCmd() *cobra.Command {
	var private bool
	var as string
	cmd := &cobra.Command{
		Use:   "fork <skill>",
		Short: "Copy a vendored skill into skills/ so you can edit it; upstream no longer touches it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			out, err := c.Fork(app.KeyRef(args[0]), private, as)
			if err != nil {
				return err
			}
			c.printf("%s forked %s → %s\n", ok("✓"), args[0], out.Path)
			printApply(c, out.Apply)
			c.jsonOut(map[string]any{"forked": args[0], "name": out.Name, "path": out.Path, "apply": out.Apply})
			return nil
		},
	}
	cmd.Flags().BoolVar(&private, "private", false, "copy into private/ instead of skills/")
	cmd.Flags().StringVar(&as, "as", "", "name for the fork (default: same name)")
	return cmd
}

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show <owner/repo | git url>[@ref]",
		Aliases: []string{"inspect", "browse"},
		Short:   "List the skills a repository offers, without installing anything",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			arg := args[0]
			if existing := c.lib.Manifest.FindPack(arg); existing != nil {
				arg = existing.Repo
			}
			src, err := pack.ParseSource(arg)
			if err != nil {
				return err
			}
			clone, commit, err := pack.Fetch(src)
			if err != nil {
				return err
			}
			upstream, rejected, err := pack.Scan(clone)
			if err != nil {
				return err
			}
			printRejected(c, src.Repo, rejected)
			p := c.lib.Manifest.FindPack(src.Repo)
			type row struct {
				Name, Category, Description string
				Installed                   bool
				// Dir is the copy that would be vendored; Variants are
				// same-named directories with different content.
				Dir      string
				Variants []string `json:",omitempty"`
			}
			rel := func(d string) string {
				r, _ := filepath.Rel(clone, d)
				return filepath.ToSlash(r)
			}
			var rows []row
			for _, s := range upstream {
				var variants []string
				for _, v := range s.Variants {
					variants = append(variants, rel(v))
				}
				rows = append(rows, row{s.Name, s.Category, s.Description, p != nil && p.Selected(s.Name, s.Category), rel(s.Dir), variants})
			}
			if c.jsonOut(map[string]any{"repo": src.Repo, "commit": commit, "subscribed": p != nil, "skills": rows, "rejected": nonNil(rejected)}) {
				return nil
			}
			c.printf("%s %s  %s\n", bold(src.Repo), dim("@ "+short(commit)), dim(plural(len(rows), "skill")))
			t := newTable("", "SKILL", "CATEGORY", "DESCRIPTION")
			for _, r := range rows {
				mark := dim("·")
				if r.Installed {
					mark = ok("✓")
				}
				cat := r.Category
				if cat == "" {
					cat = dim("·")
				}
				t.Row(mark, r.Name, cat, dim(truncate(r.Description, 60)))
			}
			c.printf("%s\n", t.Render())
			for _, r := range rows {
				if len(r.Variants) > 0 {
					c.printf("%s\n", dim(fmt.Sprintf("%s: uses %s; %d other copies with different content are ignored (%s)",
						r.Name, r.Dir, len(r.Variants), truncate(strings.Join(r.Variants, ", "), 80))))
				}
			}
			c.printf("%s\n", dim("install: skd add "+src.Repo+" --skill <name>   or   --category <category>   or   --all"))
			return nil
		},
	}
}

// lookup resolves a user-supplied skill key; see app.Session.Lookup.
func lookup(c *ctx, entries []engine.Entry, key string) (*engine.Entry, error) {
	return c.Lookup(entries, key)
}
