package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
)

func run(t *testing.T, dir string, args ...string) {
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

func skillMD(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\nbody\n"
}

// sandbox isolates HOME, the cache and git config, and redirects
// https://github.com/ to local repositories under the returned upstream dir.
func sandbox(t *testing.T) (home, upstream string) {
	t.Helper()
	home, upstream = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(home, "cache"))
	t.Setenv("SKILLDOTS_LIBRARY", "")
	gc := filepath.Join(home, "gitconfig")
	write(t, gc, "[user]\n\tname = t\n\temail = t@example.com\n[init]\n\tdefaultBranch = main\n[url \"file://"+upstream+"/\"]\n\tinsteadOf = https://github.com/\n")
	t.Setenv("GIT_CONFIG_GLOBAL", gc)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return home, upstream
}

func newLibrary(t *testing.T, home string) string {
	t.Helper()
	root := filepath.Join(home, "lib")
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
	run(t, root, "init", "-q")
	os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0o755)
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	return root
}

func findPack(s *Snapshot, repo string) *Pack {
	for i := range s.Packs {
		if s.Packs[i].Repo == repo {
			return &s.Packs[i]
		}
	}
	return nil
}

func findSkill(p *Pack, name string) *Skill {
	for i := range p.Skills {
		if p.Skills[i].Name == name {
			return &p.Skills[i]
		}
	}
	return nil
}

func isLink(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

func TestOpsEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home, up := sandbox(t)
	repo := filepath.Join(up, "acme", "skills.git")
	write(t, filepath.Join(repo, "LICENSE"), "MIT License\n\nPermission is hereby granted, free of charge…\n")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), skillMD("tdd", "Test first."))
	write(t, filepath.Join(repo, "skills", "eng", "review", "SKILL.md"), skillMD("review", "Review code."))
	write(t, filepath.Join(repo, "skills", "prod", "grill", "SKILL.md"), skillMD("grill", "Grill me."))
	run(t, repo, "init", "-q")
	run(t, repo, "add", "-A")
	run(t, repo, "commit", "-q", "-m", "init")

	root := newLibrary(t, home)
	write(t, filepath.Join(root, "skills", "mine", "SKILL.md"), skillMD("mine", "My own."))
	o := &libOps{root: root}
	agents := filepath.Join(home, ".agents", "skills")
	claude := filepath.Join(home, ".claude", "skills")

	b, err := o.Browse("acme/skills")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Skills) != 3 || len(b.Have) != 0 {
		t.Fatalf("browse: %+v", b)
	}
	if _, err := o.Add("acme/skills", AddRequest{Names: []string{"tdd"}}); err != nil {
		t.Fatal(err)
	}
	if !isLink(filepath.Join(agents, "tdd")) {
		t.Fatal("add did not link tdd")
	}

	s, err := o.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := findPack(s, "acme/skills")
	if p == nil || p.Selected != 1 || p.Total != 3 || p.License != "MIT" || !p.Upstream || p.Commit == "" || p.When == "" {
		t.Fatalf("snapshot pack: %+v", p)
	}
	if r := findSkill(p, "review"); r == nil || r.Installed || r.Category != "eng" {
		t.Fatalf("upstream-only skill: %+v", r)
	}
	if len(s.Mine) != 1 || s.Mine[0].Name != "mine" || !s.Mine[0].Enabled() {
		t.Fatalf("mine: %+v", s.Mine)
	}
	if s.Branch != "main" || !s.Dirty || s.Remote || s.Pending != 0 {
		t.Fatalf("git state: branch %q dirty %v remote %v pending %d", s.Branch, s.Dirty, s.Remote, s.Pending)
	}
	var present []string
	for _, a := range s.Agents {
		present = append(present, a.Name)
	}
	if strings.Join(present, ",") != "agents,claude" {
		t.Fatalf("present agents: %v", present)
	}

	tdd := *findSkill(p, "tdd")
	if _, err := o.SetTargets(&tdd, "", []string{"agents", "claude"}); err != nil {
		t.Fatal(err)
	}
	if !isLink(filepath.Join(claude, "tdd")) {
		t.Fatal("claude link missing")
	}
	if _, err := o.SetTargets(&tdd, "", []string{}); err != nil {
		t.Fatal(err)
	}
	if isLink(filepath.Join(claude, "tdd")) || isLink(filepath.Join(agents, "tdd")) {
		t.Fatal("disable left links")
	}
	if !dirExists(filepath.Join(root, "vendor", "acme", "skills", "eng", "tdd")) {
		t.Fatal("disable must keep the vendored files")
	}
	if _, err := o.SetTargets(&tdd, "", []string{"agents"}); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Add("acme/skills", AddRequest{Names: []string{"review"}}); err != nil {
		t.Fatal(err)
	}

	// Upstream moves on: tdd changes and a new skill appears.
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), skillMD("tdd", "Test first, always."))
	write(t, filepath.Join(repo, "skills", "eng", "fresh", "SKILL.md"), skillMD("fresh", "New."))
	run(t, repo, "add", "-A")
	run(t, repo, "commit", "-q", "-m", "more")
	reps, msg, err := o.Update([]string{"acme/skills"})
	if err != nil {
		t.Fatal(err)
	}
	if len(reps) != 1 || !slices.Contains(reps[0].Changed, "tdd") || !slices.Equal(reps[0].UpstreamNew, []string{"fresh"}) {
		t.Fatalf("update report (grill existed before, only fresh is new): %+v", reps[0])
	}
	if !strings.Contains(msg, "changed tdd ·") {
		t.Fatalf("update message: %s", msg)
	}

	// d on a vendored skill: deselected, files in the trash.
	s, _ = o.Load()
	review := *findSkill(findPack(s, "acme/skills"), "review")
	if _, err := o.Trash(review); err != nil {
		t.Fatal(err)
	}
	if isLink(filepath.Join(agents, "review")) || dirExists(filepath.Join(root, "vendor", "acme", "skills", "eng", "review")) {
		t.Fatal("review still present after trash")
	}
	if !trashed(t, root, "review") {
		t.Fatal("review not in the trash")
	}
	// d on your own skill: unlinked and safely moved to trash.
	s, _ = o.Load()
	if _, err := o.Trash(s.Mine[0]); err != nil {
		t.Fatal(err)
	}
	if isLink(filepath.Join(agents, "mine")) || dirExists(filepath.Join(root, "skills", "mine")) || !trashed(t, root, "mine") {
		t.Fatal("own skill: want unlinked and in trash")
	}

	// fork tdd into private/.
	s, _ = o.Load()
	tdd = *findSkill(findPack(s, "acme/skills"), "tdd")
	if _, err := o.Fork(tdd, true, ""); err != nil {
		t.Fatal(err)
	}
	if !dirExists(filepath.Join(root, "private", "tdd")) {
		t.Fatal("fork missing")
	}
	s, _ = o.Load()
	if len(s.Private) != 1 || !s.Private[0].Enabled() {
		t.Fatalf("fork not linked: %+v", s.Private)
	}

	// purge the pack.
	if _, err := o.Purge("acme/skills"); err != nil {
		t.Fatal(err)
	}
	s, _ = o.Load()
	if len(s.Packs) != 0 || dirExists(filepath.Join(root, "vendor", "acme")) {
		t.Fatalf("purge left %+v", s.Packs)
	}

	// Follow everything.
	// private/tdd (the fork) already uses the name: engine renames.
	if msg, err := o.Add("acme/skills", AddRequest{All: true}); err != nil || !strings.Contains(msg, "name taken, linked tdd as") {
		t.Fatalf("follow all: %q %v", msg, err)
	}
	s, _ = o.Load()
	if p := findPack(s, "acme/skills"); p == nil || !p.UsesAll || p.Selected != 4 {
		t.Fatalf("follow all: %+v", p)
	}
	if msg, err := o.Apply(); err != nil || msg != "already up to date" {
		t.Fatalf("apply: %q %v", msg, err)
	}
	_ = engine.KindVendor
	_ = library.DirVendor
}

func TestAgo(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	for in, want := range map[int64]string{0: "just now", 59: "just now", 60: "1 minute ago", 7200: "2 hours ago",
		86400 * 3: "3 days ago", 86400 * 65: "2 months ago", 86400 * 800: "2 years ago"} {
		if got := ago(strconv.FormatInt(now.Unix()-in, 10), now); got != want {
			t.Errorf("ago(-%d) = %q, want %q", in, got, want)
		}
	}
	if ago("x", now) != "" {
		t.Error("bad input")
	}
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func trashed(t *testing.T, root, name string) bool {
	t.Helper()
	items, err := (&libOps{root: root}).TrashList()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Name == name {
			return true
		}
	}
	return false
}

// The dashboard checks upstream in the background while the user may
// update, add or reload: the same cache clones, opened from two
// goroutines. Run with -race.
func TestCheckBesideOperations(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home, up := sandbox(t)
	repo := filepath.Join(up, "acme", "skills.git")
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "SKILL.md"), skillMD("tdd", "Test first."))
	write(t, filepath.Join(repo, "skills", "eng", "review", "SKILL.md"), skillMD("review", "Review code."))
	run(t, repo, "init", "-q")
	run(t, repo, "add", "-A")
	run(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init")
	root := newLibrary(t, home)
	o := NewOps(root)
	if _, err := o.Add("acme/skills", AddRequest{Names: []string{"tdd"}}); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(repo, "skills", "eng", "tdd", "more.md"), "more\n")
	run(t, repo, "add", "-A")
	run(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "c2")

	errs := make(chan error, 3)
	go func() { _, err := o.Check(context.Background()); errs <- err }()
	go func() { _, _, err := o.Update(nil); errs <- err }()
	go func() { _, err := o.Load(); errs <- err }()
	for i := 0; i < 3; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	out, err := o.Check(context.Background())
	if err != nil || len(out.Failed) > 0 || out.Updates() != 0 {
		t.Fatalf("after the update: %+v, %v", out, err)
	}
}
