package cli

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/manifest"
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

			c.printf("fetching %s …\n", src.Repo)
			clone, _, err := pack.Fetch(pack.Source{Repo: src.Repo, URL: src.URL, Ref: p.Ref})
			if err != nil {
				if isNew {
					m.Packs = m.Packs[:len(m.Packs)-1]
				}
				return err
			}
			upstream, err := pack.Scan(clone)
			if err != nil {
				return err
			}
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
				for _, s := range upstream {
					for _, cat := range categories {
						if s.Category == cat || strings.HasPrefix(s.Category, cat+"/") {
							chosen = append(chosen, s.Name)
						}
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
				c.printf("nothing selected\n")
				return nil
			}

			// Give new skills a free link name before resolving.
			entries, _ := c.resolve()
			for _, s := range upstream {
				if !p.Selected(s.Name, s.Category) {
					continue
				}
				if prev := engine.Find(entries, s.Name); prev != nil && !(prev.Kind == engine.KindVendor && prev.Origin == p.Repo) {
					if o := p.Override(s.Name, true); o.As == "" {
						o.As = engine.FreeName(entries, s.Name, p.Prefix())
						c.printf("⚠ %q already exists (%s); linking as %q\n", s.Name, engine.Describe(*prev), o.As)
					}
				}
			}

			rep, err := engine.Sync(c.lib, p)
			if err != nil {
				return err
			}
			warnLicense(c, rep)
			if err := c.lib.Save(); err != nil {
				return err
			}
			c.printf("vendored %d skill(s) from %s @ %s\n", len(rep.Added)+len(rep.Changed)+len(rep.Unchanged), p.Repo, rep.NewCommit[:7])
			return runApply(c, false, false)
		},
	}
	cmd.Flags().StringSliceVarP(&skills, "skill", "s", nil, "skill names to select (repeatable)")
	cmd.Flags().StringSliceVar(&categories, "category", nil, "select every skill in a category")
	cmd.Flags().BoolVar(&all, "all", false, "follow every skill, including ones added upstream later")
	cmd.Flags().StringVar(&alias, "alias", "", "short name for the pack")
	cmd.Flags().StringSliceVar(&targetNames, "agent", nil, "targets for this pack (default: manifest default)")
	return cmd
}

// pickSkills prints the upstream skills grouped by category and reads a
// selection. It returns ["*"] for everything.
func pickSkills(c *ctx, repo string, upstream []pack.Skill, p *manifest.Pack) ([]string, error) {
	if !isTerminal() {
		return nil, fmt.Errorf("no terminal: pass --skill, --category or --all")
	}
	c.printf("\n%s — %d skills\n", repo, len(upstream))
	idx := map[int]string{}
	n := 0
	for _, cat := range pack.Categories(upstream) {
		title := cat
		if title == "" {
			title = "(root)"
		}
		c.printf("\n  %s\n", title)
		for _, s := range upstream {
			if s.Category != cat {
				continue
			}
			n++
			idx[n] = s.Name
			mark := " "
			if p.Selected(s.Name, s.Category) {
				mark = "✓"
			}
			desc := s.Description
			if len(desc) > 70 {
				desc = desc[:67] + "…"
			}
			c.printf("  %s %3d  %-28s %s\n", mark, n, s.Name, desc)
		}
	}
	c.printf("\nSelect: numbers (1,3,5-8), names, a category name, 'all', or empty to cancel\n> ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}
	if line == "all" || line == "*" {
		return []string{"*"}, nil
	}
	var out []string
	for _, tok := range strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' }) {
		if a, b, ok := strings.Cut(tok, "-"); ok {
			var lo, hi int
			if _, err := fmt.Sscanf(a+" "+b, "%d %d", &lo, &hi); err == nil {
				for i := lo; i <= hi; i++ {
					if name, ok := idx[i]; ok {
						out = append(out, name)
					}
				}
				continue
			}
		}
		var i int
		if _, err := fmt.Sscanf(tok, "%d", &i); err == nil && idx[i] != "" {
			out = append(out, idx[i])
			continue
		}
		matched := false
		for _, s := range upstream {
			if s.Name == tok || s.Category == tok || strings.HasPrefix(s.Category, tok+"/") {
				out = append(out, s.Name)
				matched = true
			}
		}
		if !matched {
			return nil, fmt.Errorf("unknown selection %q", tok)
		}
	}
	return dedupe(out), nil
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

func warnLicense(c *ctx, rep *engine.SyncReport) {
	if rep.NoLicense {
		c.printf("⚠ %s: no license found for %s — check before redistributing\n", rep.Repo, strings.Join(rep.NoLicenseList, ", "))
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
			var reports []*engine.SyncReport
			var failed []string
			for _, p := range packs {
				rep, err := engine.Sync(c.lib, p)
				if err != nil {
					failed = append(failed, fmt.Sprintf("%s: %v", p.Repo, err))
					continue
				}
				reports = append(reports, rep)
			}
			if c.jsonOut(reports) {
				return nil
			}
			changed := false
			for _, r := range reports {
				printReport(c, r)
				warnLicense(c, r)
				changed = changed || len(r.Changed)+len(r.Added)+len(r.Dropped) > 0
			}
			for _, f := range failed {
				c.printf("✗ %s\n", f)
			}
			if err := c.lib.Save(); err != nil {
				return err
			}
			if err := runApply(c, false, false); err != nil {
				return err
			}
			if commit && changed {
				if err := gitCommit(c, "chore(skills): update vendored skills"); err != nil {
					return err
				}
			}
			if len(failed) > 0 {
				return fmt.Errorf("%d pack(s) failed", len(failed))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&commit, "commit", false, "git commit and push the library when something changed")
	return cmd
}

func printReport(c *ctx, r *engine.SyncReport) {
	head := r.Repo
	if r.OldCommit == "" {
		head += fmt.Sprintf("  (new @ %s)", short(r.NewCommit))
	} else if r.OldCommit == r.NewCommit {
		head += fmt.Sprintf("  %s  up to date", short(r.NewCommit))
	} else {
		head += fmt.Sprintf("  %s → %s", short(r.OldCommit), short(r.NewCommit))
	}
	c.printf("%s\n", head)
	row := func(label string, names []string) {
		if len(names) > 0 {
			c.printf("  %-14s %s\n", label, strings.Join(names, ", "))
		}
	}
	row("changed", r.Changed)
	row("added", r.Added)
	row("dropped", r.Dropped)
	if n := len(r.UpstreamNew); n > 0 {
		shown := r.UpstreamNew
		more := ""
		if n > 5 {
			shown, more = shown[:5], fmt.Sprintf(" … %d more (skd add %s to browse)", n-5, r.Repo)
		}
		c.printf("  %-14s %s%s\n", "upstream new", strings.Join(shown, ", "), more)
	}
	row("upstream gone", r.UpstreamGone)
	if len(r.Changed) > 0 {
		c.printf("  (see changes: git -C %s diff -- vendor/%s)\n", c.lib.Root, r.Repo)
	}
}

func short(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func gitCommit(c *ctx, msg string) error {
	root := c.lib.Root
	if _, err := c.lib.Git("add", "-A", "--", ":!private"); err != nil {
		return err
	}
	if _, err := c.lib.Git("diff", "--cached", "--quiet"); err == nil {
		return nil
	}
	if _, err := c.lib.Git("commit", "-q", "-m", msg); err != nil {
		return err
	}
	if _, err := c.lib.Git("rev-parse", "--abbrev-ref", "@{u}"); err == nil {
		if _, err := c.lib.Git("push", "-q"); err != nil {
			return err
		}
	}
	c.printf("committed in %s\n", root)
	return nil
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
	next := edit(e.TargetNames)
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
	if err := c.lib.Save(); err != nil {
		return err
	}
	return runApply(c, false, false)
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
				c.printf("%s has no skills left; purging the pack\n", p.Repo)
				return purgePack(c, p)
			}
			if _, err := engine.Sync(c.lib, p); err != nil {
				return err
			}
			if err := c.lib.Save(); err != nil {
				return err
			}
			return runApply(c, false, false)
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
			return purgePack(c, p)
		},
	}
	return cmd
}

func purgePack(c *ctx, p *manifest.Pack) error {
	entries, _ := c.resolve()
	var links []string
	for _, e := range entries {
		if e.Kind == engine.KindVendor && e.Origin == p.Repo {
			for _, t := range e.Targets {
				links = append(links, t.Name+"/"+e.LinkName)
			}
		}
	}
	sort.Strings(links)
	c.printf("purge %s: remove vendor/%s and %d link(s)\n", p.Repo, p.Repo, len(links))
	for _, l := range links {
		c.printf("  - %s\n", l)
	}
	if !confirm("proceed?") {
		return fmt.Errorf("cancelled")
	}
	m := c.lib.Manifest
	for i, q := range m.Packs {
		if q == p {
			m.Packs = append(m.Packs[:i], m.Packs[i+1:]...)
			break
		}
	}
	if err := c.lib.Save(); err != nil {
		return err
	}
	if err := runApply(c, false, false); err != nil {
		return err
	}
	if err := engine.RemovePack(c.lib, p.Repo); err != nil {
		return err
	}
	return c.lib.Save()
}
