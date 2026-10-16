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

// skdBin is built with the rawoutput tag, skdGuarded as people get it.
var skdBin, skdGuarded string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "skd-e2e")
	if err != nil {
		panic(err)
	}
	skdBin = filepath.Join(dir, "skd")
	// rawoutput: without the screen writer that cleans whatever reaches
	// the terminal, so the tests see what each command itself prints
	// (TestHostileLibraryPrintsClean).
	out, err := exec.Command("go", "build", "-tags", "rawoutput", "-o", skdBin, "github.com/Maybe404/skilldots/cmd/skd").CombinedOutput()
	if err != nil {
		panic(string(out))
	}
	skdGuarded = filepath.Join(dir, "skd-guarded")
	if out, err := exec.Command("go", "build", "-o", skdGuarded, "github.com/Maybe404/skilldots/cmd/skd").CombinedOutput(); err != nil {
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
	return e.run(skdBin, dir, args...)
}

// run runs the skd binary bin like skd does.
func (e *env) run(bin, dir string, args ...string) (stdout, stderr string, code int) {
	e.t.Helper()
	if dir == "" {
		dir = e.home
	}
	cmd := exec.Command(bin, args...)
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

// The URL given to skd clone is a repository to git even when it starts
// with "-" (a relative path here): never an option.
func TestCloneURLIsNeverAnOption(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.git(lib, "add", "-A")
	e.git(lib, "commit", "-qm", "x")
	e.git(e.home, "clone", "-q", "--bare", "--", lib, "-backup.git")
	if out, errOut, code := e.skd("", "clone", "--", "-backup.git", e.p("lib2")); code != 0 {
		t.Fatalf("clone -backup.git: exit %d\n%s\n%s", code, out, errOut)
	}
	if _, err := os.Stat(e.p("lib2/skills.toml")); err != nil {
		t.Fatal(err)
	}
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

// adopt --yes forces only the entries it adopts; an unrelated entry that
// was modified since skilldots wrote it stays untouched.
func TestAdoptForcesOnlyItsOwnEntries(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.ok("", "new", "mine")
	// The user replaced skilldots' link with a directory of their own.
	os.Remove(e.p(".agents/skills/mine"))
	e.skill(".agents/skills/mine", "mine")
	e.write(".agents/skills/mine/notes.txt", "keep me\n")
	// Something installed by another tool: a copy, and a link to a clone.
	e.skill(".agents/skills/other", "other")
	e.skill("elsewhere/linked", "linked")
	os.Symlink(e.p("elsewhere/linked"), e.p(".agents/skills/linked"))

	e.ok("", "adopt", "--yes")
	if b, err := os.ReadFile(e.p(".agents/skills/mine/notes.txt")); err != nil || string(b) != "keep me\n" {
		t.Fatal("adopt --yes touched an unrelated modified entry")
	}
	isLink(t, e.p(".agents/skills/other"), lib)
	isLink(t, e.p(".agents/skills/linked"), lib)
	if _, err := os.Stat(filepath.Join(lib, "skills", "linked", "SKILL.md")); err != nil {
		t.Fatal("linked skill was not copied from its link target")
	}
}

// A copy whose directory name differs from its frontmatter name is adopted
// under the frontmatter name; the agent dir ends up with one link only.
func TestAdoptDirNameDiffersFromSkillName(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.skill(".agents/skills/dirx", "namey")
	e.write(".agents/skills/dirx/notes.txt", "mine\n")
	out := e.ok("", "adopt", "--yes")
	if n := strings.Count(out, "moved aside"); n != 1 {
		t.Errorf("\"moved aside\" printed %d times, want once:\n%s", n, out)
	}
	entries, _ := os.ReadDir(e.p(".agents/skills"))
	if len(entries) != 1 || entries[0].Name() != "namey" {
		var names []string
		for _, x := range entries {
			names = append(names, x.Name())
		}
		t.Fatalf("agent dir has %v, want only namey", names)
	}
	isLink(t, e.p(".agents/skills/namey"), lib)
	if m, _ := filepath.Glob(filepath.Join(lib, ".trash", "*", "dirx", "notes.txt")); len(m) != 1 {
		t.Fatal("original directory not in the trash")
	}
}

// adopt never leaves the user's only copy of something in the trash: a
// directory whose content differs from what the library already holds
// under that name stays in place.
func TestAdoptKeepsDifferingDirectory(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.ok("", "new", "namey")
	e.skill(".agents/skills/dirx", "namey")
	e.write(".agents/skills/dirx/unique.txt", "only here\n")
	// Same name, same content as the library would be fine; so would a
	// .git that the library copy lacks — that also stays.
	e.skill(".claude/skills/withgit", "withgit")
	e.write(".claude/skills/withgit/.git/HEAD", "ref\n")
	out, errOut, code := e.skd("", "adopt", "--yes")
	if code != 0 {
		t.Fatalf("adopt: %d\n%s\n%s", code, out, errOut)
	}
	if b, err := os.ReadFile(e.p(".agents/skills/dirx/unique.txt")); err != nil || string(b) != "only here\n" {
		t.Fatal("differing directory was moved away")
	}
	if !strings.Contains(out+errOut, "differs from the library") {
		t.Fatalf("no warning:\n%s%s", out, errOut)
	}
	if _, err := os.Stat(e.p(".claude/skills/withgit/.git/HEAD")); err != nil {
		t.Fatal("directory with .git was replaced although the library copy lacks .git")
	}
	isLink(t, e.p(".agents/skills/namey"), lib)
}

// Two skills named foo for disjoint agents: a bare name is ambiguous and
// must not silently pick one; a qualified name picks exactly one.
func TestAmbiguousNameNeedsQualifier(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.skill("up/skills/foo", "foo")
	e.skill("up/skills/other", "other")
	up := e.repo("up")
	e.ok("", "add", up, "--all", "--agent", "agents", "--alias", "upa")
	e.ok("", "new", "foo")
	f, _ := os.OpenFile(filepath.Join(lib, "skills.toml"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("\n[[local]]\nname = \"foo\"\ntargets = [\"claude\"]\n")
	f.Close()
	e.ok("", "apply")

	for _, args := range [][]string{{"path", "foo"}, {"disable", "foo", "--agent", "claude"}, {"fork", "foo"}, {"remove", "foo"}} {
		out, errOut, code := e.skd("", args...)
		if code == 0 || !strings.Contains(errOut, up+":foo") || !strings.Contains(errOut, "skills:foo") {
			t.Fatalf("skd %v: want an ambiguity error listing both, got %d\n%s%s", args, code, out, errOut)
		}
	}
	if p := strings.TrimSpace(e.ok("", "path", "skills:foo")); p != filepath.Join(lib, "skills", "foo") {
		t.Fatalf("path skills:foo = %s", p)
	}
	if p := strings.TrimSpace(e.ok("", "path", "upa:foo")); !strings.HasPrefix(p, filepath.Join(lib, "vendor")) {
		t.Fatalf("path upa:foo = %s", p)
	}
	e.ok("", "disable", "skills:foo", "--agent", "claude")
	if _, err := os.Lstat(e.p(".claude/skills/foo")); !os.IsNotExist(err) {
		t.Fatal("skills:foo still linked into claude")
	}
	isLink(t, e.p(".agents/skills/foo"), filepath.Join(lib, "vendor"))
	// Unambiguous names behave as before.
	e.ok("", "path", "other")
	e.ok("", "remove", up+":foo")
	if _, err := os.Lstat(e.p(".agents/skills/foo")); !os.IsNotExist(err) {
		t.Fatal("vendored foo still linked after remove")
	}
}

// A pack alias may not hijack the reserved qualifiers.
func TestReservedQualifiers(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.skill("up/skills/foo", "foo")
	up := e.repo("up")
	for _, a := range []string{"skills", "private", "app", "vendor"} {
		if _, errOut, code := e.skd("", "add", up, "--all", "--alias", a); code == 0 || !strings.Contains(errOut, "reserved") {
			t.Fatalf("--alias %s accepted: %s", a, errOut)
		}
	}
	e.ok("", "add", up, "--all", "--agent", "agents")
	// An alias "skills" written into skills.toml by hand.
	b, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
	os.WriteFile(filepath.Join(lib, "skills.toml"), []byte(strings.Replace(string(b), "repo = ", "alias = 'skills'\nrepo = ", 1)), 0o644)
	e.ok("", "new", "foo")
	f, _ := os.OpenFile(filepath.Join(lib, "skills.toml"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("\n[[local]]\nname = \"foo\"\ntargets = [\"claude\"]\n")
	f.Close()
	if p := strings.TrimSpace(e.ok("", "path", "skills:foo")); p != filepath.Join(lib, "skills", "foo") {
		t.Fatalf("skills:foo = %s, want the local skill", p)
	}
}

// With two library skills named foo (vendored for agents, own for claude),
// adopt must compare against and enable the right one, or neither.
func TestAdoptWithSameNamedEntries(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.skill("up/skills/foo", "foo")
	e.write("up/skills/foo/vendor-only.md", "v\n")
	up := e.repo("up")
	e.ok("", "add", up, "--all", "--agent", "agents")
	e.ok("", "new", "foo")
	f, _ := os.OpenFile(filepath.Join(lib, "skills.toml"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("\n[[local]]\nname = \"foo\"\ntargets = [\"claude\"]\n")
	f.Close()
	e.ok("", "apply")
	ownSkill, _ := os.ReadFile(filepath.Join(lib, "skills", "foo", "SKILL.md"))

	// codex has an unmanaged copy named foo: ambiguous, so it is left alone.
	e.write(".codex/skills/foo/SKILL.md", string(ownSkill))
	e.write(".codex/skills/foo/mine.txt", "x\n")
	// gemini has dirx whose frontmatter says foo and whose content is the
	// user's own skills/foo: it is adopted as skills:foo, not the vendor one.
	e.write(".gemini/skills/dirx/SKILL.md", string(ownSkill))
	e.ok("", "adopt", "--yes")

	if b, err := os.ReadFile(e.p(".codex/skills/foo/mine.txt")); err != nil || string(b) != "x\n" {
		t.Fatal("ambiguous codex/foo was replaced")
	}
	isLink(t, e.p(".gemini/skills/foo"), filepath.Join(lib, "skills", "foo"))
	if _, err := os.Lstat(e.p(".gemini/skills/dirx")); !os.IsNotExist(err) {
		t.Fatal("gemini/dirx still there after adoption")
	}
	isLink(t, e.p(".agents/skills/foo"), filepath.Join(lib, "vendor"))
	if b, _ := os.ReadFile(filepath.Join(lib, "skills.toml")); strings.Count(string(b), "gemini") != 1 {
		t.Fatalf("gemini should be added to exactly one entry:\n%s", b)
	}
}

// Temporary files of an interrupted save never reach the backup: a new
// library ignores them, `init --from` adds the pattern to an existing
// .gitignore, and sync leaves them out even when .gitignore predates it.
func TestSyncNeverCommitsSaveTemporaries(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	if b, _ := os.ReadFile(filepath.Join(lib, ".gitignore")); !strings.Contains(string(b), ".*.tmp-*") {
		t.Errorf("new .gitignore lacks .*.tmp-*:\n%s", b)
	}

	e.write("old/.gitignore", "/private/\n/.trash/\n")
	e.write("old/notes.txt", "mine\n")
	old := e.p("old")
	e.ok("", "init", "--from", old, "--no-skill")
	b, _ := os.ReadFile(filepath.Join(old, ".gitignore"))
	if !strings.Contains(string(b), ".*.tmp-*") || strings.Count(string(b), "/private/") != 1 {
		t.Errorf("init --from .gitignore:\n%s", b)
	}

	// A library whose .gitignore was written before the pattern existed.
	e.write("lib/.gitignore", "/private/\n/.trash/\n")
	e.write("lib/.skills.toml.tmp-123", "torn")
	e.write("lib/.skills.lock.tmp-456", "torn")
	e.skill("lib/skills/mine", "mine")
	e.ok("", "--library", lib, "sync")
	files := e.git(lib, "ls-files")
	if strings.Contains(files, ".tmp-") || !strings.Contains(files, "skills/mine/SKILL.md") {
		t.Fatalf("tracked files:\n%s", files)
	}
}

// --alias may not look like a repo or a qualified key, nor spell a
// subscribed repo; it is rejected before anything is fetched.
func TestAddRejectsAliasThatShadows(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	before, _ := os.ReadFile(filepath.Join(lib, "skills.toml"))
	for _, a := range []string{"acme/skills", "x:y", "a/b/c"} {
		_, errOut, code := e.skd("", "--library", lib, "add", "nobody/nothing", "--alias", a)
		if code == 0 || !strings.Contains(errOut, "--alias") {
			t.Errorf("--alias %q: exit %d, %s", a, code, errOut)
		}
	}
	if after, _ := os.ReadFile(filepath.Join(lib, "skills.toml")); string(after) != string(before) {
		t.Errorf("skills.toml changed:\n%s", after)
	}
}

// A second pack cannot take an alias another pack already has; the
// check runs before anything is fetched.
func TestAddRejectsDuplicateAlias(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	toml := filepath.Join(lib, "skills.toml")
	b, _ := os.ReadFile(toml)
	os.WriteFile(toml, append(b, []byte("\n[[pack]]\nrepo = 'acme/one'\nalias = 'acme'\n")...), 0o644)
	before, _ := os.ReadFile(toml)
	_, errOut, code := e.skd("", "--library", lib, "add", "acme/two", "--alias", "Acme")
	if code == 0 || !strings.Contains(errOut, "already the alias of acme/one") {
		t.Errorf("duplicate alias: exit %d, %s", code, errOut)
	}
	if after, _ := os.ReadFile(toml); string(after) != string(before) {
		t.Errorf("skills.toml changed:\n%s", after)
	}
}
