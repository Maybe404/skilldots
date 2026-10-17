package targets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
)

// A custom target is a directory skills.toml names, and skills.toml may
// come from someone else's library (skd clone). skd writes links and
// copies into a target, restores trash entries there and, with --force,
// moves what is in the way into the library trash. So a custom target is
// used only when it is in an allowed place (checkPath, inspect) and a
// person approved that name with that directory and mode on this machine
// (Approve). The
// approvals are kept outside every library, beside the settings file,
// so a library cannot bring its own.

// Sensitive are places under the home directory, given relative to it,
// that a custom target may not be, be inside, or contain: keys and
// credentials, and directories whose contents run or load by themselves
// (at login, from PATH, or in a shell, an editor or git). skd's own
// settings directory and the library are too (SensitiveDirs, checkPath).
var Sensitive = append(append([]string{}, Secret...),
	// Started at login or by a service manager.
	".config/autostart", ".config/systemd", ".config/environment.d", ".local/share/systemd",
	"Library/LaunchAgents", "Library/LaunchDaemons",
	"AppData/Roaming/Microsoft/Windows/Start Menu/Programs/Startup",
	// Programs: on PATH, found by name, or opened as applications.
	".local/bin", "bin", "go/bin", ".cargo", "node_modules", "Applications", ".local/share/applications",
	// Loaded by a shell, an editor, git or an automation tool.
	".oh-my-zsh", ".zsh", ".config/zsh", ".config/fish", ".bashrc.d",
	".vim", ".config/nvim", ".local/share/nvim", ".emacs.d", ".config/emacs",
	".vscode", ".vscode-insiders", ".vscode-server", ".cursor/extensions", ".config/Code",
	"Library/Application Support/Code", ".git-templates", ".hammerspoon",
	"Library/Services", "Library/Scripts", "Library/Workflows", "Library/PreferencePanes",
	"Library/QuickLook", "Library/Input Methods",
	"Documents/PowerShell", "Documents/WindowsPowerShell",
)

// Secret are places under the home directory, given relative to it,
// that hold keys and credentials: skd never writes there (they are in
// Sensitive) and never links or copies an app's skill from there or
// from a directory that holds one (SecretDirs).
var Secret = []string{
	".ssh", ".gnupg", ".aws", ".azure", ".kube", ".docker", ".password-store",
	".config/gcloud", ".config/gh", ".config/git", "Library/Keychains",
}

// SecretDirs are the absolute places an app's skill directory may not
// be, be inside, or contain: Secret under home, and skd's settings
// directory (the approvals of custom targets).
func SecretDirs(home string) []string {
	out := make([]string, 0, len(Secret)+1)
	for _, s := range Secret {
		out = append(out, filepath.Join(home, filepath.FromSlash(s)))
	}
	return append(out, filepath.Dir(config.Path()))
}

// SensitiveDirs are the absolute places a custom target may not be, be
// inside, or contain: Sensitive under home, and skd's settings directory
// (where config.toml and the approvals are).
func SensitiveDirs(home string) []string {
	out := make([]string, 0, len(Sensitive)+1)
	for _, s := range Sensitive {
		out = append(out, filepath.Join(home, filepath.FromSlash(s)))
	}
	return append(out, filepath.Dir(config.Path()))
}

// Unusual reports whether a custom target's directory (real) is not
// named skills or skill (ignoring case): not a directory meant for
// skills, so likely read by something else too. Approving one asks the
// person to type the target's name, not just to say yes.
func Unusual(real string) bool {
	b := filepath.Base(real)
	return !strings.EqualFold(b, "skills") && !strings.EqualFold(b, "skill")
}

// CheckPath reports where a custom target's path is (real: ~ expanded,
// symbolic links resolved) and why skd may not use it there, for a
// target of the library at lib (none when "") with no other custom
// target. See checkPath.
func CheckPath(path, lib string) (real string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return checkPath(path, home, lib)
}

// checkPath is where a custom target's path is and why skd may not use
// it there. A custom target must be an absolute path (or start with ~)
// strictly inside the home directory, and must not be, be inside, or
// contain a sensitive place (SensitiveDirs) or the library at lib; it
// must not contain a built-in target's directory (the agent's own
// directory holds its settings) nor be inside one (skd writes that
// directory's entries itself, and one of them may be a link into the
// library that leads anywhere; being one is allowed). Paths are compared
// after resolving links, as the same file (another spelling of one
// directory on a file system that ignores case), and, to refuse, also
// ignoring case.
func checkPath(path, home, lib string) (string, error) {
	p := Expand(path, home)
	if path == "" || !filepath.IsAbs(p) {
		return "", i18n.Errorf("its path %q is not absolute; start it with ~/ or /", path)
	}
	real := fsutil.Resolve(p)
	// Resolve leaves a link it cannot follow as it is: a link that leads
	// nowhere yet would lead wherever something appears later (a link skd
	// itself makes in a built-in target, say), so where the path is
	// cannot be known.
	for q := real; ; q = filepath.Dir(q) {
		if st, err := os.Lstat(q); err == nil && st.Mode()&os.ModeSymlink != 0 {
			return real, i18n.Errorf("its path %q goes through %s, a symbolic link that leads nowhere", path, q)
		}
		if filepath.Dir(q) == q {
			break
		}
	}
	h := fsutil.Resolve(filepath.Clean(home))
	if fsutil.SameDir(real, h, true) {
		return real, i18n.Errorf("its path %q is your home directory itself", path)
	}
	if !fsutil.Inside(h, real, false) {
		return real, i18n.Errorf("its path %q is %s, not inside your home directory", path, real)
	}
	for _, s := range SensitiveDirs(home) {
		s = fsutil.Resolve(s)
		if fsutil.Inside(s, real, true) || fsutil.Inside(real, s, true) {
			return real, i18n.Errorf("its path %q is %s, which is or holds %s; skd never writes there", path, real, s)
		}
	}
	if lib != "" {
		if a, err := filepath.Abs(lib); err == nil {
			lib = a
		}
		if l := fsutil.Resolve(lib); fsutil.Inside(l, real, true) || fsutil.Inside(real, l, true) {
			return real, i18n.Errorf("its path %q is %s, which is, is inside or holds the library %s; skd never writes there", path, real, l)
		}
	}
	for _, b := range builtin {
		d := fsutil.Resolve(filepath.Join(home, filepath.FromSlash(b.rel)))
		if fsutil.SameDir(real, d, true) {
			continue
		}
		if fsutil.Inside(real, d, true) {
			return real, i18n.Errorf("its path %q is %s, which holds the %s agent's skill directory %s; skd never writes beside it", path, real, b.name, d)
		}
		if fsutil.Inside(d, real, true) {
			return real, i18n.Errorf("its path %q is %s, inside the %s agent's skill directory %s, whose entries skd writes itself", path, real, b.name, d)
		}
	}
	return real, nil
}

// IsBuiltin reports whether name is a built-in target's name.
func IsBuiltin(name string) bool {
	for _, b := range builtin {
		if b.name == name {
			return true
		}
	}
	return false
}

// inspect is where the custom target name of r leads (its path as
// skills.toml has it now, links resolved now) and why skd may not use
// it: a built-in target's name, a mode other than link or copy, a path
// checkPath refuses, or a directory that is, is inside or holds another
// custom target's (each target's entries are written through that
// target only; another target's writes could put a link on the way).
// Only other custom targets a person approved (in approved, with the
// directory and mode they have now) count: a library cannot withhold a
// target the user approved by adding an unapproved one inside it, and
// one bad entry does not withhold the rest. The unapproved one is not
// used anyway, and cannot be approved while the other is.
func (r *Registry) inspect(name string, approved []Approval) (string, error) {
	c := r.custom[name]
	real, err := checkPath(c.Path, r.home, r.lib)
	switch {
	case IsBuiltin(name):
		return real, i18n.Errorf("it has the name of a built-in target; the built-in %s is used, give the custom one another name in skills.toml", name)
	case c.Mode != "link" && c.Mode != "copy":
		return real, i18n.Errorf("its mode %q is neither link nor copy", c.Mode)
	case err != nil:
		return real, err
	}
	others := make([]string, 0, len(r.custom))
	for n := range r.custom {
		others = append(others, n)
	}
	sort.Strings(others)
	for _, n := range others {
		if n == name {
			continue
		}
		o, err := checkPath(r.custom[n].Path, r.home, r.lib)
		if err != nil || !isApproved(approved, n, o, r.custom[n].Mode) {
			continue
		}
		if fsutil.Inside(o, real, true) || fsutil.Inside(real, o, true) {
			return real, i18n.Errorf("its path %q is %s, which is, is inside or holds the directory %s of custom target %q; give each custom target a directory of its own", c.Path, real, o, n)
		}
	}
	return real, nil
}

// Approval is one custom target a person approved: the name, the
// directory (links resolved) and the mode (link or copy) it was approved
// with. A record without a mode (written before skd recorded modes) is
// an approval for link: a target set to copy needs approving again.
type Approval struct {
	Name     string    `toml:"name" json:"name"`
	Path     string    `toml:"path" json:"path"`
	Mode     string    `toml:"mode" json:"mode"`
	Approved time.Time `toml:"approved" json:"approved"`
}

type approvalsFile struct {
	Target []Approval `toml:"target"`
}

// ApprovalsFile is where approvals are kept: beside the settings file
// (config.toml), per user and per machine, never in a library.
func ApprovalsFile() string {
	return filepath.Join(filepath.Dir(config.Path()), "trusted-targets.toml")
}

// Approvals reads the approvals; none when the file does not exist.
func Approvals() ([]Approval, error) {
	b, err := os.ReadFile(ApprovalsFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f approvalsFile
	if err := toml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", ApprovalsFile(), err)
	}
	for i := range f.Target {
		if f.Target[i].Mode == "" {
			f.Target[i].Mode = "link"
		}
	}
	return f.Target, nil
}

func isApproved(list []Approval, name, real, mode string) bool {
	for _, a := range list {
		if a.Name == name && a.Path == real && a.Mode == mode {
			return true
		}
	}
	return false
}

// Approve records that a person approved the custom target shown: its
// name, its path as skills.toml has it, the directory it leads to (Real)
// and its mode, as the person was shown them (a Withheld of this or an
// earlier registry). When skills.toml or a link on the way changed
// meanwhile, so that the target is no longer what was shown, nothing is
// recorded. An Unusual directory (not named skills) needs typed to be
// the target's name: the person typed it to confirm. Only a person at a
// terminal may call this; the callers check.
func (r *Registry) Approve(shown Withheld, typed string) error {
	name := shown.Name
	c, ok := r.custom[name]
	if !ok {
		return i18n.Errorf("skills.toml has no custom target %q", name)
	}
	if c.Path != shown.Path || c.Mode != shown.Mode {
		return i18n.Errorf("custom target %q is now %q (%s) in skills.toml, not %q (%s) as shown; nothing was approved", name, c.Path, c.Mode, shown.Path, shown.Mode)
	}
	list, err := Approvals()
	if err != nil {
		return i18n.Errorf("%w; fix or remove it first", err)
	}
	real, err := r.inspect(name, list)
	if err != nil {
		return i18n.Errorf("custom target %q cannot be approved: %w", name, err)
	}
	if real != shown.Real {
		return i18n.Errorf("custom target %q now leads to %s, not %s as shown; nothing was approved", name, real, shown.Real)
	}
	if Unusual(real) && typed != name {
		return i18n.Errorf("custom target %q leads to %s, which is not named skills; it is approved only when its name is typed to confirm; nothing was approved", name, real)
	}
	if isApproved(list, name, real, c.Mode) {
		return nil
	}
	return writeApprovals(append(list, Approval{Name: name, Path: real, Mode: c.Mode, Approved: time.Now().UTC().Truncate(time.Second)}))
}

// Revoke drops every approval of the custom target name and returns how
// many there were.
func Revoke(name string) (int, error) {
	list, err := Approvals()
	if err != nil {
		return 0, i18n.Errorf("%w; fix or remove it first", err)
	}
	var keep []Approval
	for _, a := range list {
		if a.Name != name {
			keep = append(keep, a)
		}
	}
	n := len(list) - len(keep)
	if n == 0 {
		return 0, nil
	}
	return n, writeApprovals(keep)
}

const approvalsHeader = `# Custom targets of skills.toml a person approved on this machine with
# ` + "`skd targets trust <name>`" + `. skd writes skills into a custom target only
# when its name, its directory (symbolic links resolved) and its mode are
# here.
# Per user and per machine: never part of a library.
`

func writeApprovals(list []Approval) error {
	b, err := toml.Marshal(approvalsFile{Target: list})
	if err != nil {
		return err
	}
	p := ApprovalsFile()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return fsutil.WriteAtomic(p, []byte(approvalsHeader+strings.TrimLeft(string(b), "\n")))
}
