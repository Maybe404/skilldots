package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/i18n"
)

func resolveCmd() *cobra.Command {
	var cont, abort bool
	cmd := &cobra.Command{
		Use:   "resolve [skill]",
		Short: i18n.T("Resolve the conflicts of a shelved update, as you would finish a git merge"),
		Long: i18n.T("An update merges upstream's changes into a vendored skill you changed. When both sides changed the same lines " +
			"(or one removed a file the other changed) the update of that skill is shelved: the skill stays as it is until " +
			"its conflicts are resolved. Without arguments, resolve lists the shelved skills. resolve <skill> writes the merge " +
			"into the skill's files with git's conflict markers (<<<<<<< local, =======, >>>>>>> upstream) after setting the " +
			"skill aside in the trash, and marks the files unmerged in the library's git index, so git status, git diff and " +
			"git checkout --ours/--theirs <file> work as in a git merge. A file removed on one side, or not text, gets the " +
			"upstream version beside it as <file>.upstream (or an empty <file>.upstream-deleted where upstream removed it): " +
			"keep one and delete the other. Edit the files, then --continue takes the result (it refuses while a marker or " +
			"such a file is left); --abort puts the skill back as it was, still shelved. Until then agents linked to the skill " +
			"read the files with their markers."),
		Args: maximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			if cont && abort {
				return i18n.Errorf("--continue and --abort: pick one")
			}
			ref := app.Ref{}
			if len(args) == 1 {
				ref = app.KeyRef(args[0])
			}
			switch {
			case cont:
				out, err := c.ResolveContinue(ref)
				var ue *app.UnresolvedError
				if errors.As(err, &ue) {
					c.jsonOut(map[string]any{"skill": ue.Skill, "unresolved": ue.Files})
					for _, f := range ue.Files {
						c.printf("  %s %s\n", warn("✗"), txt(f))
					}
					return err
				}
				if err != nil {
					return err
				}
				c.printf("%s %s\n", ok("✓"), i18n.Sprintf("%s merged with upstream; the files are staged (git add), skd sync commits them", out.Name))
				if out.Apply != nil {
					printApply(c, out.Apply)
				}
				c.jsonOut(out)
				return nil
			case abort:
				out, err := c.ResolveAbort(ref)
				if err != nil {
					return err
				}
				c.printf("%s %s\n", ok("✓"), i18n.Sprintf("%s is back as it was; its update stays shelved (skd resolve %s starts again)", out.Name, out.Name))
				if out.Trash != "" {
					c.printf("  %s\n", dim(i18n.Sprintf("the files as they were are in the trash as %s", out.Trash)))
				}
				c.jsonOut(out)
				return nil
			case len(args) == 0:
				list, err := c.Shelved()
				if err != nil {
					return err
				}
				if c.jsonOut(map[string]any{"skills": list}) {
					return nil
				}
				if len(list) == 0 {
					c.printf("%s\n", dim(i18n.T("no update is shelved")))
					return nil
				}
				for _, sh := range list {
					state := warn(i18n.T("shelved"))
					next := i18n.Sprintf("skd resolve %s", sh.LinkName)
					if sh.Resolving {
						state = accent(i18n.T("resolving"))
						next = i18n.Sprintf("skd resolve --continue %s · --abort", sh.LinkName)
					}
					c.printf("%s  %s  %s  %s\n", bold(txt(sh.LinkName)), dim(txt(sh.Repo)+" @ "+short(sh.Upstream)), state, dim(next))
					for _, f := range sh.Files {
						c.printf("  %s %s\n", warn("✗"), txt(f))
					}
				}
				return nil
			}
			out, err := c.ResolveStart(ref)
			if err != nil {
				return err
			}
			if c.jsonOut(out) {
				return nil
			}
			if out.Merged {
				c.printf("%s %s\n", ok("✓"), i18n.Sprintf("%s merged with upstream without conflicts (the copy before is in the trash as %s)", out.Name, out.Backup))
				if out.Apply != nil {
					printApply(c, out.Apply)
				}
				return nil
			}
			c.printf("%s %s\n", warn("!"), i18n.Sprintf("%s: %s with conflicts in %s", out.Name, plural(len(out.Conflicts), "file"), tildePath(out.Dir)))
			for _, f := range out.Conflicts {
				c.printf("  %s %s  %s\n", warn("✗"), txt(f.Path), dim(conflictHint(f)))
			}
			c.printf("%s\n", dim(i18n.Sprintf("edit them (git checkout --ours/--theirs <file> takes one side), then skd resolve --continue %s · skd resolve --abort %s puts it back", out.Name, out.Name)))
			c.printf("%s\n", dim(i18n.Sprintf("the copy before is in the trash as %s", out.Backup)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&cont, "continue", false, i18n.T("finish: take the files as they are once no conflict is left"))
	cmd.Flags().BoolVar(&abort, "abort", false, i18n.T("give up: put the skill back as it was, its update still shelved"))
	return cmd
}

// conflictHint says what to do with one conflicted file.
func conflictHint(f engine.Conflict) string {
	switch f.Kind {
	case engine.ConflictBinary:
		return i18n.Sprintf("not text: keep yours, or replace it with %s; then delete %s", f.Path+engine.UpstreamSuffix, f.Path+engine.UpstreamSuffix)
	case engine.ConflictDeletedLocal:
		return i18n.Sprintf("you removed it, upstream changed it: rename %s to %s to keep upstream's, or delete it", f.Path+engine.UpstreamSuffix, f.Path)
	case engine.ConflictDeletedUpstream:
		return i18n.Sprintf("upstream removed it, you changed it: delete %s to keep yours, or delete both", f.Path+engine.DeletedSuffix)
	}
	return i18n.T("conflict markers")
}

// printShelved is what update says about the skills it shelved.
func printShelved(c *ctx, r *engine.SyncReport) {
	for _, s := range r.Shelved {
		c.printf("  %s %s  %s\n", warn(pad(i18n.T("shelved"), 13)), txt(s.Name)+dim(" ("+strings.Join(txts(s.Files), ", ")+")"),
			dim(i18n.Sprintf("conflicts with your changes · skd resolve %s", txt(s.Name))))
	}
}
