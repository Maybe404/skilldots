package cli_test

// End-to-end tests: build the real skd binary and run it with HOME, the
// cache and the config dir pointed at temporary directories.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var skdBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "skd-e2e")
	if err != nil {
		panic(err)
	}
	skdBin = filepath.Join(dir, "skd")
	out, err := exec.Command("go", "build", "-o", skdBin, "github.com/Maybe404/skilldots/cmd/skd").CombinedOutput()
	if err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type env struct {
	t    *testing.T
	home string
	vars []string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	home := t.TempDir()
	if r, err := filepath.EvalSymlinks(home); err == nil {
		home = r
	}
	e := &env{t: t, home: home}
	e.vars = []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"SKILLDOTS_CACHE=" + filepath.Join(home, "cache"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "NO_COLOR=1", "TERM=dumb",
	}
	return e
}

func (e *env) p(rel string) string { return filepath.Join(e.home, filepath.FromSlash(rel)) }

// skd runs the binary in dir (home when ""), with stdin not a terminal.
func (e *env) skd(dir string, args ...string) (stdout, stderr string, code int) {
	e.t.Helper()
	if dir == "" {
		dir = e.home
	}
	cmd := exec.Command(skdBin, args...)
	cmd.Dir = dir
	cmd.Env = e.vars
	var o, er bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &er
	err := cmd.Run()
	code = 0
	if x, ok := err.(*exec.ExitError); ok {
		code = x.ExitCode()
	} else if err != nil {
		e.t.Fatal(err)
	}
	return o.String(), er.String(), code
}

// ok runs skd and fails the test on a non-zero exit.
func (e *env) ok(dir string, args ...string) string {
	e.t.Helper()
	out, errOut, code := e.skd(dir, args...)
	if code != 0 {
		e.t.Fatalf("skd %v: exit %d\nstdout: %s\nstderr: %s", args, code, out, errOut)
	}
	return out
}

func (e *env) git(dir string, args ...string) string {
	e.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = e.vars
	out, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (e *env) write(rel, content string) {
	e.t.Helper()
	p := e.p(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) skill(rel, name string) {
	e.t.Helper()
	e.write(rel+"/SKILL.md", "---\nname: "+name+"\ndescription: d\n---\nbody\n")
}

// repo makes rel a git repository with everything committed.
func (e *env) repo(rel string) string {
	e.t.Helper()
	d := e.p(rel)
	e.git(d, "init", "-q")
	e.git(d, "add", "-A")
	e.git(d, "commit", "-qm", "init")
	return d
}

func isLink(t *testing.T, p, wantPrefix string) {
	t.Helper()
	dest, err := os.Readlink(p)
	if err != nil {
		t.Fatalf("%s is not a link: %v", p, err)
	}
	if !strings.HasPrefix(dest, wantPrefix) {
		t.Fatalf("%s → %s, want under %s", p, dest, wantPrefix)
	}
}

func TestCloneFromBareRepo(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.skill("lib/skills/mine", "mine")
	e.git(lib, "add", "-A")
	e.git(lib, "commit", "-qm", "x")
	e.git(e.home, "clone", "-q", "--bare", lib, e.p("backup.git"))
	os.RemoveAll(e.p(".agents"))

	e.ok("", "clone", e.p("backup.git"), "lib2")
	priv := e.p("lib2/private")
	if st, err := os.Stat(priv); err != nil || !st.IsDir() {
		t.Fatalf("private/ missing: %v", err)
	}
	if top := e.git(priv, "rev-parse", "--show-toplevel"); top != priv {
		t.Fatalf("private/ is not its own repository (toplevel %s)", top)
	}
	isLink(t, e.p(".agents/skills/mine"), e.p("lib2"))
}

func TestInitInsideOuterRepoAndOtherLibrary(t *testing.T) {
	e := newEnv(t)
	// A larger repository the library lives in.
	e.write("outer/README", "outer\n")
	outer := e.repo("outer")
	lib := filepath.Join(outer, "lib")
	e.ok(outer, "init", lib, "--no-skill")
	if top := e.git(lib, "rev-parse", "--show-toplevel"); top != lib {
		t.Fatalf("library has no repository of its own (toplevel %s)", top)
	}
	if top := e.git(filepath.Join(lib, "private"), "rev-parse", "--show-toplevel"); top != filepath.Join(lib, "private") {
		t.Fatalf("private/ has no repository of its own (toplevel %s)", top)
	}
	e.write("outer/README", "changed\n")
	e.write("outer/untracked.txt", "x\n")
	e.git(outer, "add", "README")
	e.skill("outer/lib/skills/mine", "mine")
	e.ok("", "--library", lib, "sync")
	if st := e.git(outer, "status", "--porcelain", "--", "README", "untracked.txt"); !strings.Contains(st, "M  README") || !strings.Contains(st, "?? untracked.txt") {
		t.Fatalf("outer repository was touched:\n%s", st)
	}
	if files := e.git(lib, "show", "--name-only", "--format=", "HEAD"); strings.Contains(files, "README") || !strings.Contains(files, "skills/mine/SKILL.md") {
		t.Fatalf("library commit has the wrong files:\n%s", files)
	}

	// init <dir> from inside another library installs into the new one.
	// The skilldots skill comes from a fake "GitHub" seeded into the cache.
	e.skill("selfsrc/skills/skilldots", "skilldots")
	self := e.repo("selfsrc")
	e.git(e.home, "clone", "-q", self, e.p("cache/Maybe404__skilldots"))
	before := e.git(lib, "status", "--porcelain")
	beforeToml, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
	inner := filepath.Join(lib, "skills", "nested-lib")
	e.ok(lib, "init", inner)
	afterToml, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
	if string(beforeToml) != string(afterToml) {
		t.Fatalf("init wrote into the surrounding library:\n%s", afterToml)
	}
	if b, _ := os.ReadFile(filepath.Join(inner, "skills.toml")); !strings.Contains(string(b), "Maybe404/skilldots") {
		t.Fatalf("new library did not get the skilldots pack:\n%s", b)
	}
	if _, err := os.Stat(filepath.Join(lib, "vendor", "Maybe404")); err == nil {
		t.Fatal("surrounding library got a vendor/Maybe404")
	}
	_ = before
}
