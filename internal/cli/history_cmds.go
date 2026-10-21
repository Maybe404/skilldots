package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/i18n"
)

func historyCmd() *cobra.Command {
	var n int
	var diff int
	var skill string
	cmd := &cobra.Command{
		Use:   "history [pack] [--diff [n]]",
		Short: i18n.T("What each update changed: commits, skills, and upstream's commit messages"),
		Long: i18n.Sprintf("Every `skd update` records, per pack it moved, the old and new commit, the skills that changed, were added, "+
			"moved, went away upstream or appeared there, and the subjects of the upstream commits that touched them (at most "+
			"%d). The record is %s in the library: it is committed and pushed with it, "+
			"so another machine sees it after a clone. Reading it needs no network.\n\n"+
			"With --diff it shows what upstream changed in one recorded update, as a unified diff, skill by skill: the newest "+
			"record (of the pack, when one is named), or the n-th newest (skd history <pack> --diff 2, skd history --diff 2, "+
			"--diff=2), only one skill with --skill. It reads the "+
			"upstream versions the library recorded (refs/skilldots/upstream/*, no network); for a record made before skd kept "+
			"them it reads the pack's cache clone, fetching the commits it lacks.", app.MaxCommits, app.HistoryFile),
		Args: maximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("diff") {
				return historyDiff(c, args, diff, skill)
			}
			if len(args) > 1 {
				return i18n.Errorf("accepts at most %d arg(s), received %d", 1, len(args))
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
	cmd.Flags().IntVar(&diff, "diff", 1, i18n.T("show what upstream changed in a record (the newest, or the n-th newest), as a diff"))
	// --diff alone is the newest record; --diff=2 the one before.
	cmd.Flags().Lookup("diff").NoOptDefVal = "1"
	cmd.Flags().StringVar(&skill, "skill", "", i18n.T("with --diff: only this skill"))
	return cmd
}

// historyDiff is `skd history [pack] --diff [n]`: n is the record's
// number, newest first, given as --diff=n or after the pack. A lone
// number that names no pack is the record's number among every pack's
// (skd history --diff 2).
func historyDiff(c *ctx, args []string, n int, skill string) error {
	p := ""
	number := func(a string) error {
		v, err := strconv.Atoi(a)
		if err != nil || v < 1 {
			return i18n.Errorf("%q is not a record number (1 is the newest)", a)
		}
		n = v
		return nil
	}
	switch len(args) {
	case 2:
		p = args[0]
		if err := number(args[1]); err != nil {
			return err
		}
	case 1:
		if _, err := strconv.Atoi(args[0]); err == nil && c.lib.Manifest.FindPack(args[0]) == nil {
			if err := number(args[0]); err != nil {
				return err
			}
		} else {
			p = args[0]
		}
	}
	if n < 1 {
		return i18n.Errorf("%q is not a record number (1 is the newest)", strconv.Itoa(n))
	}
	r, err := c.HistoryRecord(p, n)
	if err != nil {
		return err
	}
	d, err := c.HistoryDiff(context.Background(), r, skill)
	if err != nil {
		return err
	}
	if c.jsonOut(d) {
		return nil
	}
	printRecord(c, *r)
	if len(d.Skills) == 0 {
		c.printf("\n%s\n", dim(i18n.T("no skill's files changed upstream in this update")))
		return nil
	}
	for _, sd := range d.Skills {
		for _, f := range sd.Files {
			printFileDiff(c, sd.Name, f)
		}
		if sd.MoreFiles > 0 {
			c.printf("%s\n", dim(i18n.Sprintf("… %d more files of %s not shown", sd.MoreFiles, sd.Name)))
		}
	}
	return nil
}

// printFileDiff prints one file's diff as git does, coloured like it.
func printFileDiff(c *ctx, skill string, f app.FileDiff) {
	a, b := "a/"+skill+"/"+f.Path, "b/"+skill+"/"+f.Path
	c.printf("\n%s\n", bold(txt("diff --git "+a+" "+b)))
	switch f.Status {
	case "added":
		c.printf("%s\n", bold(i18n.T("new file")))
		a = "/dev/null"
	case "removed":
		c.printf("%s\n", bold(i18n.T("deleted file")))
		b = "/dev/null"
	}
	switch {
	case f.Binary:
		c.printf("%s\n", i18n.Sprintf("Binary files %s and %s differ", txt(a), txt(b)))
		return
	case f.TooLarge:
		c.printf("%s\n", dim(i18n.T("too large to compare line by line")))
		return
	}
	c.printf("%s\n%s\n", bold(txt("--- "+a)), bold(txt("+++ "+b)))
	for _, h := range f.Hunks {
		c.printf("%s\n", cyan(fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)))
		for _, l := range h.Lines {
			line := l.Op + txt(l.Text)
			switch l.Op {
			case "-":
				line = bad(line)
			case "+":
				line = ok(line)
			}
			c.printf("%s\n", line)
		}
	}
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
		if len(r.Merged) > 0 {
			row["merged"] = r.Merged
		}
		if len(r.Shelved) > 0 {
			row["shelved"] = r.Shelved
		}
		if r.SnapFrom != "" {
			row["snapFrom"] = r.SnapFrom
		}
		if r.SnapTo != "" {
			row["snapTo"] = r.SnapTo
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
