package cli

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/scan"
)

func scanCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "scan",
		Short: i18n.T("Inventory every agent skill directory on this machine"),
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
	cmd.Flags().BoolVar(&all, "all", false, i18n.T("include entries skilldots already manages"))
	return cmd
}

func printScan(c *ctx, rep scan.Report, all bool) {
	counts := map[scan.State]int{}
	for _, it := range rep.Items {
		counts[it.State]++
	}
	c.printf("%s  %s\n\n", bold(i18n.T("scan")), dim(i18n.Sprintf("%d entries · %d managed · %d unmanaged · %d broken",
		len(rep.Items), counts[scan.Managed]+counts[scan.Unlinked], counts[scan.Copy]+counts[scan.ExtLink]+counts[scan.AppLink], counts[scan.Broken])))
	// What adopt would leave to the user, said here rather than "skd adopt".
	need := map[string]string{}
	actionable := 0
	for _, p := range c.ScanAdvice(rep) {
		if p.Kind == "skip" {
			need[p.Item.Path] = p.Label
		} else {
			actionable++
		}
	}
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
			where := i18n.T("(not in use)")
			if tgt, err := c.reg.Get(cur); err == nil {
				where = tildePath(tgt.Path)
			}
			c.printf("%s %s\n", accent(txt(cur)), dim(where))
			t = newTable("", "SKILL", "STATE", "DETAIL")
		}
		mark, state, detail := scanRow(c, it)
		if why := need[it.Path]; why != "" {
			detail += "  " + warn(txt(why))
		}
		t.Row(mark, txt(it.Name), state, detail)
	}
	flush()
	if actionable > 0 {
		c.printf("%s\n", dim(i18n.T("take these over with: skd adopt")))
	}
	if len(need) > 0 {
		c.printf("%s %s\n", warn("!"), i18n.Count(len(need), "%d entry needs you: skd adopt leaves it as it is (why above)", "%d entries need you: skd adopt leaves them as they are (why above)"))
	}
	if counts[scan.Left] > 0 {
		c.printf("%s\n", dim(i18n.T("left: links into the library, recorded for targets no longer in use; skd leaves them in place (they may be the user's own)")))
	}
}

func scanRow(c *ctx, it scan.Item) (mark, state, detail string) {
	short := func(p string) string { return txt(tildePath(p)) }
	switch it.State {
	case scan.Managed:
		return ok("●"), dim(i18n.T("managed")), dim(short(it.LinkTo))
	case scan.Unlinked:
		return accent("●"), i18n.T("→ library"), dim(i18n.Sprintf("reaches the library via %s; not enabled for this agent yet", short(it.LinkTo)))
	case scan.AppLink:
		return warn("○"), i18n.T("app"), short(it.LinkTo)
	case scan.ExtLink:
		return warn("○"), i18n.T("link"), short(it.LinkTo)
	case scan.Copy:
		d := i18n.T("copied files")
		if it.Source != "" {
			d = i18n.Sprintf("from %s", txt(it.Source))
		}
		if it.Duplicate {
			d += "  " + warn(i18n.T("differs from a same-named copy elsewhere"))
		}
		return warn("○"), i18n.T("copy"), d
	case scan.Broken:
		return bad("✗"), i18n.T("broken"), dim(short(it.LinkTo))
	case scan.Left:
		return warn("○"), i18n.T("left"), dim(i18n.Sprintf("%s → %s; target not in use, skd no longer manages it and leaves it in place", short(it.Path), short(it.LinkTo)))
	}
	return dim("·"), string(it.State), ""
}

func adoptCmd() *cobra.Command {
	var dry bool
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: i18n.T("Bring skills installed by other tools under skilldots management"),
		Long: i18n.T(`adopt scans every agent directory and proposes how to take over each
unmanaged entry: copies installed by "npx skills" become subscriptions to
their upstream repository, links into application bundles become [[app]]
entries, and anything else becomes a skill of your own under skills/.
Displaced directories are moved aside, never deleted.`),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			rep, err := scan.Run(c.lib, c.reg)
			if err != nil {
				return err
			}
			props := c.Proposals(rep)
			if len(props) == 0 {
				if !c.jsonOut(map[string]any{"proposals": []app.Proposal{}}) {
					c.printf("%s %s\n", ok("✓"), dim(i18n.T("nothing to adopt")))
				}
				return nil
			}
			if dry || (flags.json && !flags.yes) {
				if !c.jsonOut(map[string]any{"proposals": props, "applied": false}) {
					for _, p := range props {
						c.printf("  %s %-28s %s\n", accent("→"), txt(p.Item.Name), proposalLabel(p))
					}
					c.printf("%s\n", dim(i18n.T("run without --dry-run to apply (interactive) or with --yes")))
				}
				return nil
			}
			// What adopt leaves to the user is said, in every mode.
			for _, p := range props {
				if p.Kind == "skip" {
					c.printf("%s %s  %s\n", warn("!"), txt(tildePath(p.Item.Path)), proposalLabel(p))
				}
			}
			chosen := props
			if !flags.yes {
				if !isTerminal() {
					return i18n.Errorf("no terminal: pass --yes to adopt everything, or --dry-run to see proposals")
				}
				chosen, err = pickProposals(props)
				if err != nil || len(chosen) == 0 {
					return err
				}
			}
			out, err := c.Adopt(chosen)
			if out != nil && out.Apply != nil {
				printApply(c, out.Apply)
				c.jsonOut(map[string]any{"proposals": chosen, "applied": true, "apply": out.Apply, "failed": out.Failed})
			}
			return err
		},
	}
	cmd.Flags().BoolVarP(&dry, "dry-run", "n", false, i18n.T("show proposals only"))
	return cmd
}

func pickProposals(props []app.Proposal) ([]app.Proposal, error) {
	var opts []huh.Option[int]
	var pre []int
	for i, p := range props {
		if p.Kind == "skip" {
			continue
		}
		opts = append(opts, huh.NewOption(fmt.Sprintf("%-28s %s", txt(p.Item.Name), proposalLabel(p)), i))
		pre = append(pre, i)
	}
	chosen := pre
	if err := ask(huh.NewMultiSelect[int]().Title(i18n.T("Adopt")).
		Description(i18n.T("space to toggle · enter to confirm · displaced directories go to the trash, not deleted")).
		Options(opts...).Height(18).Value(&chosen)); err != nil {
		return nil, nil
	}
	var out []app.Proposal
	for _, i := range chosen {
		out = append(out, props[i])
	}
	return out, nil
}

// proposalLabel colours a proposal's label by what accepting it does.
func proposalLabel(p app.Proposal) string {
	l := txt(p.Label)
	switch p.Kind {
	case "skip":
		return warn(l)
	case "drop":
		return bad(l)
	}
	return l
}
