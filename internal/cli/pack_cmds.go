package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/pack"
)

func addCmd() *cobra.Command {
	var skills, categories, targetNames []string
	var all bool
	var alias string
	cmd := &cobra.Command{
		Use:   "add <owner/repo | url | pasted install command>[@ref]",
		Short: i18n.T("Subscribe to a repository and pick the skills to vendor"),
		Long: i18n.T("The source is owner/repo, a GitHub URL or any git URL (@ref pins a branch or tag), a skills.sh skill page, " +
			"a GitHub link to a skill directory (/tree/<ref>/<dir>) or its SKILL.md (/blob/<ref>/…/SKILL.md), " +
			"or an install command pasted whole, quoted: `npx skills add owner/repo --skill name` (also bunx, pnpm dlx, " +
			"npx add-skill). The skills a link or --skill names are preselected; the text is only read, never run. " +
			"The ref in a GitHub link is ignored: append @ref to pin one. Without a source at a terminal, skd asks for one."),
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := app.AddArgs(args)
			if err != nil {
				return err
			}
			c, err := open()
			if err != nil {
				return err
			}
			if text == "" {
				if !isTerminal() || flags.json {
					return i18n.Errorf("give a source: skd add owner/repo (or a URL, or a pasted `npx skills add …`)")
				}
				if err := ask(huh.NewInput().Title(i18n.T("Source")).
					Description(i18n.T("owner/repo · a GitHub or skills.sh link · a pasted `npx skills add …` line · any git URL · @ref pins")).
					Placeholder(i18n.T("mattpocock/skills")).Value(&text)); err != nil || strings.TrimSpace(text) == "" {
					return nil
				}
			}
			src, hint, err := c.ParseAddText(text, alias)
			if err != nil {
				return err
			}
			c.printf("%s %s\n", dim(i18n.T("fetching")), src.Repo)
			f, err := c.Fetch(src)
			if err != nil {
				return err
			}
			printRejected(c, src.Repo, f.Rejected)
			if len(f.Skills) == 0 {
				return i18n.Errorf("%s contains no SKILL.md", src.Repo)
			}
			pre, missing := f.Preselect(hint)
			for _, m := range missing {
				i18n.Fprintf(stderr, "%s %s: nothing named %q upstream\n", warn("!"), src.Repo, m)
			}
			sel := app.Selection{Skills: skills, Categories: categories, All: all, Alias: alias, Agents: targetNames}
			switch {
			case all || len(skills) > 0 || len(categories) > 0:
				sel.Skills = dedupe(append(sel.Skills, pre...))
			case len(pre) > 0 && (!isTerminal() || flags.json || flags.yes):
				// No one to ask: what the pasted text names is the choice.
				sel.Skills = pre
			case len(missing) > 0 && (!isTerminal() || flags.json || flags.yes):
				return i18n.Errorf("no skill %q in %s (skd show %s lists them)", missing[0], src.Repo, src.Repo)
			default:
				if c.jsonOut(f.Skills) {
					return nil
				}
				chosen, err := pickSkills(c, src.Repo, f, pre)
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
				c.printf("%s\n", dim(i18n.T("nothing selected")))
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
				"changes": out.Apply.Changes, "kept": out.Apply.Kept, "backups": out.Apply.Backups, "refused": out.Apply.Refused})
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&skills, "skill", "s", nil, i18n.T("skill names to select (repeatable)"))
	cmd.Flags().StringSliceVar(&categories, "category", nil, i18n.T("select every skill in a category"))
	cmd.Flags().BoolVar(&all, "all", false, i18n.T("follow every skill, including ones added upstream later"))
	cmd.Flags().StringVar(&alias, "alias", "", i18n.T("short name for the pack"))
	cmd.Flags().StringSliceVar(&targetNames, "agent", nil, i18n.T("targets for this pack (default: manifest default)"))
	return cmd
}

// pickSkills shows an interactive multi-select grouped by category.
// It returns ["*"] when the user chose to follow everything.
func pickSkills(c *ctx, repo string, f *app.Fetched, pre []string) ([]string, error) {
	upstream := f.Skills
	if !isTerminal() {
		return nil, i18n.Errorf("no terminal: pass --skill, --category or --all")
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
			if f.Have[s.Name] || slices.Contains(pre, s.Name) {
				preselected = append(preselected, s.Name)
			}
		}
	}
	mode := "pick"
	chosen := preselected
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(i18n.Sprintf("%s — %d skills", repo, len(upstream))).
				Options(
					huh.NewOption(i18n.T("Pick skills"), "pick"),
					huh.NewOption(i18n.T("Follow everything (new upstream skills are added automatically)"), "all"),
				).
				Value(&mode),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(i18n.T("Select skills")).
				Description(i18n.T("space to toggle · type to filter · enter to confirm")).
				Options(opts...).
				Filterable(true).
				Height(18).
				Value(&chosen),
		).WithHideFunc(func() bool { return mode == "all" }),
	)
	form = form.WithTheme(theme()).WithKeyMap(keyMap())
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
		fmt.Fprintf(stderr, "%s %s: %s\n", warn("!"), repo, r)
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
		i18n.Fprintf(stderr, "%s %s: %q is already used in the library; linking it as %q (as in skills.toml)\n", warn("!"), rep.Repo, n, rep.Renamed[n])
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
		fmt.Fprintf(stderr, "%s %s: %s\n", warn("!"), rep.Repo, txt(w))
	}
	if rep.NoLicense {
		c.printf("%s %s: no license found for %s — check before redistributing\n", warn("!"), rep.Repo, strings.Join(rep.NoLicenseList, ", "))
	}
}

func updateCmd() *cobra.Command {
	var commit bool
	cmd := &cobra.Command{
		Use:   "update [pack...]",
		Short: i18n.T("Fetch upstream changes for packs and re-vendor selected skills"),
		Long: i18n.T("At a terminal, update first checks upstream and lists what would change per pack (changed, new, gone, moved), " +
			"all ticked; untick what you do not want, and only the ticked packs are updated. With --yes, --json or without a " +
			"terminal it updates straight away (every pack when none are named), as scripts and agents expect. " +
			"Every update is recorded: skd history."),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			var checkErr error
			if isTerminal() && !flags.yes && !flags.json {
				chosen, failed, err := confirmUpdate(c, args)
				if err != nil {
					return err
				}
				if len(chosen) == 0 {
					return failed
				}
				args, checkErr = chosen, failed
			}
			// Whatever succeeded is saved and linked even if another pack failed.
			out, err := c.Update(args, func(repo string, rep *engine.SyncReport, err error) {
				if err != nil {
					c.printf("%s %s: %s\n", bad("✗"), repo, errText(err))
					return
				}
				printReport(c, rep)
				printRenamed(c, rep)
				warnLicense(c, rep)
			})
			if out != nil && out.Apply != nil {
				printApply(c, out.Apply)
			}
			committed := false
			// Past this point the update may have happened: under --json a
			// failure still reports it, with the error.
			report := func(err error) error {
				if out != nil {
					res := map[string]any{"packs": out.Reports, "failed": out.Failed, "apply": out.Apply, "committed": committed, "history": out.History}
					if err != nil {
						res["error"] = txt(err.Error())
					}
					c.jsonOut(res)
				}
				return err
			}
			if err != nil {
				return report(err)
			}
			if commit && out.Changed {
				if committed, err = c.Commit("chore(skills): update vendored skills", true); committed {
					c.printf("%s committed in %s\n", ok("✓"), c.lib.Root)
				}
				if err != nil {
					return report(err)
				}
			}
			if len(out.History) > 0 {
				c.printf("%s\n", dim(i18n.Sprintf("recorded in %s · skd history shows what changed and why", app.HistoryFile)))
			}
			report(nil)
			if len(out.Failed) > 0 {
				return i18n.Errorf("%d pack(s) failed: %s", len(out.Failed), out.Failed[0].Repo)
			}
			return checkErr
		},
	}
	cmd.Flags().BoolVar(&commit, "commit", false, i18n.T("git commit and push the library when something changed"))
	return cmd
}

// confirmUpdate is `skd update` at a terminal: check upstream, show what
// would change and let the person pick the packs. It returns the chosen
// packs, none when there is nothing to do or the person chose nothing,
// and failed, an error naming the packs that could not be checked.
func confirmUpdate(c *ctx, repos []string) (chosen []string, failed, err error) {
	c.printf("%s\n", dim(i18n.T("checking upstream…")))
	plan, err := c.PlanUpdate(context.Background(), repos, func(repo string, st *app.PackStatus, err error) {
		if err != nil {
			c.printf("%s %s: %s\n", bad("✗"), repo, errText(err))
		}
	})
	if err != nil {
		return nil, nil, err
	}
	if len(plan.Failed) > 0 {
		failed = i18n.Errorf("%d pack(s) could not be checked: %s", len(plan.Failed), plan.Failed[0].Repo)
	}
	if len(plan.Updates) == 0 {
		// Packs named and checked: offer to update them anyway, as u in
		// the dashboard does. That re-vendors skills missing from vendor/
		// and re-links; --yes does it without asking.
		var checked []string
		for _, st := range plan.Packs {
			checked = append(checked, st.Repo)
		}
		if len(repos) == 0 || len(checked) == 0 {
			c.printf("%s %s\n", ok(i18n.T("✓ nothing to update upstream")), dim(i18n.T("· skd apply re-links · skd update <pack> re-vendors one anyway")))
			return nil, failed, nil
		}
		c.printf("%s\n", ok(i18n.Sprintf("✓ nothing changed upstream for %s", strings.Join(checked, ", "))))
		yes := false
		err := ask(huh.NewConfirm().
			Title(i18n.T("Update anyway?")).
			Description(i18n.T("re-vendors skills missing from vendor/ and re-links them")).
			Affirmative(i18n.T("Update anyway")).Negative(i18n.T("No")).Value(&yes))
		if errors.Is(err, huh.ErrUserAborted) || (err == nil && !yes) {
			c.printf("%s\n", dim(i18n.T("nothing updated")))
			return nil, failed, nil
		}
		if err != nil {
			return nil, nil, err
		}
		return checked, failed, nil
	}
	var opts []huh.Option[string]
	for _, st := range plan.Updates {
		c.printf("\n")
		printStatus(c, &st)
		opts = append(opts, huh.NewOption(st.Repo+"  "+dim(st.Summary()), st.Repo).Selected(true))
		chosen = append(chosen, st.Repo)
	}
	c.printf("\n")
	err = ask(huh.NewMultiSelect[string]().
		Title(i18n.Sprintf("Update %s?", plural(len(plan.Updates), "pack"))).
		Description(i18n.T("space or x unticks · enter updates the ticked ones · ctrl+c cancels")).
		Options(opts...).Value(&chosen))
	if errors.Is(err, huh.ErrUserAborted) || (err == nil && len(chosen) == 0) {
		c.printf("%s\n", dim(i18n.T("nothing updated")))
		return nil, failed, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return chosen, failed, nil
}

func printReport(c *ctx, r *engine.SyncReport) {
	head := bold(r.Repo)
	switch {
	case r.OldCommit == "":
		head += dim(i18n.Sprintf("  new @ %s", short(r.NewCommit)))
	case r.OldCommit == r.NewCommit:
		head += dim(i18n.Sprintf("  %s  up to date", short(r.NewCommit)))
	default:
		head += fmt.Sprintf("  %s → %s", dim(short(r.OldCommit)), accent(short(r.NewCommit)))
	}
	c.printf("%s\n", head)
	row := func(label string, style func(string) string, names []string) {
		if len(names) > 0 {
			c.printf("  %s %s\n", style(pad(label, 13)), strings.Join(txts(names), ", "))
		}
	}
	row(i18n.T("changed"), accent, r.Changed)
	row(i18n.T("added"), ok, r.Added)
	row(i18n.T("dropped"), bad, r.Dropped)
	row(i18n.T("moved"), dim, r.Moved)
	if n := len(r.UpstreamNew); n > 0 {
		shown := r.UpstreamNew
		more := ""
		if n > 5 {
			shown, more = shown[:5], dim(i18n.Sprintf(" … %d more (skd add %s to browse)", n-5, r.Repo))
		}
		c.printf("  %s %s%s\n", dim(pad(i18n.T("upstream new"), 13)), strings.Join(txts(shown), ", "), more)
	}
	row(i18n.T("upstream gone"), warn, r.UpstreamGone)
	for _, rj := range r.Rejected {
		c.printf("  %s %s\n", warn(pad(i18n.T("rejected"), 13)), rj)
	}
	for _, b := range r.Backups {
		c.printf("  %s %s\n", dim(pad(i18n.T("moved aside"), 13)), txt(b))
	}
	if len(r.Changed) > 0 {
		c.printf("  %s\n", dim(i18n.Sprintf("see changes: git -C %s diff -- vendor/%s", c.lib.Root, r.Repo)))
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
		Short: i18n.T("Link a skill (or whole pack) into agents"),
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			return targetsDone(c)(c.Enable(app.KeyRef(args[0]), agents))
		},
	}
	cmd.Flags().StringSliceVar(&agents, "agent", nil, i18n.T("targets to add (default: manifest default)"))
	return cmd
}

func disableCmd() *cobra.Command {
	var agents []string
	cmd := &cobra.Command{
		Use:   "disable <skill|pack>",
		Short: i18n.T("Unlink a skill (or pack) from agents; the library keeps it"),
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			return targetsDone(c)(c.Disable(app.KeyRef(args[0]), agents))
		},
	}
	cmd.Flags().StringSliceVar(&agents, "agent", nil, i18n.T("targets to remove (default: all)"))
	return cmd
}

// targetsDone reports a targets edit like `skd apply`.
func targetsDone(c *ctx) func(*app.TargetsOutcome, error) error {
	return func(out *app.TargetsOutcome, err error) error {
		if err != nil {
			return err
		}
		for _, a := range out.NotThere {
			if _, err := c.reg.Get(a); err != nil {
				c.printf("%s %s\n", warn("!"), i18n.Sprintf("%s: %v; nothing to take off", txt(a), err))
				continue
			}
			c.printf("%s %s\n", warn("!"), i18n.Sprintf("%s was not installed for %s; nothing to take off", txt(out.Label), txt(a)))
		}
		for _, a := range out.AlreadyThere {
			c.printf("%s %s\n", dim("·"), i18n.Sprintf("%s was already installed for %s", txt(out.Label), txt(a)))
		}
		printApply(c, out.Apply)
		c.jsonOut(out.Apply)
		return nil
	}
}

func removeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <skill>",
		Short: i18n.T("Move a library skill to the trash and unlink it from every agent"),
		Args:  exactArgs(1),
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
				// The apply lists the links taken out; the pack goes with
				// its last skill.
				printApply(c, out.Purged.Apply)
				c.printf("%s %s\n", ok("✓"), i18n.Sprintf("removed %s, the last skill of %s: the pack is dropped from skills.toml too", txt(out.Entry.LinkName), txt(out.Purged.Repo)))
				printUndo(c, out.Trash)
				c.jsonOut(map[string]any{"removed": out.Entry.LinkName, "purged": out.Purged, "trash": out.Trash})
				return nil
			}
			printApply(c, out.Apply)
			c.printf("%s %s\n", ok("✓"), i18n.Sprintf("removed %s", txt(out.Entry.LinkName)))
			printUndo(c, out.Trash)
			c.jsonOut(map[string]any{"removed": out.Entry.LinkName, "apply": out.Apply, "trash": out.Trash})
			return nil
		},
	}
	return cmd
}

func purgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "purge <pack>",
		Short: i18n.T("Delete a pack entirely: manifest, lock, vendor/ and every link"),
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			p := c.lib.Manifest.FindPack(args[0])
			if p == nil {
				return i18n.Errorf("no pack %q", args[0])
			}
			printPurge(c, p.Repo, c.PurgeLinks(p.Repo))
			if !confirm("proceed?") {
				return i18n.Errorf("cancelled")
			}
			out, err := c.Purge(p.Repo)
			if err != nil {
				return err
			}
			printApply(c, out.Apply)
			printUndo(c, out.Trash)
			c.jsonOut(out)
			return nil
		},
	}
	return cmd
}

// printUndo says where in the trash the files went and how to put them
// back (id is "" when nothing was moved).
func printUndo(c *ctx, id string) {
	if id == "" {
		return
	}
	c.printf("  %s\n", dim(i18n.Sprintf("its files are in the library trash as %s; `skd trash restore %s` puts everything back", txt(id), txt(id))))
}

// printPurge lists what purging repo removes.
func printPurge(c *ctx, repo string, links []string) {
	c.printf("%s %s: unlink %s and move vendor/%s to the trash\n", warn(i18n.T("purge")), bold(repo), plural(len(links), "link"), repo)
	for _, l := range links {
		c.printf("  %s %s\n", bad("−"), l)
	}
}

func pathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path <skill>",
		Short: i18n.T("Print where a skill's files live in the library"),
		Args:  exactArgs(1),
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
				return i18n.Errorf("no skill %q", args[0])
			}
			if !c.jsonOut(map[string]any{"name": e.LinkName, "path": e.Dir}) {
				fmt.Fprintln(stdout, e.Dir)
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
		Short: i18n.T("Copy a vendored skill into skills/ so you can edit it; upstream no longer touches it"),
		Args:  exactArgs(1),
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
	cmd.Flags().BoolVar(&private, "private", false, i18n.T("copy into private/ instead of skills/"))
	cmd.Flags().StringVar(&as, "as", "", i18n.T("name for the fork (default: same name)"))
	return cmd
}

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show <owner/repo | git url>[@ref]",
		Aliases: []string{"inspect", "browse"},
		Short:   i18n.T("List the skills a repository offers, without installing anything"),
		Args:    exactArgs(1),
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
			unlock := pack.Hold(src.Repo)
			clone, commit, err := pack.Fetch(src)
			if err != nil {
				unlock()
				return err
			}
			upstream, rejected, err := pack.Scan(clone)
			unlock()
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
					c.printf("%s\n", dim(i18n.Sprintf("%s: uses %s; %d other copies with different content are ignored (%s)",
						r.Name, txt(r.Dir), len(r.Variants), truncate(strings.Join(txts(r.Variants), ", "), 80))))
				}
			}
			c.printf("%s\n", dim(i18n.Sprintf("install: skd add %s --skill <name>   or   --category <category>   or   --all", src.Repo)))
			return nil
		},
	}
}

// lookup resolves a user-supplied skill key; see app.Session.Lookup.
func lookup(c *ctx, entries []engine.Entry, key string) (*engine.Entry, error) {
	return c.Lookup(entries, key)
}

func outdatedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "outdated [pack...]",
		Short: i18n.T("Check upstream for changes to the selected skills, without changing anything"),
		Long: i18n.T("Fetches each pack into the cache and compares it with what is vendored: skills that changed, " +
			"appeared or disappeared upstream. vendor/, skills.lock and skills.toml are left alone; " +
			"`skd update` applies the changes. The result is remembered for `skd list` and the menu."),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			out, err := c.Outdated(args, func(repo string, st *app.PackStatus, err error) {
				if err != nil {
					c.printf("%s %s: %s\n", bad("✗"), repo, errText(err))
					return
				}
				printStatus(c, st)
			})
			if err != nil {
				return err
			}
			if !c.jsonOut(outdatedJSON(out)) {
				switch n := out.Updates(); {
				case n == 1:
					c.printf("\n%s · %s\n", accent(i18n.T("1 pack has updates")), dim(i18n.T("skd update")))
				case n > 1:
					c.printf("\n%s · %s\n", accent(i18n.Sprintf("%d packs have updates", n)), dim(i18n.T("skd update")))
				case len(out.Failed) == 0:
					c.printf("\n%s\n", ok(i18n.T("✓ everything is up to date")))
				}
			}
			if len(out.Failed) > 0 {
				return i18n.Errorf("%d pack(s) could not be checked: %s", len(out.Failed), out.Failed[0].Repo)
			}
			return nil
		},
	}
}

// outdatedJSON preserves the established CLI JSON contract. Desktop has a
// richer bridge contract, but adding its commit detail here would break
// agents that compare the CLI shape.
func outdatedJSON(out *app.Outdated) map[string]any {
	packs := []map[string]any{}
	for _, p := range out.Packs {
		packs = append(packs, map[string]any{
			"repo": p.Repo, "commit": p.Commit, "latest": p.Latest,
			"changed": p.Changed, "moved": p.Moved, "new": p.New,
			"gone": p.Gone, "pending": p.Pending, "checked": p.Checked,
		})
	}
	return map[string]any{"checked": out.Checked, "packs": packs, "failed": out.Failed, "updates": out.Updates()}
}

// printStatus is one pack of `skd outdated`: a block when it has updates,
// one line when not.
func printStatus(c *ctx, st *app.PackStatus) {
	head := bold(st.Repo)
	moved := func() {
		if len(st.Moved) > 0 {
			c.printf("  %s %s\n", dim(pad(i18n.T("moved"), 13)), dim(strings.Join(st.Moved, ", ")+i18n.T(" (path only)")))
		}
	}
	switch {
	case st.Commit == "":
		c.printf("%s  %s\n", head, warn(i18n.Sprintf("not vendored yet · skd update %s", st.Repo)))
		return
	case !st.HasUpdates() && st.Commit == st.Latest:
		c.printf("%s  %s\n", head, dim(i18n.Sprintf("%s  ✓ up to date", short(st.Commit))))
		moved()
		return
	case !st.HasUpdates():
		c.printf("%s  %s\n", head, dim(i18n.Sprintf("%s → %s  no skill changes", short(st.Commit), short(st.Latest))))
		moved()
		return
	}
	c.printf("%s  %s → %s\n", head, dim(short(st.Commit)), accent(short(st.Latest)))
	row := func(label string, style func(string) string, names []string, hint string) {
		if len(names) > 0 {
			line := fmt.Sprintf("  %s %s", style(pad(label, 13)), strings.Join(names, ", "))
			if hint != "" {
				line += "  " + dim(hint)
			}
			c.printf("%s\n", line)
		}
	}
	row(i18n.T("changed"), accent, st.Changed, "")
	row(i18n.T("will add"), ok, st.Pending, "")
	// New skills the pack selects anyway (it follows "*") are under
	// "will add"; the rest are offered.
	var offered []string
	for _, n := range st.New {
		if !slices.Contains(st.Pending, n) {
			offered = append(offered, n)
		}
	}
	row(i18n.T("upstream new"), cyan, offered, "skd add "+st.Repo+" --skill <name>")
	row(i18n.T("upstream gone"), warn, st.Gone, i18n.T("kept by update"))
	moved()
}
