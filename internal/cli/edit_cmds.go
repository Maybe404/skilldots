package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/i18n"
)

func editCmd() *cobra.Command {
	var opts app.EditOptions
	cmd := &cobra.Command{
		Use:   "edit <skill>",
		Short: i18n.T("Open a skill's directory in your editor"),
		Long: i18n.T("The editor is the editor setting (skd config set editor <command>), else $VISUAL, else $EDITOR, else VS Code, " +
			"Cursor, Zed or Sublime Text when installed, else the system's default opener. A vendored skill (or one an " +
			"application ships) is replaced on update, so skd does not open it as is: at a terminal it asks whether to fork " +
			"it first; otherwise it refuses unless --fork (copy it into skills/, or private/ with --private, and open the copy) " +
			"or --force (open the files that will be replaced) is given."),
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			if opts.Private || opts.As != "" {
				opts.Fork = true
			}
			if opts.Fork && opts.Force {
				return i18n.Errorf("--fork and --force: pick one")
			}
			// A terminal editor runs here only at a terminal and without
			// --json; Edit refuses it otherwise, before forking anything.
			foreground := isTerminal() && !flags.json
			opts.NoTerminal = !foreground
			out, err := c.Edit(app.KeyRef(args[0]), opts)
			var over *app.OverwrittenError
			if errors.As(err, &over) && isTerminal() && !flags.json && !flags.yes {
				var how string
				if err := ask(huh.NewSelect[string]().
					Title(over.Entry.LinkName+" "+app.OverwriteNote(over.Entry)).
					Options(
						huh.NewOption(i18n.T("Fork it into skills/ and open your copy (upstream no longer touches it)"), "fork"),
						huh.NewOption(i18n.T("Fork it into private/ (this machine only) and open your copy"), "private"),
						huh.NewOption(i18n.T("Open the files anyway (edits are lost on the next update)"), "force"),
						huh.NewOption(i18n.T("Cancel"), ""),
					).Value(&how)); err != nil || how == "" {
					return nil
				}
				opts = app.EditOptions{Fork: how != "force", Private: how == "private", Force: how == "force", NoTerminal: !foreground}
				out, err = c.Edit(app.KeyRef(args[0]), opts)
			}
			if err != nil {
				return err
			}
			if out.Fork != nil {
				c.printf("%s forked %s → %s\n", ok("✓"), out.Entry.LinkName, out.Fork.Path)
				printApply(c, out.Fork.Apply)
			}
			c.printf("%s %s %s\n", dim(i18n.T("opening")), txt(out.Dir), dim(i18n.Sprintf("with %s (%s)", txt(out.Editor.Name()), i18n.T(out.Editor.From))))
			var runErr error
			if out.Editor.Terminal && foreground {
				ed := out.Editor.Command(out.Dir)
				ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
				runErr = ed.Run()
			} else if out.Editor.Terminal {
				return app.NoTerminalError(out.Editor, out.Dir)
			} else {
				runErr = out.Editor.Start(out.Dir)
			}
			if runErr != nil {
				return fmt.Errorf("%s: %w", out.Editor.Name(), runErr)
			}
			res := map[string]any{"name": out.Entry.LinkName, "path": out.Dir, "editor": out.Editor.Argv, "editorFrom": out.Editor.From, "forked": out.Fork != nil}
			c.jsonOut(res)
			return nil
		},
	}
	cmd.Flags().BoolVar(&opts.Fork, "fork", false, i18n.T("for a vendored or app skill: fork it into skills/ first and open the copy"))
	cmd.Flags().BoolVar(&opts.Private, "private", false, i18n.T("with --fork: into private/ instead of skills/"))
	cmd.Flags().StringVar(&opts.As, "as", "", i18n.T("with --fork: name for the copy"))
	cmd.Flags().BoolVar(&opts.Force, "force", false, i18n.T("open a vendored or app skill's own files although an update replaces them"))
	return cmd
}
