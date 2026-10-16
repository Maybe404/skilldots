// Package library locates a skilldots library on disk and wraps the handful
// of git operations skilldots needs. git is invoked as a subprocess so the
// behaviour matches what the user sees in their own shell.
package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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

// ConfigPath is the small per-user file that remembers where the library is.
func ConfigPath() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "skilldots", "config.toml")
	}
	return filepath.Join(DefaultRoot(), "config.toml")
}

// Remember records root in the user config so later invocations find it
// from any directory.
func Remember(root string) error {
	p := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte("# Written by skd init/clone. The library skd uses when none is given.\nlibrary = \""+filepath.ToSlash(root)+"\"\n"), 0o644)
}

// remembered returns the library recorded by Remember, or "".
func remembered() string {
	b, err := os.ReadFile(ConfigPath())
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "library" {
			return strings.Trim(strings.TrimSpace(v), "\"'")
		}
	}
	return ""
}

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
	candidates = append(candidates, remembered(), DefaultRoot())
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, manifest.FileName)); err == nil {
			return c, nil
		}
		if c == flag {
			return "", fmt.Errorf("no %s in %s", manifest.FileName, c)
		}
	}
	return "", errors.New("no skilldots library found; run `skd init`, `skd clone <url>`, or `skd use <dir>`")
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

// Save writes manifest and lock back.
func (l *Library) Save() error {
	if err := l.Manifest.Save(filepath.Join(l.Root, manifest.FileName)); err != nil {
		return err
	}
	return l.Lock.Save(filepath.Join(l.Root, lock.FileName))
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
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if ctx.Done() != nil {
		cancelTree(cmd)
		cmd.WaitDelay = time.Second
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), ctx.Err())
		}
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
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

// EnsureGit initialises dir as a git repository when it is not one,
// creating dir first if needed.
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
