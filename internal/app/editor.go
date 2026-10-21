package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
)

// Editor is the program that opens a skill directory.
type Editor struct {
	// Argv is the command; the directory is appended.
	Argv []string
	// Terminal is set for an editor that runs in the terminal (vim,
	// nano, …): it needs the terminal to itself until it exits. Others
	// open a window and are started without waiting.
	Terminal bool
	// From says where the choice came from: "editor setting", "$VISUAL",
	// "$EDITOR", "found on this machine", "system default".
	From string
}

// Name is the editor's command name.
func (e Editor) Name() string { return filepath.Base(e.Argv[0]) }

// Command is the command that opens dir.
func (e Editor) Command(dir string) *exec.Cmd {
	return exec.Command(e.Argv[0], append(append([]string{}, e.Argv[1:]...), dir)...)
}

// Start opens dir in a window and returns without waiting for the editor
// to close; the process is reaped in the background.
func (e Editor) Start(dir string) error {
	cmd := e.Command(dir)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// windowed are editors (and openers) that open a window rather than run in
// the terminal.
var windowed = map[string]bool{"code": true, "code-insiders": true, "codium": true, "cursor": true, "windsurf": true, "zed": true,
	"zeditor": true, "subl": true, "sublime_text": true, "mate": true, "bbedit": true, "nova": true, "fleet": true, "idea": true,
	"goland": true, "webstorm": true, "pycharm": true, "atom": true, "gedit": true, "kate": true, "gvim": true, "mvim": true,
	"open": true, "xdg-open": true, "explorer": true, "explorer.exe": true, "notepad++": true}

// detected are the editors looked for when nothing is configured: the
// command-line launcher, then the macOS application.
var detected = []struct{ cmd, app string }{
	{"code", "Visual Studio Code.app"}, {"cursor", "Cursor.app"}, {"zed", "Zed.app"}, {"subl", "Sublime Text.app"},
}

// Replaceable in tests.
var (
	lookPath = exec.LookPath
	goos     = runtime.GOOS
	// appDirs are where macOS applications are looked for.
	appDirs = func() []string {
		home, _ := os.UserHomeDir()
		return []string{"/Applications", filepath.Join(home, "Applications")}
	}
)

// FindEditor picks the editor: the editor setting, $VISUAL, $EDITOR, then
// VS Code, Cursor, Zed or Sublime Text (their command, or on macOS the
// application), then the system's default opener.
func FindEditor() (Editor, error) {
	for _, c := range []struct{ value, from string }{
		{config.Get(config.Editor), i18n.M("editor setting")}, {os.Getenv("VISUAL"), "$VISUAL"}, {os.Getenv("EDITOR"), "$EDITOR"},
	} {
		if strings.TrimSpace(c.value) == "" {
			continue
		}
		argv, err := SplitCommand(c.value)
		if err != nil {
			return Editor{}, i18n.Errorf("%s %q: %w", i18n.T(c.from), c.value, err)
		}
		terminal := !windowed[strings.ToLower(filepath.Base(argv[0]))]
		switch strings.ToLower(strings.TrimSpace(config.Get(config.EditorKind))) {
		case "window":
			terminal = false
		case "terminal":
			terminal = true
		}
		return Editor{Argv: argv, Terminal: terminal, From: c.from}, nil
	}
	for _, d := range detected {
		if p, err := lookPath(d.cmd); err == nil {
			return Editor{Argv: []string{p}, From: i18n.M("found on this machine")}, nil
		}
	}
	if goos == "darwin" {
		for _, d := range detected {
			for _, dir := range appDirs() {
				if p := filepath.Join(dir, d.app); fsutil.Exists(p) {
					return Editor{Argv: []string{"open", "-a", p}, From: i18n.M("found on this machine")}, nil
				}
			}
		}
	}
	opener := map[string][]string{"darwin": {"open"}, "windows": {"explorer"}}[goos]
	if opener == nil {
		opener = []string{"xdg-open"}
	}
	if _, err := lookPath(opener[0]); err != nil {
		return Editor{}, i18n.New("no editor found: set one with `skd config set editor <command>`, or $EDITOR")
	}
	return Editor{Argv: opener, From: i18n.M("system default")}, nil
}

// NoTerminalError is why ed cannot open dir: it is taken for a terminal
// editor and there is no terminal to hand it. It says how to tell skd the
// editor opens a window instead, when that is what it is.
func NoTerminalError(ed Editor, dir string) error {
	why := i18n.Sprintf("%s runs in a terminal", ed.Name())
	if config.Get(config.EditorKind) != "terminal" {
		why = i18n.Sprintf("%s is not an editor skd knows to open a window, so it runs it in the terminal", ed.Name())
	}
	return i18n.Errorf("%s, and there is none. If it opens a window: skd config set editor-kind window. "+
		"Or choose another editor (skd config set editor <command>), or open %s yourself", why, dir)
}

// OverwrittenError is returned by Edit for a skill whose files skd or an
// application replaces: edits would be lost. Fork it, or open it anyway.
type OverwrittenError struct {
	Entry engine.Entry
}

func (e *OverwrittenError) Error() string {
	return i18n.Sprintf("%s %s; fork it to edit your own copy (skd edit %s --fork), or open the files anyway (--force)",
		e.Entry.LinkName, OverwriteNote(e.Entry), e.Entry.LinkName)
}

// OverwriteNote says why edits to e's files would be lost, or "" when
// they are the user's own.
func OverwriteNote(e engine.Entry) string {
	switch e.Kind {
	case engine.KindVendor:
		return i18n.Sprintf("is vendored from %s: skd update merges upstream's changes into edits there, and waits for skd resolve when they conflict", e.Origin)
	case engine.KindApp:
		return i18n.Sprintf("ships with the %s application: its updates replace the files, so edits there are lost", e.Origin)
	}
	return ""
}

// EditOptions say what to do with a skill whose files are replaced on
// update (vendored, or an application's).
type EditOptions struct {
	// Fork copies it into skills/ (private/ with Private) as As (its name
	// when empty) and opens the copy: skd fork.
	Fork    bool
	Private bool
	As      string
	// Force opens the files anyway.
	Force bool
	// NoTerminal says the caller has no terminal to hand a terminal
	// editor (vim, nano): Edit then refuses one before changing anything.
	NoTerminal bool
}

// EditOutcome is what Edit decided to open.
type EditOutcome struct {
	Entry engine.Entry
	// Dir is the directory to open: the skill's, or the fork's.
	Dir string
	// Fork is set when the skill was forked first.
	Fork   *ForkOutcome
	Editor Editor
}

// Edit finds the skill r names and the editor to open it with; front
// ends then open Dir (Editor.Start, or in the foreground for a terminal
// editor). A vendored or app skill is refused with *OverwrittenError
// unless opts fork it (then the fork is opened) or force it. Nothing is
// forked when the editor could not be started anyway: one that is not
// installed, or a terminal editor with opts.NoTerminal.
func (s *Session) Edit(r Ref, opts EditOptions) (*EditOutcome, error) {
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	e, err := s.mustEntry(entries, r)
	if err != nil {
		return nil, err
	}
	ed, err := FindEditor()
	if err != nil {
		return nil, err
	}
	if _, err := lookPath(ed.Argv[0]); err != nil {
		return nil, i18n.Errorf("editor %q (%s) not found; set one with skd config set editor <command>", ed.Argv[0], i18n.T(ed.From))
	}
	if ed.Terminal && opts.NoTerminal {
		return nil, NoTerminalError(ed, e.Dir)
	}
	out := &EditOutcome{Entry: *e, Dir: e.Dir, Editor: ed}
	over := OverwriteNote(*e) != ""
	switch {
	case opts.Fork && !over:
		return nil, i18n.Errorf("%q is already yours (%s); nothing to fork", e.LinkName, e.Dir)
	case opts.Fork:
		f, err := s.Fork(r, opts.Private, opts.As)
		if err != nil {
			return nil, err
		}
		out.Fork, out.Dir = f, f.Path
	case over && !opts.Force:
		return nil, &OverwrittenError{Entry: *e}
	}
	if !fsutil.Exists(out.Dir) {
		return nil, i18n.Errorf("%s is missing (skd update fetches a vendored skill again)", out.Dir)
	}
	return out, nil
}
