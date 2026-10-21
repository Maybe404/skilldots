package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/i18n"
)

func historyCmd() *cobra.Command {
	var n int
	cmd := &cobra.Command{
		Use:   "history [pack]",
		Short: i18n.T("What each update changed: commits, skills, and upstream's commit messages"),
		Long: i18n.Sprintf("Every `skd update` records, per pack it moved, the old and new commit, the skills that changed, were added, "+
			"moved, went away upstream or appeared there, and the subjects of the upstream commits that touched them (at most "+
			"%d). The record is %s in the library: it is committed and pushed with it, "+
			"so another machine sees it after a clone. Reading it needs no network.", app.MaxCommits, app.HistoryFile),
		Args: maximumNArgs(1),
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
				i18n.Fprintf(stderr, "%s %s: %d line(s) could not be read and were skipped\n", warn("!"), c.HistoryPath(), skipped)
			}
			if c.jsonOut(historyJSON(recs, skipped)) {
				return nil
			}
			if len(recs) == 0 {
				what := i18n.T("no updates recorded yet")
				if p != "" {
					what = i18n.Sprintf("no updates of %s recorded", txt(p))
				}
				c.printf("%s\n", dim(what+i18n.T(" · skd update records each one")))
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
	cmd.Flags().IntVarP(&n, "number", "n", 20, i18n.T("show at most this many records (0: all)"))
	return cmd
}

// historyJSON keeps the CLI contract stable while the desktop bridge exposes
// transient backup details separately.
func historyJSON(recs []app.UpdateRecord, skipped int) map[string]any {
	rows := []map[string]any{}
	for _, r := range recs {
		row := map[string]any{
			"time": r.Time, "repo": r.Repo, "from": r.From, "to": r.To,
			"changed": r.Changed, "added": r.Added, "moved": r.Moved,
			"gone": r.Gone, "dropped": r.Dropped, "new": r.New,
			"commits": r.Commits,
		}
		if r.MoreCommits {
			row["moreCommits"] = true
		}
		if r.CommitsUnknown {
			row["commitsUnknown"] = true
		}
		if r.ApplyError != "" {
			row["applyError"] = r.ApplyError
		}
		if r.Omitted != 0 {
			row["omitted"] = r.Omitted
		}
		rows = append(rows, row)
	}
	return map[string]any{"records": rows, "skipped": skipped}
}

// printRecord is one entry of `skd history`.
func printRecord(c *ctx, r app.UpdateRecord) {
	from := short(r.From)
	if from == "" {
		from = i18n.T("new")
	}
	c.printf("%s  %s  %s → %s\n", dim(r.Time.Local().Format("2006-01-02 15:04")), bold(r.Repo), dim(from), accent(short(r.To)))
	if r.ApplyError != "" {
		c.printf("  %s %s\n", warn(pad(i18n.T("link failed"), 13)), r.ApplyError+dim(i18n.T(" · skd apply finishes it")))
	}
	for _, row := range app.RecordRows(r) {
		c.printf("  %s %s\n", dim(pad(row[0], 13)), row[1])
	}
	switch {
	case r.CommitsUnknown:
		c.printf("  %s\n", dim(i18n.T("upstream commits unknown (history rewritten or not fetched)")))
	case len(r.Commits) > 0:
		for _, cm := range r.Commits {
			c.printf("    %s %s\n", cyan(short(cm.Hash)), cm.Subject)
		}
		if r.MoreCommits {
			c.printf("    %s\n", dim(i18n.Sprintf("… more (git -C %s log %s..%s)", tildePath(c.lib.Root), short(r.From), short(r.To))))
		}
	}
}

// tildeIn writes every path in the home directory that s holds as ~/…
// (a line such as "<path> → <path>").
func tildeIn(s string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == string(filepath.Separator) {
		return s
	}
	home = strings.TrimSuffix(home, string(filepath.Separator))
	return strings.ReplaceAll(s, home+string(filepath.Separator), "~"+string(filepath.Separator))
}

// tildePath writes a path in the home directory as ~/…, as `skd list`
// shows the library.
func tildePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == string(filepath.Separator) {
		return p
	}
	home = strings.TrimSuffix(home, string(filepath.Separator))
	if p == home || strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
