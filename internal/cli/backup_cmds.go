package cli

import (
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
)

func backupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "backup <dir>",
		Short: i18n.T("Save the whole library, with its history, as a zip file in dir"),
		Long: i18n.T("backup writes skilldots-backup-<time>.zip into dir: skills.toml, skills.lock, the update history, " +
			"vendor/, skills/, private/ and the git history of the library and of private/, with a checksum of every file. " +
			"The library trash and the skilldots skill that ships with the app are left out. skd restore <zip> puts it back " +
			"on this or another machine; no git remote is needed."),
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			res, err := c.BackupZip(args[0], Version)
			if err != nil {
				return err
			}
			say("%s %s\n  %s\n", ok("✓"), i18n.Sprintf("backed up %s to %s", tildePath(c.lib.Root), res.Path),
				dim(i18n.Sprintf("%s, %d files; restore with: skd restore %s", plural(len(res.Skills), "skill"), res.Files, res.Path)))
			jsonOut(res)
			return nil
		},
	}
}

func restoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <backup.zip> [dir]",
		Short: i18n.T("Restore a library from a zip backup (default into ~/.skilldots) and apply it"),
		Long: i18n.T("restore checks every file of a backup made by skd backup against its checksums, unpacks the library " +
			"into dir (default ~/.skilldots), makes it the current library and links its skills into the agents. dir must " +
			"not exist or be empty: restore never writes over a library. To restore over one, move it to the Trash first."),
		Args: rangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !strings.HasSuffix(strings.ToLower(args[0]), ".zip") {
				return i18n.Errorf("%s is not a .zip backup (skd trash restore brings back something from the trash)", args[0])
			}
			dir := library.DefaultRoot()
			if len(args) == 2 {
				dir = args[1]
			}
			dir, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			app.SealOlderSkd()
			res, err := app.RestoreBackup(args[0], dir)
			if err != nil {
				return err
			}
			say("%s %s\n", ok("✓"), i18n.Sprintf("restored %s into %s", plural(len(res.Info.Skills), "skill"), dir))
			for _, s := range res.Skipped {
				say("  %s %s\n", warn("!"), i18n.Sprintf("%s: a link outside the library, not restored", s))
			}
			flags.library = dir
			c, err := open()
			if err != nil {
				return err
			}
			approved, err := reviewCustomTargets(c.reg)
			if err != nil {
				return err
			}
			if approved {
				if c, err = open(); err != nil {
					return err
				}
			}
			out, err := c.Apply(app.ApplyOptions{})
			if out != nil {
				printApply(c, out)
			}
			if err != nil {
				return err
			}
			jsonOut(map[string]any{"library": dir, "backup": res.Info, "skipped": res.Skipped, "apply": out})
			return nil
		},
	}
}
