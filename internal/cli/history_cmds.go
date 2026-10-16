package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
)

func historyCmd() *cobra.Command {
	var n int
	cmd := &cobra.Command{
		Use:   "history [pack]",
		Short: "What each update changed: commits, skills, and upstream's commit messages",
		Long: "Every `skd update` records, per pack it moved, the old and new commit, the skills that changed, were added, " +
			"moved, went away upstream or appeared there, and the subjects of the upstream commits that touched them (at most " +
			fmt.Sprint(app.MaxCommits) + "). The record is " + app.HistoryFile + " in the library: it is committed and pushed with it, " +
			"so another machine sees it after a clone. Reading it needs no network.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			p := ""
			if len(args) == 1 {
				p = args[0]
			}
			recs, skipped, err := c.History(p, n)
			if err != nil {
				return err
			}
			if skipped > 0 {
				fmt.Fprintf(stderr, "%s %s: %d line(s) could not be read and were skipped\n", warn("!"), c.HistoryPath(), skipped)
			}
			if c.jsonOut(map[string]any{"records": recs, "skipped": skipped}) {
				return nil
			}
			if len(recs) == 0 {
				what := "no updates recorded yet"
				if p != "" {
					what = "no updates of " + txt(p) + " recorded"
				}
				c.printf("%s\n", dim(what+" · skd update records each one"))
				return nil
			}
			for i, r := range recs {
				if i > 0 {
					c.printf("\n")
				}
				printRecord(c, r)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&n, "number", "n", 20, "show at most this many records (0: all)")
	return cmd
}

// printRecord is one entry of `skd history`.
func printRecord(c *ctx, r app.UpdateRecord) {
	from := short(r.From)
	if from == "" {
		from = "new"
	}
	c.printf("%s  %s  %s → %s\n", dim(r.Time.Local().Format("2006-01-02 15:04")), bold(r.Repo), dim(from), accent(short(r.To)))
	if r.ApplyError != "" {
		c.printf("  %s %s\n", warn(fmt.Sprintf("%-13s", "link failed")), r.ApplyError+dim(" · skd apply finishes it"))
	}
	for _, row := range app.RecordRows(r) {
		c.printf("  %s %s\n", dim(fmt.Sprintf("%-13s", row[0])), row[1])
	}
	switch {
	case r.CommitsUnknown:
		c.printf("  %s\n", dim("upstream commits unknown (history rewritten or not fetched)"))
	case len(r.Commits) > 0:
		for _, cm := range r.Commits {
			c.printf("    %s %s\n", cyan(short(cm.Hash)), cm.Subject)
		}
		if r.MoreCommits {
			c.printf("    %s\n", dim(fmt.Sprintf("… more (git -C %s log %s..%s)", tildePath(c.lib.Root), short(r.From), short(r.To))))
		}
	}
}

func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
