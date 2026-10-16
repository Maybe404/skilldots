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
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/scan"
	"github.com/Maybe404/skilldots/internal/skillmd"
)

func scanCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Inventory every agent skill directory on this machine",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			rep, err := scan.Run(c.lib, c.reg)
			if err != nil {
				return err
			}
			if c.jsonOut(rep) {
				return nil
			}
			printScan(c, rep, all)
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include entries skilldots already manages")
	return cmd
}

func printScan(c *ctx, rep scan.Report, all bool) {
	counts := map[scan.State]int{}
	for _, it := range rep.Items {
		counts[it.State]++
	}
	c.printf("%s  %s\n\n", bold("scan"), dim(fmt.Sprintf("%d entries · %d managed · %d unmanaged · %d broken",
		len(rep.Items), counts[scan.Managed]+counts[scan.Unlinked], counts[scan.Copy]+counts[scan.ExtLink]+counts[scan.AppLink], counts[scan.Broken])))
	cur := ""
	var t = newTable("", "SKILL", "STATE", "DETAIL")
	flush := func() {
		if cur != "" {
			c.printf("%s\n", t.Render())
		}
	}
	for _, it := range rep.Items {
		if !all && (it.State == scan.Managed || it.State == scan.NotASkill) {
			continue
		}
		if it.Target != cur {
			flush()
			cur = it.Target
			tgt, _ := c.reg.Get(cur)
			c.printf("%s %s\n", accent(cur), dim(tgt.Path))
			t = newTable("", "SKILL", "STATE", "DETAIL")
		}
		mark, state, detail := scanRow(c, it)
		t.Row(mark, it.Name, state, detail)
	}
	flush()
	if counts[scan.Copy]+counts[scan.ExtLink]+counts[scan.AppLink]+counts[scan.Broken] > 0 {
		c.printf("%s\n", dim("take these over with: skd adopt"))
	}
}

func scanRow(c *ctx, it scan.Item) (mark, state, detail string) {
	home, _ := os.UserHomeDir()
	short := func(p string) string { return strings.Replace(p, home, "~", 1) }
	switch it.State {
	case scan.Managed:
		return ok("●"), dim("managed"), dim(short(it.LinkTo))
	case scan.Unlinked:
		return accent("●"), "→ library", dim("reaches the library via " + short(it.LinkTo) + "; not enabled for this agent yet")
	case scan.AppLink:
		return warn("○"), "app", short(it.LinkTo)
	case scan.ExtLink:
		return warn("○"), "link", short(it.LinkTo)
	case scan.Copy:
		d := "copied files"
		if it.Source != "" {
			d = "from " + it.Source
		}
		if it.Duplicate {
			d += "  " + warn("differs from a same-named copy elsewhere")
		}
		return warn("○"), "copy", d
	case scan.Broken:
		return bad("✗"), "broken", dim(short(it.LinkTo))
	}
	return dim("·"), string(it.State), ""
}

// proposal is one adoption the user can accept.
type proposal struct {
	Item   scan.Item
	Kind   string // pack | app | own | drop
	Label  string
	Repo   string
	Skill  string
	Reason string
}

func adoptCmd() *cobra.Command {
	var dry bool
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Bring skills installed by other tools under skilldots management",
		Long: `adopt scans every agent directory and proposes how to take over each
unmanaged entry: copies installed by "npx skills" become subscriptions to
their upstream repository, links into application bundles become [[app]]
entries, and anything else becomes a skill of your own under skills/.
Displaced directories are moved aside, never deleted.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			rep, err := scan.Run(c.lib, c.reg)
			if err != nil {
				return err
			}
			props := proposals(c, rep)
			if len(props) == 0 {
				if !c.jsonOut(map[string]any{"proposals": []proposal{}}) {
					c.printf("%s %s\n", ok("✓"), dim("nothing to adopt"))
				}
				return nil
			}
			if dry || (flags.json && !flags.yes) {
				if !c.jsonOut(map[string]any{"proposals": props, "applied": false}) {
					for _, p := range props {
						c.printf("  %s %-28s %s\n", accent("→"), p.Item.Name, p.Label)
					}
					c.printf("%s\n", dim("run without --dry-run to apply (interactive) or with --yes"))
				}
				return nil
			}
			chosen := props
			if !flags.yes {
				if !isTerminal() {
					return fmt.Errorf("no terminal: pass --yes to adopt everything, or --dry-run to see proposals")
				}
				chosen, err = pickProposals(props)
				if err != nil || len(chosen) == 0 {
					return err
				}
			}
			return applyProposals(c, chosen)
		},
	}
	cmd.Flags().BoolVarP(&dry, "dry-run", "n", false, "show proposals only")
	return cmd
}

func proposals(c *ctx, rep scan.Report) []proposal {
	entries, _ := c.resolve()
	seen := map[string]bool{}
	var out []proposal
	for _, it := range rep.Unmanaged() {
		if seen[it.Name] {
			continue
		}
		if e := engine.Find(entries, it.Name); e != nil {
			// Already in the library: this agent just is not a target yet.
			enabled := false
			for _, t := range e.Targets {
				enabled = enabled || t.Name == it.Target
			}
			if enabled {
				continue
			}
			out = append(out, proposal{Item: it, Kind: "enable", Label: "enable for " + it.Target + " (already in library)"})
			continue
		}
		p := proposal{Item: it}
		switch {
		case it.State == scan.Broken:
			p.Kind, p.Label = "drop", bad("broken link; remove")
		case it.Duplicate:
			p.Kind, p.Label = "skip", warn("same name with different content in several agents; resolve by hand")
		case it.State == scan.AppLink:
			p.Kind, p.Label = "app", "app → "+it.LinkTo
		case it.Source != "":
			p.Kind, p.Repo, p.Skill = "pack", it.Source, it.Name
			p.Label = "subscribe to " + it.Source
		case it.State == scan.ExtLink:
			if repo := gitRemote(it.LinkTo); repo != "" {
				p.Kind, p.Repo, p.Skill, p.Label = "pack", repo, it.Name, "subscribe to "+repo+" (link target is a clone of it)"
			} else {
				p.Kind, p.Label = "own", "copy into skills/ as your own"
			}
		default:
			p.Kind, p.Label = "own", "copy into skills/ as your own (no known upstream)"
		}
		seen[it.Name] = true
		out = append(out, p)
	}
	// "own"/"pack" copies found in several agents: enable the extra agents too.
	for _, it := range rep.Unmanaged() {
		for _, p := range out {
			if p.Item.Name == it.Name && p.Item.Target != it.Target && (p.Kind == "own" || p.Kind == "pack" || p.Kind == "app") {
				out = append(out, proposal{Item: it, Kind: "enable", Label: "enable for " + it.Target})
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Item.Name < out[j].Item.Name })
	return out
}

// gitRemote returns owner/repo when dir is inside a GitHub clone.
func gitRemote(dir string) string {
	if !fsutil.Exists(dir) {
		return ""
	}
	url, err := library.Git(dir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	src, err := pack.ParseSource(url)
	if err != nil || !strings.Contains(src.URL, "github.com") {
		return ""
	}
	return src.Repo
}

func pickProposals(props []proposal) ([]proposal, error) {
	var opts []huh.Option[int]
	var pre []int
	for i, p := range props {
		if p.Kind == "skip" {
			continue
		}
		opts = append(opts, huh.NewOption(fmt.Sprintf("%-28s %s", p.Item.Name, p.Label), i))
		pre = append(pre, i)
	}
	chosen := pre
	if err := ask(huh.NewMultiSelect[int]().Title("Adopt").
		Description("space to toggle · enter to confirm · displaced directories go to the trash, not deleted").
		Options(opts...).Height(18).Value(&chosen)); err != nil {
		return nil, nil
	}
	var out []proposal
	for _, i := range chosen {
		out = append(out, props[i])
	}
	return out, nil
}

func applyProposals(c *ctx, props []proposal) error {
	m := c.lib.Manifest
	packs := map[string]*manifest.Pack{}
	// renamed maps an agent entry name to the skill name it was adopted as.
	renamed := map[string]string{}
	adoptedAs := func(it scan.Item) string {
		if n := renamed[it.Name]; n != "" {
			return n
		}
		return it.Name
	}
	for _, p := range props {
		switch p.Kind {
		case "pack":
			pk := m.FindPack(p.Repo)
			if pk == nil {
				pk = &manifest.Pack{Repo: p.Repo}
				m.Packs = append(m.Packs, pk)
			}
			if !pk.UsesAll() {
				pk.Use = dedupe(append(pk.Use, p.Skill))
			}
			packs[p.Repo] = pk
		case "app":
			dup := false
			for _, a := range m.Apps {
				if a.Name == p.Item.Name {
					dup = true
					if a.Path != p.Item.LinkTo {
						c.printf("%s [[app]] %s already points at %s; not changing it to %s\n", warn("!"), a.Name, a.Path, p.Item.LinkTo)
					}
				}
			}
			if !dup {
				m.Apps = append(m.Apps, &manifest.App{Name: p.Item.Name, Path: p.Item.LinkTo})
			}
		case "own":
			src := p.Item.Path
			if p.Item.LinkTo != "" {
				src = p.Item.LinkTo
			}
			// The library knows a skill by its frontmatter name, which may
			// differ from the directory name it had in the agent dir.
			name := p.Item.Name
			if meta, err := skillmd.Read(src); err == nil && meta.Name != name && manifest.CheckName(meta.Name) == nil {
				name = meta.Name
				renamed[p.Item.Name] = name
			}
			dest := filepath.Join(c.lib.SkillsDir(), name)
			if !fsutil.Lexists(dest) {
				if err := fsutil.CopyTree(src, dest); err != nil {
					return err
				}
				c.printf("%s copied %s → %s\n", ok("✓"), p.Item.Name, c.lib.Rel(dest))
			}
		case "drop":
			if err := os.Remove(p.Item.Path); err != nil {
				return err
			}
			c.printf("%s removed broken link %s\n", ok("✓"), p.Item.Path)
		}
	}
	failed := 0
	for repo, pk := range packs {
		rep, err := engine.Sync(c.lib, pk)
		if err != nil {
			c.printf("%s %s: %v\n", bad("✗"), repo, err)
			failed++
			continue
		}
		c.printf("%s %s %s\n", ok("✓"), repo, dim("@ "+short(rep.NewCommit)))
		// Names the upstream no longer has stay as the user's own copy.
		for _, n := range pk.Use {
			if pack.Find(rep.Upstream, n) == nil {
				c.printf("  %s %s is not in %s any more; keeping your copy under skills/\n", warn("!"), n, repo)
				for _, p := range props {
					if p.Kind != "pack" || p.Skill != n {
						continue
					}
					src := p.Item.Path
					if p.Item.LinkTo != "" {
						src = p.Item.LinkTo
					}
					dest := filepath.Join(c.lib.SkillsDir(), n)
					if fsutil.Lexists(dest) {
						c.printf("  %s %s already exists; leaving it as is\n", warn("!"), c.lib.Rel(dest))
						break
					}
					if err := fsutil.CopyTree(src, dest); err != nil {
						return err
					}
					break
				}
				var use []string
				for _, u := range pk.Use {
					if u != n {
						use = append(use, u)
					}
				}
				pk.Use = use
			}
		}
	}
	if err := c.lib.Save(); err != nil {
		return err
	}
	// Targets: every adopted entry is enabled for each agent it was found in.
	entries, err := c.resolve()
	if err != nil {
		return err
	}
	// A real directory is only replaced by a link when the library holds
	// exactly its content; otherwise it may be the user's only copy of
	// something, so it stays where it is.
	keep := map[string]bool{}
	for _, p := range props {
		if p.Kind == "skip" || p.Kind == "drop" || p.Item.LinkTo != "" {
			continue
		}
		e := engine.Find(entries, adoptedAs(p.Item))
		if e == nil {
			continue
		}
		h, err := fsutil.HashTree(e.Dir)
		if err == nil && fsutil.Pristine(p.Item.Path, h) {
			continue
		}
		keep[p.Item.Target+"/"+p.Item.Name] = true
		c.printf("%s %s differs from the library's %s (%s); left in place — compare them and remove one by hand\n",
			warn("!"), p.Item.Path, adoptedAs(p.Item), e.Dir)
	}
	want := map[string][]string{}
	for _, p := range props {
		if p.Kind == "skip" || p.Kind == "drop" || keep[p.Item.Target+"/"+p.Item.Name] {
			continue
		}
		want[adoptedAs(p.Item)] = append(want[adoptedAs(p.Item)], p.Item.Target)
	}
	for name, extra := range want {
		e := engine.Find(entries, name)
		if e == nil {
			continue
		}
		next := dedupe(append(append([]string{}, e.TargetNames...), extra...))
		if len(next) != len(e.TargetNames) {
			editTargets(c, e, next)
		}
	}
	if err := c.lib.Save(); err != nil {
		return err
	}
	// Force only the agent entries this adoption takes over; anything else
	// that is modified or unmanaged keeps being skipped.
	force := map[string]bool{}
	for _, p := range props {
		if p.Kind != "skip" && p.Kind != "drop" && !keep[p.Item.Target+"/"+p.Item.Name] {
			force[p.Item.Target+"/"+p.Item.Name] = true
		}
	}
	out, err := doApply(c, false, false, force)
	if err != nil {
		return err
	}
	// An entry adopted under another name now has a link of that name next
	// to it; move the original aside so the agent does not see the skill twice.
	for _, p := range props {
		n := adoptedAs(p.Item)
		if p.Kind == "skip" || p.Kind == "drop" || n == p.Item.Name || keep[p.Item.Target+"/"+p.Item.Name] {
			continue
		}
		t, err := c.reg.Get(p.Item.Target)
		if err != nil || c.lib.Lock.Links[t.Name+"/"+n] == nil || !fsutil.Lexists(filepath.Join(t.Path, n)) || !fsutil.Lexists(p.Item.Path) {
			continue
		}
		to, err := engine.Trash(c.lib, p.Item.Path)
		if err != nil {
			return err
		}
		if to == "" {
			c.printf("  %s %s (adopted as %s)\n", dim("removed link"), p.Item.Path, n)
			continue
		}
		out.Backups = append(out.Backups, p.Item.Path+" → "+c.lib.Rel(to))
		c.printf("  %s %s → %s (adopted as %s)\n", dim("moved aside"), p.Item.Path, c.lib.Rel(to), n)
	}
	c.jsonOut(map[string]any{"proposals": props, "applied": true, "apply": out, "failed": failed})
	if failed > 0 {
		return fmt.Errorf("%d repositories could not be fetched", failed)
	}
	return nil
}
