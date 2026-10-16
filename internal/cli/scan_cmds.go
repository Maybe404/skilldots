package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/scan"
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
		t.Row(mark, txt(it.Name), state, detail)
	}
	flush()
	if counts[scan.Copy]+counts[scan.ExtLink]+counts[scan.AppLink]+counts[scan.Broken] > 0 {
		c.printf("%s\n", dim("take these over with: skd adopt"))
	}
}

func scanRow(c *ctx, it scan.Item) (mark, state, detail string) {
	home, _ := os.UserHomeDir()
	short := func(p string) string { return txt(strings.Replace(p, home, "~", 1)) }
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
			d = "from " + txt(it.Source)
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
			props := c.Proposals(rep)
			if len(props) == 0 {
				if !c.jsonOut(map[string]any{"proposals": []app.Proposal{}}) {
					c.printf("%s %s\n", ok("✓"), dim("nothing to adopt"))
				}
				return nil
			}
			if dry || (flags.json && !flags.yes) {
				if !c.jsonOut(map[string]any{"proposals": props, "applied": false}) {
					for _, p := range props {
						c.printf("  %s %-28s %s\n", accent("→"), p.Item.Name, proposalLabel(p))
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
			out, err := c.Adopt(chosen)
			if out != nil && out.Apply != nil {
				printApply(c, out.Apply)
				c.jsonOut(map[string]any{"proposals": chosen, "applied": true, "apply": out.Apply, "failed": out.Failed})
			}
			return err
		},
	}
	cmd.Flags().BoolVarP(&dry, "dry-run", "n", false, "show proposals only")
	return cmd
}

func pickProposals(props []app.Proposal) ([]app.Proposal, error) {
	var opts []huh.Option[int]
	var pre []int
	for i, p := range props {
		if p.Kind == "skip" {
			continue
		}
		opts = append(opts, huh.NewOption(fmt.Sprintf("%-28s %s", p.Item.Name, proposalLabel(p)), i))
		pre = append(pre, i)
	}
	chosen := pre
	if err := ask(huh.NewMultiSelect[int]().Title("Adopt").
		Description("space to toggle · enter to confirm · displaced directories go to the trash, not deleted").
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
	switch p.Kind {
	case "skip":
		return warn(p.Label)
	case "drop":
		return bad(p.Label)
	}
	return p.Label
}
