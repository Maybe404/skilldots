package app_test

// The CLI and the dashboard both run their writes through package app.
// This test drives the same steps through each front end on identical
// fixtures and checks they leave the same manifest, lock, files and links.

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/cli"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/tui"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillMD(name string) string {
	return "---\nname: " + name + "\ndescription: " + name + " skill\n---\nbody\n"
}

// upstream builds one fake GitHub with acme/skills, shared by both sides
// so commits are identical.
func upstream(t *testing.T) string {
	up := t.TempDir()
	repo := filepath.Join(up, "acme", "skills.git")
	write(t, filepath.Join(repo, "LICENSE"), "MIT License\n")
	for _, s := range []string{"eng/tdd", "eng/review", "prod/grill", "prod/plan"} {
		write(t, filepath.Join(repo, "skills", s, "SKILL.md"), skillMD(filepath.Base(s)))
	}
	git(t, repo, "init", "-q")
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init")
	return up
}

// side isolates HOME, cache and git config for one front end and creates
// an empty library.
func side(t *testing.T, up string) (home, root string) {
	home = t.TempDir()
	if r, err := filepath.EvalSymlinks(home); err == nil {
		home = r
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(home, "cache"))
	t.Setenv("SKILLDOTS_LIBRARY", "")
	gc := filepath.Join(home, "gitconfig")
	write(t, gc, "[user]\n\tname = t\n\temail = t@example.com\n[init]\n\tdefaultBranch = main\n[url \"file://"+up+"/\"]\n\tinsteadOf = https://github.com/\n")
	t.Setenv("GIT_CONFIG_GLOBAL", gc)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root = filepath.Join(home, "lib")
	for _, d := range []string{"skills", "vendor", "private"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	if err := manifest.New([]string{"agents"}).Save(filepath.Join(root, manifest.FileName)); err != nil {
		t.Fatal(err)
	}
	l := &lock.Lock{Version: 1, Packs: map[string]*lock.Pack{}, Links: map[string]*lock.Link{}}
	if err := l.Save(filepath.Join(root, lock.FileName)); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q")
	os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0o755)
	os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755)
	return home, root
}

// state is everything an operation can change, with the side's home
// replaced by ~ so both sides compare equal.
func state(t *testing.T, home, root string) map[string]string {
	t.Helper()
	norm := func(s string) string { return strings.ReplaceAll(s, home, "~") }
	out := map[string]string{}
	for _, f := range []string{manifest.FileName, lock.FileName} {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		out[f] = norm(string(b))
	}
	for _, dir := range []string{".agents/skills", ".claude/skills"} {
		entries, _ := os.ReadDir(filepath.Join(home, dir))
		for _, e := range entries {
			to, err := os.Readlink(filepath.Join(home, dir, e.Name()))
			if err != nil {
				to = "not a link"
			}
			out["link "+dir+"/"+e.Name()] = norm(to)
		}
	}
	var files []string
	for _, d := range []string{"skills", "private", "vendor"} {
		filepath.Walk(filepath.Join(root, d), func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				files = append(files, norm(p))
			}
			return nil
		})
	}
	sort.Strings(files)
	out["files"] = strings.Join(files, "\n")
	// What went to the trash, by origin (ids carry the time).
	var origins []string
	entries, _ := os.ReadDir(filepath.Join(root, engine.TrashDir))
	for _, b := range entries {
		items, _ := os.ReadDir(filepath.Join(root, engine.TrashDir, b.Name()))
		for _, it := range items {
			if strings.HasSuffix(it.Name(), ".origin") {
				o, _ := os.ReadFile(filepath.Join(root, engine.TrashDir, b.Name(), it.Name()))
				origins = append(origins, norm(strings.TrimSpace(string(o))))
			}
		}
	}
	sort.Strings(origins)
	out["trash"] = strings.Join(origins, "\n")
	return out
}

func runCLI(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := cli.Root()
	cmd.SetArgs(append([]string{"--library", root}, args...))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("skd %v: %v", args, err)
	}
}

func find(t *testing.T, o tui.Ops, kind engine.Kind, name string) tui.Skill {
	t.Helper()
	s, err := o.Load()
	if err != nil {
		t.Fatal(err)
	}
	var all []tui.Skill
	for _, p := range s.Packs {
		all = append(all, p.Skills...)
	}
	all = append(append(append(all, s.Mine...), s.Private...), s.Apps...)
	for _, sk := range all {
		if sk.Kind == kind && sk.Name == name && sk.Installed {
			return sk
		}
	}
	t.Fatalf("no %s %s in the dashboard", kind, name)
	return tui.Skill{}
}

func TestCLIAndDashboardAgree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := upstream(t)
	type step struct {
		name string
		cli  []string
		tui  func(o tui.Ops) error
	}
	steps := []step{
		{"add", []string{"add", "acme/skills", "--skill", "tdd,review,grill", "--alias", "acme", "--agent", "agents"},
			func(o tui.Ops) error {
				_, err := o.Add("acme/skills", tui.AddRequest{Names: []string{"tdd", "review", "grill"}, Alias: "acme", Agents: []string{"agents"}})
				return err
			}},
		{"add by category", []string{"add", "acme", "--category", "prod"},
			func(o tui.Ops) error {
				_, err := o.Add("acme", tui.AddRequest{Names: []string{"grill", "plan"}})
				return err
			}},
		{"enable for claude", []string{"enable", "tdd", "--agent", "claude"},
			func(o tui.Ops) error {
				sk := find(t, o, engine.KindVendor, "tdd")
				_, err := o.SetTargets(&sk, "", append(sk.Agents, "claude"))
				return err
			}},
		{"disable", []string{"disable", "review"},
			func(o tui.Ops) error { _, err := o.Disable(find(t, o, engine.KindVendor, "review")); return err }},
		{"enable", []string{"enable", "review"},
			func(o tui.Ops) error { _, err := o.Enable(find(t, o, engine.KindVendor, "review")); return err }},
		{"remove", []string{"remove", "acme:grill"},
			func(o tui.Ops) error { _, err := o.Trash(find(t, o, engine.KindVendor, "grill")); return err }},
		{"fork as", []string{"fork", "tdd", "--as", "my-tdd"},
			func(o tui.Ops) error {
				_, err := o.Fork(find(t, o, engine.KindVendor, "tdd"), false, "my-tdd")
				return err
			}},
		{"fork private", []string{"fork", "plan", "--private"},
			func(o tui.Ops) error { _, err := o.Fork(find(t, o, engine.KindVendor, "plan"), true, ""); return err }},
		{"new", []string{"new", "notes", "--private"},
			func(o tui.Ops) error { _, err := o.New("notes", true); return err }},
		{"disable own", []string{"disable", "skills:my-tdd"},
			func(o tui.Ops) error { _, err := o.Trash(find(t, o, engine.KindSkill, "my-tdd")); return err }},
		{"remove last skill drops the pack", []string{"remove", "review"},
			func(o tui.Ops) error { _, err := o.Trash(find(t, o, engine.KindVendor, "review")); return err }},
	}

	// One step at a time on both sides, so a difference names its step.
	cliHome, cliRoot := side(t, up)
	var cliStates []map[string]string
	for _, s := range steps {
		runCLI(t, cliRoot, s.cli...)
		cliStates = append(cliStates, state(t, cliHome, cliRoot))
	}
	tuiHome, tuiRoot := side(t, up)
	o := tui.NewOps(tuiRoot)
	for i, s := range steps {
		if err := s.tui(o); err != nil {
			t.Fatalf("dashboard %s: %v", s.name, err)
		}
		got := state(t, tuiHome, tuiRoot)
		for k, want := range cliStates[i] {
			if got[k] != want {
				t.Errorf("after %q, %s differs:\ncli:\n%s\ndashboard:\n%s", s.name, k, want, got[k])
			}
		}
		for k := range got {
			if _, ok := cliStates[i][k]; !ok {
				t.Errorf("after %q, the dashboard has %s = %s and the CLI does not", s.name, k, got[k])
			}
		}
	}
}
