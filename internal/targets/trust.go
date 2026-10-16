package targets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/fsutil"
)

// A custom target is a directory skills.toml names, and skills.toml may
// come from someone else's library (skd clone). skd writes links and
// copies into a target, restores trash entries there and, with --force,
// moves what is in the way into the library trash. So a custom target is
// used only when it is in an allowed place (CheckPath) and a person
// approved that name with that directory on this machine (Approve). The
// approvals are kept outside every library, beside the settings file,
// so a library cannot bring its own.

// Sensitive are places under the home directory, given relative to it,
// that a custom target may not be, be inside, or contain: keys and
// credentials, and directories whose contents run or load by themselves.
// skd's own settings directory is one too (SensitiveDirs).
var Sensitive = []string{
	".ssh", ".gnupg", ".aws", ".azure", ".kube", ".docker", ".password-store",
	".config/gcloud", ".config/gh", ".config/git", ".config/autostart", ".config/systemd", ".config/environment.d",
	".local/bin", "bin",
	"Library/LaunchAgents", "Library/Keychains",
	"AppData/Roaming/Microsoft/Windows/Start Menu/Programs/Startup",
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

// CheckPath reports where a custom target's path is (real: ~ expanded,
// symbolic links resolved) and why skd may not use it there. A custom
// target must be an absolute path (or start with ~) strictly inside the
// home directory, and must not be, be inside, or contain a sensitive
// place (SensitiveDirs), nor contain a built-in target's directory (the
// agent's own directory holds its settings). Paths are compared after
// resolving links, as the same file (another spelling of one directory
// on a file system that ignores case), and, to refuse, also ignoring
// case.
func CheckPath(path string) (real string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return checkPath(path, home)
}

func checkPath(path, home string) (string, error) {
	p := Expand(path, home)
	if path == "" || !filepath.IsAbs(p) {
		return "", fmt.Errorf("its path %q is not absolute; start it with ~/ or /", path)
	}
	real := fsutil.Resolve(p)
	h := fsutil.Resolve(filepath.Clean(home))
	if fsutil.SameDir(real, h, true) {
		return real, fmt.Errorf("its path %q is your home directory itself", path)
	}
	if !fsutil.Inside(h, real, false) {
		return real, fmt.Errorf("its path %q is %s, not inside your home directory", path, real)
	}
	for _, s := range SensitiveDirs(home) {
		s = fsutil.Resolve(s)
		if fsutil.Inside(s, real, true) || fsutil.Inside(real, s, true) {
			return real, fmt.Errorf("its path %q is %s, which is or holds %s; skd never writes there", path, real, s)
		}
	}
	for _, b := range builtin {
		d := fsutil.Resolve(filepath.Join(home, filepath.FromSlash(b.rel)))
		if fsutil.Inside(real, d, true) && !fsutil.SameDir(real, d, true) {
			return real, fmt.Errorf("its path %q is %s, which holds the %s agent's skill directory %s; skd never writes beside it", path, real, b.name, d)
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

// inspect is checkPath for a custom target named name, which must not
// have a built-in target's name either.
func inspect(name, path, home string) (string, error) {
	if IsBuiltin(name) {
		real, _ := checkPath(path, home)
		return real, fmt.Errorf("it has the name of a built-in target; the built-in %s is used, give the custom one another name in skills.toml", name)
	}
	return checkPath(path, home)
}

// Approval is one custom target a person approved: the name and the
// directory (links resolved) it was approved with.
type Approval struct {
	Name     string    `toml:"name" json:"name"`
	Path     string    `toml:"path" json:"path"`
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
	return f.Target, nil
}

func isApproved(list []Approval, name, real string) bool {
	for _, a := range list {
		if a.Name == name && a.Path == real {
			return true
		}
	}
	return false
}

// Approve records that a person approved the custom target name at path
// (as skills.toml has it). want is the directory the person was shown:
// when the path no longer leads there (the library or a link changed
// meanwhile) nothing is recorded. Only a person at a terminal may call
// this; the callers check.
func Approve(name, path, want string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	real, err := inspect(name, path, home)
	if err != nil {
		return fmt.Errorf("custom target %q cannot be approved: %w", name, err)
	}
	if real != want {
		return fmt.Errorf("custom target %q now leads to %s, not %s as shown; nothing was approved", name, real, want)
	}
	list, err := Approvals()
	if err != nil {
		return fmt.Errorf("%w; fix or remove it first", err)
	}
	if isApproved(list, name, real) {
		return nil
	}
	return writeApprovals(append(list, Approval{Name: name, Path: real, Approved: time.Now().UTC().Truncate(time.Second)}))
}

// Revoke drops every approval of the custom target name and returns how
// many there were.
func Revoke(name string) (int, error) {
	list, err := Approvals()
	if err != nil {
		return 0, fmt.Errorf("%w; fix or remove it first", err)
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
# when its name and its directory (symbolic links resolved) are here.
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
