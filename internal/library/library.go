// Package library locates a skilldots library on disk and wraps the handful
// of git operations skilldots needs. git is invoked as a subprocess so the
// behaviour matches what the user sees in their own shell.
package library

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
)

const (
	EnvLibrary = "SKILLDOTS_LIBRARY"
	EnvCache   = "SKILLDOTS_CACHE"
	DirSkills  = "skills"
	DirPrivate = "private"
	DirVendor  = "vendor"
)

// Library is an opened skilldots library.
type Library struct {
	Root     string
	Manifest *manifest.Manifest
	Lock     *lock.Lock
}

// DefaultRoot is where `skd init` creates a library unless told otherwise.
func DefaultRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".skilldots")
}

// ConfigPath is the small per-user file that remembers where the library
// is, along with the settings `skd config` changes.
func ConfigPath() string { return config.Path() }

// Remember records root in the user config so later invocations find it
// from any directory. Other settings in the file are kept.
func Remember(root string) error { return config.SetLibrary(root) }

// Locate finds the library: explicit flag, $SKILLDOTS_LIBRARY, a skills.toml
// in the current directory or any parent, the remembered config, then the
// default root.
func Locate(flag string) (string, error) {
	candidates := []string{flag, os.Getenv(EnvLibrary)}
	if cwd, err := os.Getwd(); err == nil {
		for d := cwd; ; d = filepath.Dir(d) {
			if _, err := os.Stat(filepath.Join(d, manifest.FileName)); err == nil {
				candidates = append(candidates, d)
				break
			}
			if filepath.Dir(d) == d {
				break
			}
		}
	}
	// The settings file remembers the library (skd use). When it cannot
	// be read, say so rather than fall back to the default directory or
	// claim there is no library.
	settings, serr := config.Load()
	if serr != nil {
		for _, c := range candidates {
			if _, err := os.Stat(filepath.Join(c, manifest.FileName)); c != "" && err == nil {
				return c, nil
			}
			if c != "" && c == flag {
				return "", i18n.Errorf("no %s in %s", manifest.FileName, c)
			}
		}
		return "", i18n.Errorf("cannot find the library: %w (it remembers the library); fix it, or name the library with --library or $%s", serr, EnvLibrary)
	}
	candidates = append(candidates, settings[config.Library], DefaultRoot())
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, manifest.FileName)); err == nil {
			return c, nil
		}
		if c == flag {
			return "", i18n.Errorf("no %s in %s", manifest.FileName, c)
		}
	}
	return "", i18n.New("no skilldots library found; run `skd init`, `skd clone <url>`, or `skd use <dir>`")
}

// Open loads manifest and lock from root.
func Open(root string) (*Library, error) {
	m, err := manifest.Load(filepath.Join(root, manifest.FileName))
	if err != nil {
		return nil, err
	}
	l, err := lock.Load(filepath.Join(root, lock.FileName))
	if err != nil {
		return nil, err
	}
	return &Library{Root: root, Manifest: m, Lock: l}, nil
}

// Writable reports why Save would refuse to write the manifest or the
// lock (CheckFile), before anything else is changed.
func (l *Library) Writable() error {
	for _, f := range []string{manifest.FileName, lock.FileName} {
		if err := CheckFile(l.Root, filepath.Join(l.Root, f)); err != nil {
			return err
		}
	}
	return nil
}

// Save writes manifest and lock back.
func (l *Library) Save() error {
	if err := l.Writable(); err != nil {
		return err
	}
	if err := l.Manifest.Save(filepath.Join(l.Root, manifest.FileName)); err != nil {
		return err
	}
	return l.Lock.Save(filepath.Join(l.Root, lock.FileName))
}

// ownFiles are the library files skd writes at its root, besides
// .gitignore.
var ownFiles = []string{manifest.FileName, lock.FileName, "skills.history.jsonl"}

// CheckFile reports why skd must not write p, one of the library's own
// files at root (skills.toml, skills.lock, the update history,
// .gitignore). A library may come from someone else (skd clone), and a
// link there would have skd overwrite or append to any file it names:
// skills.lock -> ../.codex/config.toml outside the library, or
// skills.toml -> .git/config (the library's git settings) and
// skills.history.jsonl -> private/.git/hooks/... inside it. So when p is
// a symbolic link, skd writes through it only to a regular file directly
// in the library root, not hidden (no ".git", ".gitattributes", ...) and
// not another of skd's own files; anything else (outside the library,
// deeper in it, nowhere, or a directory) is refused. Reading through it
// is harmless.
func CheckFile(root, p string) error {
	st, err := os.Lstat(p)
	if err != nil || st.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	refuse := func(why string) error {
		return i18n.Errorf("%s is a symbolic link %s; skd does not write through it (replace it with the file itself)", p, why)
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return refuse(i18n.T("that leads nowhere"))
	}
	if a, err := filepath.Abs(real); err == nil {
		real = a
	}
	r, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if rr, err := filepath.EvalSymlinks(r); err == nil {
		r = rr
	}
	if !fsutil.Within(r, real) {
		return refuse(i18n.Sprintf("to %s, outside the library", real))
	}
	base := filepath.Base(real)
	if !fsutil.SameDir(filepath.Dir(real), r, false) {
		return refuse(i18n.Sprintf("to %s, not a file directly in the library's folder", real))
	}
	if strings.HasPrefix(base, ".") || slices.Contains(ownFiles, base) {
		return refuse(i18n.Sprintf("to %s, one of the library's hidden or own files", real))
	}
	if st, err := os.Stat(real); err != nil || !st.Mode().IsRegular() {
		return refuse(i18n.Sprintf("to %s, which is not a file", real))
	}
	return nil
}

// Git runs git inside the library root.
func (l *Library) Git(args ...string) (string, error) { return Git(l.Root, args...) }

func (l *Library) SkillsDir() string  { return filepath.Join(l.Root, DirSkills) }
func (l *Library) PrivateDir() string { return filepath.Join(l.Root, DirPrivate) }
func (l *Library) VendorDir() string  { return filepath.Join(l.Root, DirVendor) }

// PackDir is where a pack's vendored skills live.
func (l *Library) PackDir(repo string) string {
	return filepath.Join(l.VendorDir(), filepath.FromSlash(repo))
}

// Rel returns p relative to the library root with forward slashes, or p
// unchanged when it is outside the library.
func (l *Library) Rel(p string) string {
	r, err := filepath.Rel(l.Root, p)
	if err != nil || strings.HasPrefix(r, "..") {
		return p
	}
	return filepath.ToSlash(r)
}

// Abs resolves a path stored by Rel.
func (l *Library) Abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(l.Root, filepath.FromSlash(p))
}

// CacheDir is where upstream repositories are cloned.
func CacheDir() string {
	if v := os.Getenv(EnvCache); v != "" {
		return v
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "skilldots", "repos")
	}
	return filepath.Join(DefaultRoot(), ".cache", "repos")
}

// Git runs git in dir and returns trimmed stdout.
func Git(dir string, args ...string) (string, error) {
	return GitContext(context.Background(), dir, args...)
}

// GitContext is Git that stops when ctx is done. Then git and everything
// it started (a transport helper, ssh, upload-pack) are asked to stop
// with SIGTERM, on which git removes its lock files; whatever is still
// running after a second is killed. A ctx that can never be done runs
// git exactly as Git always has.
func GitContext(ctx context.Context, dir string, args ...string) (string, error) {
	return GitEnvContext(ctx, dir, nil, args...)
}

// GitEnvContext is GitContext with env added to git's environment.
func GitEnvContext(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if ctx.Done() != nil {
		cancelTree(cmd)
		cmd.WaitDelay = time.Second
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", i18n.Errorf("git %s: %w", strings.Join(args, " "), ctx.Err())
		}
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		if strings.Contains(msg, "end-of-options") {
			// git before 2.24. Without the marker git reads the arguments
			// the same, as long as none of them up to "--" starts with "-"
			// (commits from the lock are hex, checked when it is read).
			if older, ok := withoutEndOfOptions(args); ok {
				return GitEnvContext(ctx, dir, env, older...)
			}
			if slices.Contains(args, "--end-of-options") {
				msg += i18n.T("\n(skd needs git 2.24 or later: this git does not know --end-of-options)")
			}
		}
		return "", i18n.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// withoutEndOfOptions is args without "--end-of-options", for a git that
// does not know it; ok is false when it is not there or when an argument
// after it (up to "--") starts with "-" and would be taken for an option.
func withoutEndOfOptions(args []string) (out []string, ok bool) {
	i := slices.Index(args, "--end-of-options")
	if i < 0 {
		return nil, false
	}
	for _, a := range args[i+1:] {
		if a == "--" {
			break
		}
		if strings.HasPrefix(a, "-") {
			return nil, false
		}
	}
	return append(append([]string{}, args[:i]...), args[i+1:]...), true
}

// IsGitRepo reports whether dir is the top of its own git work tree. A
// directory merely inside some larger repository does not count: the
// library and private/ each need their own.
func IsGitRepo(dir string) bool {
	out, err := Git(dir, "rev-parse", "--show-toplevel")
	if err != nil || out == "" {
		return false
	}
	return samePath(out, dir)
}

func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// ErrPrivateLink is returned by EnsurePrivateGit when the library's
// private/ is reached through a symbolic link inside the library.
var ErrPrivateLink = i18n.New("a symbolic link")

// PrivateLink returns the symbolic link on the way from root to its
// private/ (private/ itself, as a rule), or "" when there is none.
func PrivateLink(root string) string {
	return fsutil.LinkOnPath(root, filepath.Join(root, DirPrivate))
}

// EnsurePrivateGit makes the library's private/ a git repository of its
// own (creating it if needed), unless private/ is reached through a
// symbolic link inside the library: a library may come from someone
// else (skd clone), and its private -> .. would have git init run in
// whatever the link names, outside the library (a ~/.git that makes the
// whole home folder a repository). Then nothing is created or changed
// and the error wraps ErrPrivateLink.
func EnsurePrivateGit(root string) error {
	if l := PrivateLink(root); l != "" {
		return i18n.Errorf("%s is %w; skd does not set up a git repository through it (make private/ a directory of its own)", l, ErrPrivateLink)
	}
	return EnsureGit(filepath.Join(root, DirPrivate))
}

// EnsureGit initialises dir as a git repository when it is not one,
// creating dir first if needed. dir is trusted: for the library's
// private/ use EnsurePrivateGit.
func EnsureGit(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if IsGitRepo(dir) {
		return nil
	}
	_, err := Git(dir, "init", "-q")
	return err
}
