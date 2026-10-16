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

	"github.com/Maybe404/skilldots/internal/fsutil"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/skillmd"
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
			m := c.lib.Manifest
			arg := args[0]
			if existing := m.FindPack(arg); existing != nil {
				arg = existing.Repo
			}
			src, err := pack.ParseSource(arg)
			if err != nil {
				return err
			}
			p := m.FindPack(src.Repo)
			isNew := p == nil
			if isNew {
				p = &manifest.Pack{Repo: src.Repo, Ref: src.Ref, Alias: alias}
				m.Packs = append(m.Packs, p)
			} else if alias != "" {
				p.Alias = alias
			}
			if len(targetNames) > 0 {
				t := targetNames
				p.Targets = &t
			}

			c.printf("%s %s\n", dim("fetching"), src.Repo)
			clone, _, err := pack.Fetch(pack.Source{Repo: src.Repo, URL: src.URL, Ref: p.Ref})
			if err != nil {
				if isNew {
					m.Packs = m.Packs[:len(m.Packs)-1]
				}
				return err
			}
			upstream, rejected, err := pack.Scan(clone)
			if err != nil {
				return err
			}
			printRejected(c, src.Repo, rejected)
			if len(upstream) == 0 {
				if isNew {
					m.Packs = m.Packs[:len(m.Packs)-1]
				}
				return fmt.Errorf("%s contains no SKILL.md", src.Repo)
			}

			var chosen []string
			switch {
			case all:
				p.Use = []string{"*"}
			case len(skills) > 0 || len(categories) > 0:
				for _, s := range skills {
					if pack.Find(upstream, s) == nil {
						return fmt.Errorf("no skill %q in %s (try `skd add %s` without --skill to browse)", s, src.Repo, src.Repo)
					}
					chosen = append(chosen, s)
				}
				for _, cat := range categories {
					n := 0
					for _, s := range upstream {
						if s.Category == cat || strings.HasPrefix(s.Category, cat+"/") {
							chosen = append(chosen, s.Name)
							n++
						}
					}
					if n == 0 {
						if isNew {
							m.Packs = m.Packs[:len(m.Packs)-1]
						}
						return fmt.Errorf("no category %q in %s (categories: %s)", cat, src.Repo, strings.Join(nonEmpty(pack.Categories(upstream)), ", "))
					}
				}
			default:
				if c.jsonOut(upstream) {
					return nil
				}
				chosen, err = pickSkills(c, src.Repo, upstream, p)
				if err != nil {
					return err
				}
				if len(chosen) == 1 && chosen[0] == "*" {
					p.Use, chosen = []string{"*"}, nil
				}
			}
			if !p.UsesAll() {
				p.Use = dedupe(append(p.Use, chosen...))
			}
			if len(p.Use) == 0 {
				if isNew {
					m.Packs = m.Packs[:len(m.Packs)-1]
				}
				c.printf("%s\n", dim("nothing selected"))
				return nil
			}

			rep, err := engine.Sync(c.lib, p)
			if err != nil {
				return err
			}
			printRenamed(c, rep)
			if err := c.lib.Save(); err != nil {
				return err
			}
			warnLicense(c, rep)
			c.printf("%s vendored %s from %s %s\n", ok("✓"), plural(len(rep.Added)+len(rep.Changed)+len(rep.Unchanged), "skill"), p.Repo, dim("@ "+short(rep.NewCommit)))
			out, err := doApply(c, false, false, nil)
			if err != nil {
				return err
			}
			if flags.json {
				entries, _ := c.resolve()
				linked := []string{}
				for _, e := range entries {
					if e.Kind == engine.KindVendor && e.Origin == p.Repo {
						linked = append(linked, e.LinkName)
					}
				}
				c.jsonOut(map[string]any{"repo": p.Repo, "commit": rep.NewCommit, "skills": linked,
					"added": nonNil(rep.Added), "changed": nonNil(rep.Changed), "unchanged": nonNil(rep.Unchanged),
					"noLicense": nonNil(rep.NoLicenseList), "renamed": rep.Renamed, "rejected": nonNil(rep.Rejected),
					"changes": out.Changes, "kept": out.Kept, "backups": out.Backups})
			}
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
func pickSkills(c *ctx, repo string, upstream []pack.Skill, p *manifest.Pack) ([]string, error) {
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
			if p.Selected(s.Name, s.Category) {
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

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
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
			m := c.lib.Manifest
			var packs []*manifest.Pack
			if len(args) == 0 {
				packs = m.Packs
			}
			for _, a := range args {
				p := m.FindPack(a)
				if p == nil {
					return fmt.Errorf("no pack %q", a)
				}
				packs = append(packs, p)
			}
			type failure struct {
				Repo  string `json:"repo"`
				Error string `json:"error"`
			}
			reports := []*engine.SyncReport{}
			failed := []failure{}
			changed := false
			for _, p := range packs {
				rep, err := engine.Sync(c.lib, p)
				if err != nil {
					failed = append(failed, failure{p.Repo, err.Error()})
					c.printf("%s %s: %v\n", bad("✗"), p.Repo, err)
					continue
				}
				reports = append(reports, rep)
				printReport(c, rep)
				printRenamed(c, rep)
				warnLicense(c, rep)
				changed = changed || len(rep.Changed)+len(rep.Added)+len(rep.Dropped) > 0 || len(rep.Renamed) > 0
			}
			// Whatever succeeded is saved and linked even if another pack failed.
			if err := c.lib.Save(); err != nil {
				return err
			}
			out, err := doApply(c, false, false, nil)
			if err != nil {
				return err
			}
			committed := false
			if commit && changed {
				if committed, err = gitCommit(c, "chore(skills): update vendored skills", true); err != nil {
					return err
				}
			}
			c.jsonOut(map[string]any{"packs": reports, "failed": failed, "apply": out, "committed": committed})
			if len(failed) > 0 {
				return fmt.Errorf("%d pack(s) failed: %s", len(failed), failed[0].Repo)
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

// gitCommit commits the library's own files and, with push, pushes when
// the branch has an upstream. It reports whether a commit was made.
func gitCommit(c *ctx, msg string, push bool) (bool, error) {
	root := c.lib.Root
	if _, err := c.lib.Git("add", "-A", "--", "."); err != nil {
		return false, err
	}
	if _, err := c.lib.Git("diff", "--cached", "--quiet", "--", "."); err == nil {
		return false, nil
	}
	if _, err := c.lib.Git("commit", "-q", "-m", msg, "--", "."); err != nil {
		return false, err
	}
	if _, err := c.lib.Git("rev-parse", "--abbrev-ref", "@{u}"); push && err == nil {
		if _, err := c.lib.Git("push", "-q"); err != nil {
			return true, err
		}
	}
	c.printf("%s committed in %s\n", ok("✓"), root)
	return true, nil
}

// setTargets edits the targets of one skill and re-applies.
func setTargets(c *ctx, key string, edit func(cur []string) []string) error {
	entries, err := c.resolve()
	if err != nil {
		return err
	}
	e := engine.Find(entries, key)
	if e == nil {
		if p := c.lib.Manifest.FindPack(key); p != nil {
			cur := e2names(c.lib.Manifest.Targets.Default)
			if p.Targets != nil {
				cur = *p.Targets
			}
			next := edit(cur)
			p.Targets = &next
			if err := c.lib.Save(); err != nil {
				return err
			}
			return runApply(c, false, false)
		}
		return fmt.Errorf("no skill or pack %q", key)
	}
	editTargets(c, e, edit(e.TargetNames))
	if err := c.lib.Save(); err != nil {
		return err
	}
	return runApply(c, false, false)
}

// editTargets records next as the target list of e in the manifest.
func editTargets(c *ctx, e *engine.Entry, next []string) {
	m := c.lib.Manifest
	switch e.Kind {
	case engine.KindVendor:
		p := m.FindPack(e.Origin)
		p.Override(e.Name, true).Targets = &next
	case engine.KindApp:
		for _, a := range m.Apps {
			if a.Name == e.Name {
				a.Targets = &next
			}
		}
	default:
		found := false
		for _, l := range m.Locals {
			if l.Name == e.Name {
				l.Targets, found = &next, true
			}
		}
		if !found {
			m.Locals = append(m.Locals, &manifest.Local{Name: e.Name, Targets: &next})
		}
	}
}

func e2names(s []string) []string { return append([]string(nil), s...) }

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
			return setTargets(c, args[0], func(cur []string) []string {
				if len(agents) == 0 {
					return dedupe(append(cur, c.lib.Manifest.Targets.Default...))
				}
				return dedupe(append(cur, agents...))
			})
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
			return setTargets(c, args[0], func(cur []string) []string {
				if len(agents) == 0 {
					return []string{}
				}
				drop := map[string]bool{}
				for _, a := range agents {
					drop[a] = true
				}
				var out []string
				for _, t := range cur {
					if !drop[t] {
						out = append(out, t)
					}
				}
				if out == nil {
					out = []string{}
				}
				return out
			})
		},
	}
	cmd.Flags().StringSliceVar(&agents, "agent", nil, "targets to remove (default: all)")
	return cmd
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
			entries, err := c.resolve()
			if err != nil {
				return err
			}
			e := engine.Find(entries, args[0])
			if e == nil {
				return fmt.Errorf("no skill %q", args[0])
			}
			if e.Kind != engine.KindVendor {
				return fmt.Errorf("%q is a %s skill; use `skd disable` to unlink it, or delete the directory to drop it", args[0], e.Kind)
			}
			p := c.lib.Manifest.FindPack(e.Origin)
			if p.UsesAll() {
				p.Skip = dedupe(append(p.Skip, e.Name))
			} else {
				var use []string
				for _, u := range p.Use {
					if u != e.Name && u != e.Category+"/"+e.Name {
						use = append(use, u)
					}
				}
				p.Use = use
			}
			delete(p.Skill, e.Name)
			if len(p.Use) == 0 && !p.UsesAll() {
				// Removing the last skill of a pack is the whole pack: the
				// user named exactly what goes, so no further confirmation.
				c.printf("%s\n", dim(p.Repo+" has no skills left; dropping the pack"))
				out, err := purgePack(c, p, false)
				if err != nil {
					return err
				}
				c.jsonOut(map[string]any{"removed": e.LinkName, "purged": out})
				return nil
			}
			if _, err := engine.Sync(c.lib, p); err != nil {
				return err
			}
			if err := c.lib.Save(); err != nil {
				return err
			}
			out, err := doApply(c, false, false, nil)
			if err != nil {
				return err
			}
			c.jsonOut(map[string]any{"removed": e.LinkName, "apply": out})
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
			out, err := purgePack(c, p, true)
			if err != nil {
				return err
			}
			c.jsonOut(out)
			return nil
		},
	}
	return cmd
}

type purgeOutcome struct {
	Repo     string        `json:"repo"`
	Unlinked []string      `json:"unlinked"`
	Apply    *applyOutcome `json:"apply"`
}

// purgePack drops a pack from manifest, lock, vendor/ (to the trash) and
// every agent directory. ask asks for confirmation first.
func purgePack(c *ctx, p *manifest.Pack, ask bool) (*purgeOutcome, error) {
	entries, _ := c.resolve()
	links := []string{}
	for _, e := range entries {
		if e.Kind == engine.KindVendor && e.Origin == p.Repo {
			for _, t := range e.Targets {
				links = append(links, t.Name+"/"+e.LinkName)
			}
		}
	}
	sort.Strings(links)
	c.printf("%s %s: unlink %s and move vendor/%s to the trash\n", warn("purge"), bold(p.Repo), plural(len(links), "link"), p.Repo)
	for _, l := range links {
		c.printf("  %s %s\n", bad("−"), l)
	}
	if ask && !confirm("proceed?") {
		return nil, fmt.Errorf("cancelled")
	}
	m := c.lib.Manifest
	for i, q := range m.Packs {
		if q == p {
			m.Packs = append(m.Packs[:i], m.Packs[i+1:]...)
			break
		}
	}
	if err := c.lib.Save(); err != nil {
		return nil, err
	}
	out, err := doApply(c, false, false, nil)
	if err != nil {
		return nil, err
	}
	if err := engine.RemovePack(c.lib, p.Repo); err != nil {
		return nil, err
	}
	if err := c.lib.Save(); err != nil {
		return nil, err
	}
	return &purgeOutcome{Repo: p.Repo, Unlinked: links, Apply: out}, nil
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
			e := engine.Find(entries, args[0])
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
			entries, err := c.resolve()
			if err != nil {
				return err
			}
			e := engine.Find(entries, args[0])
			if e == nil {
				return fmt.Errorf("no skill %q", args[0])
			}
			if e.Kind != engine.KindVendor && e.Kind != engine.KindApp {
				return fmt.Errorf("%q is already yours (%s)", args[0], e.Dir)
			}
			name := e.LinkName
			if as != "" {
				if err := manifest.CheckName(as); err != nil {
					return fmt.Errorf("--as: %w", err)
				}
				name = as
			}
			base := c.lib.SkillsDir()
			if private {
				base = c.lib.PrivateDir()
			}
			dest := filepath.Join(base, name)
			if fsutil.Lexists(dest) {
				return fmt.Errorf("%s already exists", dest)
			}
			if err := fsutil.CopyTree(e.Dir, dest); err != nil {
				return err
			}
			// The fork is known by its directory name; make the frontmatter
			// agree, or an aliased fork would clash with the upstream name.
			if meta, err := skillmd.Read(dest); err == nil && meta.Name != name {
				if err := skillmd.SetName(dest, name); err != nil {
					return err
				}
			}
			note := fmt.Sprintf("\n<!-- forked by skilldots from %s -->\n", engine.Describe(*e))
			f, err := os.OpenFile(filepath.Join(dest, "SKILL.md"), os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				f.WriteString(note)
				f.Close()
			}
			// Deselect the vendored original so the names do not collide.
			if e.Kind == engine.KindVendor {
				p := c.lib.Manifest.FindPack(e.Origin)
				if p.UsesAll() {
					p.Skip = dedupe(append(p.Skip, e.Name))
				} else {
					var use []string
					for _, u := range p.Use {
						if u != e.Name && u != e.Category+"/"+e.Name {
							use = append(use, u)
						}
					}
					p.Use = use
				}
				delete(p.Skill, e.Name)
				if _, err := engine.Sync(c.lib, p); err != nil {
					return err
				}
			} else {
				for i, a := range c.lib.Manifest.Apps {
					if a.Name == e.Name {
						c.lib.Manifest.Apps = append(c.lib.Manifest.Apps[:i], c.lib.Manifest.Apps[i+1:]...)
						break
					}
				}
			}
			if err := c.lib.Save(); err != nil {
				return err
			}
			c.printf("%s forked %s → %s\n", ok("✓"), args[0], dest)
			out, err := doApply(c, false, false, nil)
			if err != nil {
				return err
			}
			c.jsonOut(map[string]any{"forked": args[0], "name": name, "path": dest, "apply": out})
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
