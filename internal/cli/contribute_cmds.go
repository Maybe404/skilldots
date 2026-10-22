package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/i18n"
)

func diffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <skill>",
		Short: i18n.T("Show your changes to a vendored skill, against the upstream version it is based on"),
		Long: i18n.T("Compares a vendored skill's files with the upstream version they are based on (the one skills.lock records, " +
			"found in the library's upstream snapshots or its git history), as a unified diff: what you changed, which " +
			"skd update keeps by merging upstream's changes in, and what skd contribute would send. When that version is " +
			"not found it says so and shows nothing."),
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			d, err := c.LocalChanges(context.Background(), app.KeyRef(args[0]))
			if err != nil {
				return err
			}
			if c.jsonOut(d) {
				return nil
			}
			switch {
			case !d.Local:
				c.printf("%s\n", dim(i18n.Sprintf("%s has no changes of yours: it is what skd vendored", d.Name)))
			case !d.Base:
				c.printf("%s\n", warn(i18n.Sprintf("%s has changes of yours, but the upstream version they are based on is not known (no upstream snapshot, never committed in the library), so skd cannot show them", d.Name)))
			default:
				for _, f := range d.Files {
					printFileDiff(c, d.Name, f)
				}
				if d.MoreFiles > 0 {
					c.printf("%s\n", dim(i18n.Sprintf("… %d more files of %s not shown", d.MoreFiles, d.Name)))
				}
			}
			return nil
		},
	}
}

func contributeCmd() *cobra.Command {
	var pr, push, dry, refresh bool
	var msg string
	cmd := &cobra.Command{
		Use:   "contribute [skill | pack] [--pr | --push]",
		Short: i18n.T("Send your changes to a vendored skill upstream, as a pull request or a push"),
		Long: i18n.T("Your changes to a vendored skill stay yours (skd update merges upstream's changes into them); nothing is ever " +
			"sent upstream unless you run this. contribute <skill> --pr opens a pull request on the pack's GitHub repository " +
			"from a branch skilldots/<skill>-<time> (pushed to the repository when you may push there, else to your fork, " +
			"which gh makes); --push pushes the change straight to the pack's branch, when you may. The change is committed, " +
			"with your git identity and -m as the message, in a clone of its own in the cache, never in the library. The " +
			"skill must be based on upstream's current version (skd update first), and --pr needs gh, logged in. " +
			"contribute <pack> sends every skill of the pack you changed, and every skill you made in it (skd new --in), in " +
			"one commit. For your own repository (skd own) it pulls first when the repository moved on, merging as skd update " +
			"does, and pulls back after a push, so nothing is left marked as changed. --dry-run shows the repository, the " +
			"branch, what is possible and the diff, and sends nothing; without a terminal --yes is needed to send. Without a " +
			"skill, contribute lists the contributions on record; --refresh asks GitHub for the state of the pull requests."),
		Args: maximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			ctx := context.Background()
			if len(args) == 0 {
				if refresh {
					if err := c.RefreshContributions(ctx); err != nil {
						return err
					}
				}
				list := c.Contributions()
				if c.jsonOut(map[string]any{"contributions": list}) {
					return nil
				}
				if len(list) == 0 {
					c.printf("%s\n", dim(i18n.T("no contributions on record · skd contribute <skill> --pr sends one")))
					return nil
				}
				for _, it := range list {
					what := it.Branch
					if it.Kind == app.ModePR {
						what = fmt.Sprintf("PR #%d %s", it.Number, it.URL)
					}
					c.printf("%s  %s  %s  %s\n", bold(txt(it.Name)), dim(txt(it.Repo)), accent(contributionState(it.State)), txt(what))
				}
				return nil
			}
			if pr && push {
				return i18n.Errorf("--pr and --push: pick one")
			}
			whole, err := contributesPack(c, args[0])
			if err != nil {
				return err
			}
			var o *app.ContributeOptions
			if whole != "" {
				o, err = c.ContributePackOptions(ctx, whole)
			} else {
				o, err = c.ContributeOptions(ctx, app.KeyRef(args[0]))
			}
			if err != nil {
				return err
			}
			mode := ""
			switch {
			case pr:
				mode = app.ModePR
			case push:
				mode = app.ModePush
			}
			if dry || mode == "" {
				if c.jsonOut(o) {
					return nil
				}
				c.printf("%s  %s %s  %s\n", bold(txt(args[0])), dim(i18n.T("to")), txt(o.Repo), dim(i18n.Sprintf("branch %s", txt(o.Branch))))
				if whole != "" || len(o.Drafts) > 0 {
					for _, n := range o.Skills {
						if p, isNew := o.Paths[n]; isNew {
							c.printf("  %s %s\n", ok("+"), i18n.Sprintf("%s  new, at %s", txt(n), txt(p)))
						} else {
							c.printf("  %s %s\n", accent("✎"), txt(n))
						}
					}
				}
				c.printf("  %s %s · %s %s · gh %s\n", dim(i18n.T("push")), yesNo(o.CanPush), dim(i18n.T("pull request")), yesNo(o.CanPR), o.GH)
				if o.Pull {
					c.printf("  %s\n", accent(i18n.Sprintf("%s moved on: it is pulled first, merging as skd update does", o.Repo)))
				}
				if o.Reason != "" {
					c.printf("  %s\n", warn(o.Reason))
				}
				for _, n := range o.Skills {
					if d, err := c.LocalChanges(ctx, app.KeyRef(qualified(c, o.Repo, n))); err == nil && d.Base {
						for _, f := range d.Files {
							printFileDiff(c, d.Name, f)
						}
					}
				}
				if mode == "" {
					if o.Own {
						c.printf("%s\n", dim(i18n.Sprintf("skd contribute %s --push pushes to %s · --pr opens a pull request", args[0], o.Branch)))
					} else {
						c.printf("%s\n", dim(i18n.Sprintf("skd contribute %s --pr opens a pull request · --push pushes to %s", args[0], o.Branch)))
					}
				}
				return nil
			}
			target := i18n.Sprintf("open a pull request on %s", o.Repo)
			if mode == app.ModePush {
				target = i18n.Sprintf("push to %s's %s", o.Repo, o.Branch)
			}
			if !flags.yes {
				if !isTerminal() || flags.json {
					return i18n.Errorf("contribute sends your changes upstream (%s); run it with --dry-run to see what it sends, then with --yes", target)
				}
				q := i18n.Sprintf("Send %s's changes: %s?", args[0], target)
				if whole != "" {
					q = i18n.Sprintf("Send %s (%s): %s?", plural(len(o.Skills), "skill"), strings.Join(o.Skills, ", "), target)
				}
				if !confirm(q) {
					c.printf("%s\n", dim(i18n.T("nothing sent")))
					return nil
				}
			}
			var out *app.ContributeOutcome
			if whole != "" {
				out, err = c.ContributePack(ctx, whole, mode, msg)
			} else {
				out, err = c.Contribute(ctx, app.KeyRef(args[0]), mode, msg)
			}
			if out != nil {
				if out.Pulled != nil {
					c.printf("%s %s\n", ok("✓"), i18n.Sprintf("pulled %s first", out.Repo))
					printReport(c, out.Pulled)
				}
				c.jsonOut(out)
			}
			if err != nil {
				return err
			}
			if out.URL != "" {
				c.printf("%s %s\n", ok("✓"), i18n.Sprintf("pull request #%d opened: %s", out.Number, out.URL))
			} else {
				c.printf("%s %s\n", ok("✓"), i18n.Sprintf("pushed to %s's %s", out.Repo, out.Branch))
			}
			if out.PulledBack != nil {
				c.printf("%s %s\n", ok("✓"), i18n.Sprintf("pulled it back: %s now match %s", strings.Join(out.Skills, ", "), out.Repo))
			}
			if out.PullBackError != "" {
				c.printf("%s %s\n", warn("!"), i18n.Sprintf("pulling it back failed (%s): skd update %s finishes it", txt(out.PullBackError), out.Repo))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&pr, "pr", false, i18n.T("open a pull request (GitHub, through gh)"))
	cmd.Flags().BoolVar(&push, "push", false, i18n.T("push to the pack's branch (when you may)"))
	cmd.Flags().StringVarP(&msg, "message", "m", "", i18n.T("commit message (its first line titles the pull request)"))
	cmd.Flags().BoolVar(&dry, "dry-run", false, i18n.T("show where it would go and the diff; send nothing"))
	cmd.Flags().BoolVar(&refresh, "refresh", false, i18n.T("ask GitHub for the state of the pull requests on record"))
	return cmd
}

// contributesPack tells whether what contribute was given names a pack
// (its repository, then) rather than a skill (""): a name both could be
// is refused, with the two spellings that tell them apart.
func contributesPack(c *ctx, arg string) (string, error) {
	p := c.lib.Manifest.FindPack(arg)
	if p == nil {
		return "", nil
	}
	entries, err := c.Resolve()
	if err != nil {
		return "", err
	}
	if e, err := c.Lookup(entries, arg); err == nil && e != nil && arg != p.Repo {
		return "", i18n.Errorf("%q is both the pack %s and the skill %s: say skd contribute %s for the pack, or skd contribute %s for the skill", arg, p.Repo, e.LinkName, p.Repo, engine.Qualified(*e))
	}
	return p.Repo, nil
}

// qualified is the key of a pack's skill by its link name.
func qualified(c *ctx, repo, link string) string {
	if entries, err := c.Resolve(); err == nil {
		for _, e := range entries {
			if e.Kind == engine.KindVendor && e.Origin == repo && e.LinkName == link {
				return engine.Qualified(e)
			}
		}
	}
	return link
}

func yesNo(b bool) string {
	if b {
		return ok(i18n.T("yes"))
	}
	return warn(i18n.T("no"))
}

// contributionState is a contribution's state for a person.
func contributionState(s string) string {
	switch s {
	case "open":
		return i18n.T("open")
	case "merged":
		return i18n.T("merged")
	case "closed":
		return i18n.T("closed")
	case "pushed":
		return i18n.T("pushed")
	}
	return strings.TrimSpace(s)
}
