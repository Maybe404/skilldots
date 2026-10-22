package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/safetext"
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
		home, _ := os.UserHomeDir()
		status := ok("●") + " " + dim(i18n.T("everything linked"))
		if n := c.Pending(entries); n > 0 {
			status = warn("○") + " " + dim(i18n.Sprintf("%d changes pending — Apply", n))
		}
		last := ""
		if out, err := c.lib.Git("log", "-1", "--format=%cr"); err == nil && out != "" {
			last = dim(i18n.Sprintf("  ·  last change %s", out))
		}
		lines := []string{brand(), dim(strings.Replace(root, home, "~", 1)), "",
			cyan(plural(len(c.lib.Manifest.Packs), "pack")) + dim(" · ") + cyan(plural(len(entries), "skill")) + dim(" · ") + cyan(i18n.Sprintf("%d in agents", linked)) + dim(i18n.Sprintf(" · agents: %s", strings.Join(c.lib.Manifest.Targets.Default, ", ")))+pausedNote(c.lib.Manifest.Targets.Disabled),
			status + last}
		rec := c.RecordedOutdated()
		if line := updatesLine(rec, time.Now()); line != "" {
			lines = append(lines, line)
		}
		updateDesc := i18n.T("fetch upstream changes")
		if rec != nil {
			switch n := rec.Updates(); {
			case n == 1:
				updateDesc = i18n.T("1 pack has updates")
			case n > 1:
				updateDesc = i18n.Sprintf("%d packs have updates", n)
			}
		}
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, card(lines...))
		fmt.Fprintln(stdout)
		var choice string
		err = ask(huh.NewSelect[string]().
			Options(
				opt("Overview", "packs and skills at a glance", "list"),
				opt("Add", "subscribe to a repository and pick skills", "add"),
				opt("Update", updateDesc, "update"),
				opt("Outdated", "check upstream without changing anything", "outdated"),
				opt("Agents", "choose which agents get a skill", "targets"),
				opt("Fork", "make a vendored skill your own", "fork"),
				opt("Remove", "unlink, drop, or purge", "remove"),
				opt("New skill", "scaffold one of your own", "new"),
				opt("Scan", "find skills installed by other tools", "scan"),
				opt("Trash", "what skd set aside", "trash"),
				opt("Apply", "re-link to match the manifest", "apply"),
				opt("Backup", "commit and push the library", "sync"),
				opt("Quit", "", "quit"),
			).
			Value(&choice))
		if err != nil || choice == "quit" {
			fmt.Fprintln(stdout)
			return nil
		}
		if err := menuAction(c, choice, entries); err != nil {
			fmt.Fprintln(stderr, tag(bad, i18n.T("error:"), fmt.Sprint(err)))
		}
		fmt.Fprintln(stdout)
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
	case "outdated":
		return run("outdated")
	case "apply":
		return run("apply")
	case "add":
		var src string
		if err := ask(huh.NewInput().Title(i18n.T("Source")).
			Description(i18n.T("owner/repo · a GitHub or skills.sh link · a pasted `npx skills add …` line · any git URL · @ref pins")).
			Placeholder(i18n.T("mattpocock/skills")).Value(&src)); err != nil || strings.TrimSpace(src) == "" {
			return nil
		}
		return run("add", strings.TrimSpace(src))
	case "new":
		var name string
		var private bool
		if err := ask(
			huh.NewInput().Title(i18n.T("Skill name")).Placeholder(i18n.T("my-skill")).Value(&name),
			huh.NewConfirm().Title(i18n.T("Keep it private to this machine?")).Value(&private),
		); err != nil || strings.TrimSpace(name) == "" {
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
		e, _ := engine.Lookup(entries, name)
		if e == nil {
			return nil
		}
		var how string
		opts := []huh.Option[string]{
			huh.NewOption(i18n.T("Unlink from all agents, keep in library  (skd disable)"), "disable"),
		}
		if e.Kind == engine.KindVendor {
			opts = append(opts,
				huh.NewOption(i18n.T("Unlink and drop from the library           (skd remove)"), "remove"),
				huh.NewOption(i18n.Sprintf("Delete the whole pack %s   (skd purge)", e.Origin), "purge"))
		}
		opts = append(opts, huh.NewOption(i18n.T("Cancel"), ""))
		if err := ask(huh.NewSelect[string]().Title(name).Options(opts...).Value(&how)); err != nil || how == "" {
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
		e, _ := engine.Lookup(entries, name)
		if e == nil {
			return nil
		}
		var opts []huh.Option[string]
		for _, t := range c.reg.Present() {
			opts = append(opts, huh.NewOption(fmt.Sprintf("%-12s %s", t.Name, dim(t.Path)), t.Name))
		}
		chosen := append([]string(nil), e.TargetNames...)
		if err := ask(huh.NewMultiSelect[string]().
			Title(i18n.Sprintf("Agents for %s", name)).
			Description(i18n.T("only agents installed on this machine are listed · none = keep in library, link nowhere")).
			Options(opts...).Value(&chosen)); err != nil {
			return nil
		}
		return targetsDone(c, chosen, true)(c.SetTargets(app.KeyRef(name), chosen))
	case "sync":
		return run("sync")
	case "scan":
		if err := run("scan"); err != nil {
			return err
		}
		var adopt bool
		if err := ask(huh.NewConfirm().Title(i18n.T("Adopt unmanaged entries now?")).Affirmative(i18n.T("Adopt")).Negative(i18n.T("Not now")).Value(&adopt)); err != nil || !adopt {
			return nil
		}
		return run("adopt")
	case "trash":
		return run("trash")
	}
	return nil
}

// opt builds a menu option with a dimmed description.
func opt(label, desc, value string) huh.Option[string] {
	label, desc = i18n.T(label), i18n.T(desc)
	if desc == "" {
		return huh.NewOption(label, value)
	}
	return huh.NewOption(pad(label, 12)+dim(desc), value)
}

func pickEntry(title string, entries []engine.Entry) (string, bool) {
	title = i18n.T(title)
	if len(entries) == 0 {
		fmt.Fprintln(stdout, dim(i18n.T("nothing to choose from")))
		return "", false
	}
	var opts []huh.Option[string]
	for _, e := range entries {
		label := fmt.Sprintf("%-28s %s", e.LinkName, dim(engine.Describe(e)))
		opts = append(opts, huh.NewOption(label, engine.Qualified(e)))
	}
	var name string
	if err := ask(huh.NewSelect[string]().Title(title).Options(opts...).Filtering(true).Height(16).Value(&name)); err != nil {
		return "", false
	}
	return name, name != ""
}

// setupMenu runs when no library exists. It returns done=true once one does.
func setupMenu() (bool, error) {
	var how string
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, card(brand(), dim(i18n.T("no library on this machine yet"))))
	fmt.Fprintln(stdout)
	if err := ask(huh.NewSelect[string]().
		Title(i18n.T("Where should your skills live?")).
		Description(i18n.T("A library is a git repository holding your skills and a skills.toml manifest.")).
		Options(
			huh.NewOption(i18n.Sprintf("Create a new library at %s", library.DefaultRoot()), "new"),
			huh.NewOption(i18n.T("Clone an existing library from git (e.g. your backup on GitHub)"), "clone"),
			huh.NewOption(i18n.T("Use a directory that already has a skills.toml"), "existing"),
			huh.NewOption(i18n.T("Quit"), ""),
		).Value(&how)); err != nil || how == "" {
		return false, nil
	}
	cmd := Root()
	switch how {
	case "new":
		cmd.SetArgs([]string{"init"})
	case "clone":
		var url string
		if err := ask(huh.NewInput().Title(i18n.T("Git URL")).Placeholder(i18n.T("git@github.com:you/agent-skills.git")).Value(&url)); err != nil || url == "" {
			return false, nil
		}
		cmd.SetArgs([]string{"clone", strings.TrimSpace(url)})
	case "existing":
		var dir string
		if err := ask(huh.NewInput().Title(i18n.T("Directory")).Placeholder(i18n.T("~/code/agent-skills")).Value(&dir)); err != nil || dir == "" {
			return false, nil
		}
		home, _ := os.UserHomeDir()
		dir = strings.Replace(strings.TrimSpace(dir), "~", home, 1)
		if !fsutil.Exists(filepath.Join(dir, manifest.FileName)) {
			return false, i18n.Errorf("%s has no %s", dir, manifest.FileName)
		}
		if err := app.UseLibrary(dir); err != nil {
			return false, err
		}
		i18n.Fprintf(stdout, "%s skd now uses %s\n", ok("✓"), dir)
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
		Short: i18n.T("Clone a library (your backup from another machine) and apply it"),
		Args:  rangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := library.DefaultRoot()
			if len(args) == 2 {
				dir = args[1]
			}
			dir, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			if fsutil.Lexists(dir) {
				return i18n.Errorf("%s already exists", dir)
			}
			if safetext.Has(args[0]) {
				return i18n.Errorf("%q has control characters: not a git URL", args[0])
			}
			if _, err := config.Load(); err != nil {
				return i18n.Errorf("%w; clone records the library there, so fix or remove it first (nothing was cloned)", err)
			}
			// Someone else's library is never one an older skd made links
			// from: no link outside it is taken for skd's on what its
			// skills.toml and skills.lock say.
			app.SealOlderSkd()
			// A repository that is not a library is not kept: no ✓, and
			// the error names skd add for a repository of skills.
			if err := app.CloneLibrary(args[0], dir); err != nil {
				return err
			}
			say("%s cloned into %s\n", ok("✓"), dir)
			// The upstream snapshots pushed with it: the merge base of the
			// next update of a skill changed there.
			if err := app.FetchSnapshots(dir); err != nil {
				say("%s could not fetch upstream's versions (%s): %v\n", warn("!"), engine.SnapshotRefs, err)
			}
			if err := library.EnsurePrivateGit(dir); errors.Is(err, library.ErrPrivateLink) {
				// Someone else's library: nothing is set up through the
				// link, and the skills under it are not linked (Resolve).
				say("%s %v; nothing was set up there, and skills under it are skipped\n", warn("!"), err)
			} else if err != nil {
				return err
			}
			if err := app.UseLibrary(dir); err != nil {
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
			// Someone else's library may name custom targets anywhere:
			// none is used until a person approves it here.
			approved, err := reviewCustomTargets(c.reg)
			if err != nil {
				return err
			}
			if approved {
				if c, err = open(); err != nil {
					return err
				}
			}
			// Vendored skills come with the clone; only links need creating.
			out, err := c.Apply(app.ApplyOptions{})
			if out != nil {
				printApply(c, out)
			}
			if err != nil {
				return err
			}
			jsonOut(map[string]any{"library": dir, "apply": out, "withheld": c.reg.Withheld()})
			return nil
		},
	}
}

func syncCmd() *cobra.Command {
	var remote, msg string
	var resolve, cont, abort bool
	cmd := &cobra.Command{
		Use:   "sync [--resolve | --continue | --abort]",
		Short: i18n.T("Commit the library, merge what its git remote has, and push (your backup)"),
		Long: i18n.T("sync commits the library, fetches its git remote and merges what the remote has that the library has not " +
			"(git merge, never rebase; what the merge brings in is linked), then pushes, with the upstream snapshots. When " +
			"both sides changed the same lines the sync is shelved: the merge is aborted, the library stays as it was, nothing " +
			"is pushed, and the conflicting files are listed. sync --resolve redoes the merge and leaves git's conflict state " +
			"in the library (conflict markers, files unmerged in the index: git status, git diff and git checkout " +
			"--ours/--theirs work); agents linked to skills among them read the markers until it is finished. Edit the files, " +
			"then sync --continue commits the merge and pushes (it refuses while a marker is left); sync --abort gives up " +
			"(git merge --abort). private/ is a separate repository and is never pushed."),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := open()
			if err != nil {
				return err
			}
			n := 0
			for _, b := range []bool{resolve, cont, abort} {
				if b {
					n++
				}
			}
			if n > 1 {
				return i18n.Errorf("--resolve, --continue and --abort: pick one")
			}
			switch {
			case resolve:
				res, err := c.SyncResolve()
				if err != nil {
					return err
				}
				if c.jsonOut(res) {
					return nil
				}
				if res.Sync != nil {
					say("%s %s\n", ok("✓"), i18n.T("merged without conflicts and pushed"))
					return nil
				}
				say("%s %s\n", warn("!"), i18n.Sprintf("the sync is being resolved: %s with conflicts", plural(len(res.Files), "file")))
				for _, f := range res.Files {
					say("  %s %s\n", warn("✗"), txt(f.Path))
				}
				say("%s\n", dim(i18n.T("edit them (git checkout --ours/--theirs <file> takes one side; agents linked to these skills read the markers meanwhile), then skd sync --continue · skd sync --abort gives up")))
				return nil
			case cont:
				out, err := c.SyncContinue()
				var ue *app.SyncUnresolvedError
				if errors.As(err, &ue) {
					jsonOut(map[string]any{"unresolved": ue.Files})
					return err
				}
				if out != nil && out.Apply != nil {
					printApply(c, out.Apply)
				}
				if err != nil {
					return err
				}
				say("%s %s\n", ok("✓"), i18n.T("merged and pushed"))
				jsonOut(out)
				return nil
			case abort:
				if err := c.SyncAbort(); err != nil {
					return err
				}
				say("%s %s\n", ok("✓"), i18n.T("the merge is aborted: the library is as it was, the sync still shelved"))
				jsonOut(map[string]any{"aborted": true})
				return nil
			}
			out, err := c.Backup(remote, msg)
			if out != nil {
				if out.Committed {
					c.printf("%s committed in %s\n", ok("✓"), c.lib.Root)
				} else {
					say("%s\n", dim(i18n.T("nothing to commit")))
				}
				if out.Pulled {
					say("%s %s\n", ok("✓"), i18n.T("merged what the remote had"))
					if out.Apply != nil {
						printApply(c, out.Apply)
					}
				}
			}
			if err != nil {
				return err
			}
			if len(out.Conflicts) > 0 {
				jsonOut(out)
				say("%s %s\n", warn("!"), i18n.Sprintf("sync shelved: the remote and this library both changed %s; nothing was merged or pushed", plural(len(out.Conflicts), "file")))
				for _, f := range out.Conflicts {
					say("  %s %s\n", warn("✗"), txt(f))
				}
				return i18n.Errorf("sync shelved with conflicts; skd sync --resolve to resolve them")
			}
			if out.Remote == "" {
				say("%s no remote configured; add one with: skd sync --remote <git-url>\n", warn("!"))
			} else {
				say("%s pushed\n", ok("✓"))
			}
			if out.PrivateUncommitted {
				say("%s private/ has uncommitted changes (it is a separate repo and is never pushed here)\n", dim(i18n.T("note:")))
			}
			jsonOut(out)
			return nil
		},
	}
	cmd.Flags().StringVar(&remote, "remote", "", i18n.T("set the git remote (origin) before pushing"))
	cmd.Flags().StringVarP(&msg, "message", "m", "", i18n.T("commit message"))
	cmd.Flags().BoolVar(&resolve, "resolve", false, i18n.T("redo a shelved sync's merge, leaving its conflicts in the library to resolve"))
	cmd.Flags().BoolVar(&cont, "continue", false, i18n.T("finish a sync being resolved: commit the merge and push"))
	cmd.Flags().BoolVar(&abort, "abort", false, i18n.T("give up a sync being resolved (git merge --abort)"))
	return cmd
}

var errNoTTY = i18n.New("no terminal; run `skd --help` for commands")
