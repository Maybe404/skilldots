package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// runMenu is what `skd` does with no arguments in a terminal: a guided
// front door for people who do not remember the subcommands.
func runMenu() error {
	for {
		root, err := library.Locate(flags.library)
		if err != nil {
			if done, err := setupMenu(); err != nil || !done {
				return err
			}
			continue
		}
		c, err := open()
		if err != nil {
			return err
		}
		entries, _ := c.resolve()
		linked := 0
		for _, e := range entries {
			if len(e.Targets) > 0 {
				linked++
			}
		}
		var choice string
		err = huh.NewSelect[string]().
			Title("skilldots").
			Description(fmt.Sprintf("%s\n%s · %s · %d linked", root,
				plural(len(c.lib.Manifest.Packs), "pack"), plural(len(entries), "skill"), linked)).
			Options(
				huh.NewOption("Overview            show packs and skills", "list"),
				huh.NewOption("Add                 subscribe to a GitHub repository", "add"),
				huh.NewOption("Update              fetch upstream changes", "update"),
				huh.NewOption("Agents              choose which agents get a skill", "targets"),
				huh.NewOption("Fork                make a vendored skill your own", "fork"),
				huh.NewOption("Remove              drop a skill or a whole pack", "remove"),
				huh.NewOption("New skill           scaffold one of your own", "new"),
				huh.NewOption("Apply               re-link everything to match the manifest", "apply"),
				huh.NewOption("Backup              commit and push the library", "sync"),
				huh.NewOption("Quit", "quit"),
			).
			Value(&choice).Run()
		if err != nil || choice == "quit" {
			return nil
		}
		if err := menuAction(c, choice, entries); err != nil {
			fmt.Fprintln(os.Stderr, bad("error:"), err)
		}
		fmt.Println()
	}
}

func menuAction(c *ctx, choice string, entries []engine.Entry) error {
	run := func(args ...string) error {
		cmd := Root()
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	switch choice {
	case "list":
		return run("list", "--all")
	case "update":
		return run("update")
	case "apply":
		return run("apply")
	case "add":
		var src string
		if err := huh.NewInput().Title("Repository").
			Description("owner/repo, a GitHub URL, or any git URL; add @ref to pin").
			Placeholder("mattpocock/skills").Value(&src).Run(); err != nil || strings.TrimSpace(src) == "" {
			return nil
		}
		return run("add", strings.TrimSpace(src))
	case "new":
		var name string
		var private bool
		if err := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Skill name").Placeholder("my-skill").Value(&name),
			huh.NewConfirm().Title("Keep it private to this machine?").Value(&private),
		)).Run(); err != nil || strings.TrimSpace(name) == "" {
			return nil
		}
		args := []string{"new", strings.TrimSpace(name)}
		if private {
			args = append(args, "--private")
		}
		return run(args...)
	case "fork":
		var vendored []engine.Entry
		for _, e := range entries {
			if e.Kind == engine.KindVendor || e.Kind == engine.KindApp {
				vendored = append(vendored, e)
			}
		}
		name, ok := pickEntry("Fork which skill?", vendored)
		if !ok {
			return nil
		}
		return run("fork", name)
	case "remove":
		name, ok := pickEntry("Remove which skill?", entries)
		if !ok {
			return nil
		}
		e := engine.Find(entries, name)
		var how string
		opts := []huh.Option[string]{
			huh.NewOption("Unlink from all agents, keep in library  (skd disable)", "disable"),
		}
		if e.Kind == engine.KindVendor {
			opts = append(opts,
				huh.NewOption("Unlink and drop from the library           (skd remove)", "remove"),
				huh.NewOption("Delete the whole pack "+e.Origin+"   (skd purge)", "purge"))
		}
		opts = append(opts, huh.NewOption("Cancel", ""))
		if err := huh.NewSelect[string]().Title(name).Options(opts...).Value(&how).Run(); err != nil || how == "" {
			return nil
		}
		if how == "purge" {
			return run("purge", e.Origin)
		}
		return run(how, name)
	case "targets":
		name, ok := pickEntry("Which skill?", entries)
		if !ok {
			return nil
		}
		e := engine.Find(entries, name)
		var opts []huh.Option[string]
		for _, t := range c.reg.Present() {
			opts = append(opts, huh.NewOption(fmt.Sprintf("%-12s %s", t.Name, dim(t.Path)), t.Name))
		}
		chosen := append([]string(nil), e.TargetNames...)
		if err := huh.NewMultiSelect[string]().
			Title("Agents for " + name).
			Description("only agents installed on this machine are listed · none = keep in library, link nowhere").
			Options(opts...).Value(&chosen).Run(); err != nil {
			return nil
		}
		return setTargets(c, name, func([]string) []string {
			if chosen == nil {
				return []string{}
			}
			return chosen
		})
	case "sync":
		return run("sync")
	}
	return nil
}

func pickEntry(title string, entries []engine.Entry) (string, bool) {
	if len(entries) == 0 {
		fmt.Println(dim("nothing to choose from"))
		return "", false
	}
	var opts []huh.Option[string]
	for _, e := range entries {
		label := fmt.Sprintf("%-28s %s", e.LinkName, dim(engine.Describe(e)))
		opts = append(opts, huh.NewOption(label, e.LinkName))
	}
	var name string
	if err := huh.NewSelect[string]().Title(title).Options(opts...).Filtering(true).Height(16).Value(&name).Run(); err != nil {
		return "", false
	}
	return name, name != ""
}

// setupMenu runs when no library exists. It returns done=true once one does.
func setupMenu() (bool, error) {
	var how string
	if err := huh.NewSelect[string]().
		Title("No skilldots library yet").
		Description("A library is a git repository holding your skills and a skills.toml manifest.").
		Options(
			huh.NewOption("Create a new library at "+library.DefaultRoot(), "new"),
			huh.NewOption("Clone an existing library from git (e.g. your backup on GitHub)", "clone"),
			huh.NewOption("Use a directory that already has a skills.toml", "existing"),
			huh.NewOption("Quit", ""),
		).Value(&how).Run(); err != nil || how == "" {
		return false, nil
	}
	cmd := Root()
	switch how {
	case "new":
		cmd.SetArgs([]string{"init"})
	case "clone":
		var url string
		if err := huh.NewInput().Title("Git URL").Placeholder("git@github.com:you/agent-skills.git").Value(&url).Run(); err != nil || url == "" {
			return false, nil
		}
		cmd.SetArgs([]string{"clone", strings.TrimSpace(url)})
	case "existing":
		var dir string
		if err := huh.NewInput().Title("Directory").Placeholder("~/code/agent-skills").Value(&dir).Run(); err != nil || dir == "" {
			return false, nil
		}
		home, _ := os.UserHomeDir()
		dir = strings.Replace(strings.TrimSpace(dir), "~", home, 1)
		if !fsutil.Exists(filepath.Join(dir, manifest.FileName)) {
			return false, fmt.Errorf("%s has no %s", dir, manifest.FileName)
		}
		if err := library.Remember(dir); err != nil {
			return false, err
		}
		fmt.Printf("%s skd now uses %s\n", ok("✓"), dir)
		return true, nil
	}
	if err := cmd.Execute(); err != nil {
		return false, err
	}
	return true, nil
}

func cloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone <git-url> [dir]",
		Short: "Clone a library (your backup from another machine) and apply it",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := library.DefaultRoot()
			if len(args) == 2 {
				dir = args[1]
			}
			if fsutil.Lexists(dir) {
				return fmt.Errorf("%s already exists", dir)
			}
			if _, err := library.Git("", "clone", "-q", args[0], dir); err != nil {
				return err
			}
			if err := library.EnsureGit(filepath.Join(dir, library.DirPrivate)); err != nil {
				return err
			}
			fmt.Printf("%s cloned into %s\n", ok("✓"), dir)
			if err := library.Remember(dir); err != nil {
				return err
			}
			flags.library = dir
			c, err := open()
			if err != nil {
				return err
			}
			if _, err := c.lib.Git("status", "--short"); err != nil {
				return err
			}
			// Vendored skills come with the clone; only links need creating.
			return runApply(c, false, false)
		},
	}
}

func syncCmd() *cobra.Command {
	var remote, msg string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Commit the library and push it to its git remote (your backup)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			if remote != "" {
				if _, err := c.lib.Git("remote", "get-url", "origin"); err == nil {
					if _, err := c.lib.Git("remote", "set-url", "origin", remote); err != nil {
						return err
					}
				} else if _, err := c.lib.Git("remote", "add", "origin", remote); err != nil {
					return err
				}
			}
			if msg == "" {
				msg = "chore: update skills library"
			}
			if _, err := c.lib.Git("add", "-A"); err != nil {
				return err
			}
			if _, err := c.lib.Git("diff", "--cached", "--quiet"); err != nil {
				if _, err := c.lib.Git("commit", "-q", "-m", msg); err != nil {
					return err
				}
				fmt.Printf("%s committed\n", ok("✓"))
			} else {
				fmt.Println(dim("nothing to commit"))
			}
			if _, err := c.lib.Git("remote", "get-url", "origin"); err != nil {
				fmt.Printf("%s no remote configured; add one with: skd sync --remote <git-url>\n", warn("!"))
				return nil
			}
			if _, err := c.lib.Git("rev-parse", "--abbrev-ref", "@{u}"); err != nil {
				if _, err := c.lib.Git("push", "-q", "-u", "origin", "HEAD"); err != nil {
					return err
				}
			} else if _, err := c.lib.Git("push", "-q"); err != nil {
				return err
			}
			fmt.Printf("%s pushed\n", ok("✓"))
			if fsutil.Exists(filepath.Join(c.lib.PrivateDir(), ".git")) {
				if out, _ := library.Git(c.lib.PrivateDir(), "status", "--short"); out != "" {
					fmt.Printf("%s private/ has uncommitted changes (it is a separate repo and is never pushed here)\n", dim("note:"))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&remote, "remote", "", "set the git remote (origin) before pushing")
	cmd.Flags().StringVarP(&msg, "message", "m", "", "commit message")
	return cmd
}

var errNoTTY = errors.New("no terminal; run `skd --help` for commands")
