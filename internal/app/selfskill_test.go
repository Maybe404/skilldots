package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// offline isolates HOME, the settings and git, with GitHub leading to an
// empty directory: any fetch of Maybe404/skilldots fails. It returns home.
func offline(t *testing.T) string {
	t.Helper()
	home, _ := filepath.EvalSymlinks(t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(home, "cache"))
	t.Setenv("SKILLDOTS_LIBRARY", "")
	nowhere := filepath.Join(home, "nowhere")
	os.MkdirAll(nowhere, 0o755)
	gitconfig(t, home, nowhere)
	return home
}

// gitconfig makes https://github.com/ lead to dir.
func gitconfig(t *testing.T, home, dir string) {
	t.Helper()
	gc := filepath.Join(home, "gitconfig")
	write(t, gc, "[user]\n\tname = t\n\temail = t@example.com\n[init]\n\tdefaultBranch = main\n[url \"file://"+dir+"/\"]\n\tinsteadOf = https://github.com/\n")
	t.Setenv("GIT_CONFIG_GLOBAL", gc)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// bundle makes <dir>/skilldots.app with the skilldots skill in its
// resources and returns the skill's directory.
func bundle(t *testing.T, dir string) string {
	t.Helper()
	skill := filepath.Join(dir, "skilldots.app", "Contents", "Resources", "skills", "skilldots")
	write(t, filepath.Join(skill, "SKILL.md"), skillMD("skilldots"))
	return skill
}

// useBundle makes app.BundledSelfSkill return dir for the test.
func useBundle(t *testing.T, dir string) {
	t.Helper()
	prev := app.BundledSelfSkill
	app.BundledSelfSkill = func() string { return dir }
	t.Cleanup(func() { app.BundledSelfSkill = prev })
}

func readlink(t *testing.T, path string) string {
	t.Helper()
	got, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// Run by the app, a new library links the skilldots skill shipped in the
// bundle with an [[app]] entry: no network, the defaults and every agent.
func TestInstallSelfSkillFromBundleOffline(t *testing.T) {
	home := offline(t)
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	skill := bundle(t, filepath.Join(home, "Applications"))
	useBundle(t, skill)

	root := filepath.Join(home, "lib")
	if err := app.InitLibrary(root, nil); err != nil {
		t.Fatal(err)
	}
	s, _ := app.Open(root)
	if s.HasSelfSkill() {
		t.Fatal("a new library already has the skilldots skill")
	}
	if _, _, err := s.InstallSelfSkill(); err != nil {
		t.Fatalf("offline install from the bundle: %v", err)
	}
	s, _ = app.Open(root)
	if !s.HasSelfSkill() {
		t.Fatal("HasSelfSkill does not count the [[app]] entry")
	}
	if len(s.Lib.Manifest.Packs) != 0 || len(s.Lib.Manifest.Apps) != 1 {
		t.Fatalf("want one [[app]] and no pack, got %d packs, %d apps", len(s.Lib.Manifest.Packs), len(s.Lib.Manifest.Apps))
	}
	a := s.Lib.Manifest.Apps[0]
	if a.Name != "skilldots" || a.Path != "~/Applications/skilldots.app/Contents/Resources/skills/skilldots" {
		t.Fatalf("[[app]] = %+v; want skilldots at ~/Applications/…", a)
	}
	if a.Targets == nil || !slices.Equal(*a.Targets, []string{"agents", "claude", "*"}) {
		t.Fatalf("[[app]] targets = %v, want [agents claude *]", a.Targets)
	}
	for _, dir := range []string{".agents/skills", ".claude/skills"} {
		if got := readlink(t, filepath.Join(home, dir, "skilldots")); got != skill {
			t.Fatalf("~/%s/skilldots → %s, want %s", dir, got, skill)
		}
	}
}

// In development (no bundle) skd keeps the GitHub source: offline, it
// fails as before and writes no [[app]] entry.
func TestInstallSelfSkillWithoutBundleUsesGitHub(t *testing.T) {
	home := offline(t)
	useBundle(t, "")
	root := filepath.Join(home, "lib")
	if err := app.InitLibrary(root, nil); err != nil {
		t.Fatal(err)
	}
	s, _ := app.Open(root)
	rep, _, err := s.InstallSelfSkill()
	if err == nil || rep != nil {
		t.Fatalf("without a bundle and without GitHub the install succeeded (report %v, err %v)", rep, err)
	}
	s, _ = app.Open(root)
	if len(s.Lib.Manifest.Apps) != 0 {
		t.Fatalf("an [[app]] entry was written without a bundle: %+v", s.Lib.Manifest.Apps[0])
	}
}

// A library that already has a skilldots skill (the GitHub source, a fork,
// another [[app]]) gets no second one; one subscribed to SelfRepo keeps
// that source even when the app has its own copy.
func TestBundledSelfSkillDoesNotDuplicate(t *testing.T) {
	up := t.TempDir()
	repo := filepath.Join(up, "Maybe404", "skilldots.git")
	write(t, filepath.Join(repo, "LICENSE"), "MIT License\n")
	write(t, filepath.Join(repo, "skills", "skilldots", "SKILL.md"), skillMD("skilldots"))
	git(t, repo, "init", "-q")
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init")

	home := offline(t)
	gitconfig(t, home, up)
	skill := bundle(t, filepath.Join(home, "Applications"))

	// Subscribed from GitHub first (no bundle), then the app runs.
	useBundle(t, "")
	root := filepath.Join(home, "lib")
	if err := app.InitLibrary(root, nil); err != nil {
		t.Fatal(err)
	}
	s, _ := app.Open(root)
	if _, _, err := s.InstallSelfSkill(); err != nil {
		t.Fatal(err)
	}
	useBundle(t, skill)
	s, _ = app.Open(root)
	if !s.HasSelfSkill() {
		t.Fatal("the GitHub source does not count")
	}
	// The pack without the skill: installing adds it there, not an [[app]].
	s.Lib.Manifest.FindPack(app.SelfRepo).Use = []string{}
	if err := s.Lib.Manifest.Save(filepath.Join(root, "skills.toml")); err != nil {
		t.Fatal(err)
	}
	s, _ = app.Open(root)
	if s.HasSelfSkill() {
		t.Fatal("HasSelfSkill with use = []")
	}
	if _, _, err := s.InstallSelfSkill(); err != nil {
		t.Fatal(err)
	}
	s, _ = app.Open(root)
	if len(s.Lib.Manifest.Apps) != 0 || !s.HasSelfSkill() {
		t.Fatalf("a library subscribed to %s got an [[app]] entry (%d) or no skill", app.SelfRepo, len(s.Lib.Manifest.Apps))
	}

	// A fork in skills/ counts, and so does an [[app]] skilldots that is
	// missing on this machine (a second one would clash with it by name).
	root2 := filepath.Join(home, "lib2")
	if err := app.InitLibrary(root2, nil); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root2, "skills", "skilldots", "SKILL.md"), skillMD("skilldots"))
	s, _ = app.Open(root2)
	if !s.HasSelfSkill() {
		t.Fatal("a fork does not count")
	}
	os.RemoveAll(filepath.Join(root2, "skills", "skilldots"))
	s, _ = app.Open(root2)
	s.Lib.Manifest.Apps = append(s.Lib.Manifest.Apps, &manifest.App{Name: "skilldots", Path: "/Applications/gone.app/Contents/Resources/skills/skilldots"})
	if err := s.Lib.Manifest.Save(filepath.Join(root2, "skills.toml")); err != nil {
		t.Fatal(err)
	}
	s, _ = app.Open(root2)
	if !s.HasSelfSkill() {
		t.Fatal("an [[app]] skilldots missing on this machine does not count")
	}
}

// The app moved (or the library came from a Mac that keeps it elsewhere):
// the [[app]] entry and the agents' links follow the running app's copy.
func TestRepointAppSelfSkill(t *testing.T) {
	home := offline(t)
	old := bundle(t, filepath.Join(home, "Applications"))
	useBundle(t, old)
	root := filepath.Join(home, "lib")
	if err := app.InitLibrary(root, nil); err != nil {
		t.Fatal(err)
	}
	s, _ := app.Open(root)
	if _, _, err := s.InstallSelfSkill(); err != nil {
		t.Fatal(err)
	}

	// Already this app's: nothing to do.
	s, _ = app.Open(root)
	if changed, _, err := s.RepointAppSelfSkill(old); changed || err != nil {
		t.Fatalf("repointing at the same copy: changed %v, err %v", changed, err)
	}

	// Moved out of the home folder: the path is absolute, the link follows.
	apps, _ := filepath.EvalSymlinks(t.TempDir())
	if err := os.Rename(filepath.Join(home, "Applications", "skilldots.app"), filepath.Join(apps, "skilldots.app")); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(apps, "skilldots.app", "Contents", "Resources", "skills", "skilldots")
	s, _ = app.Open(root)
	changed, out, err := s.RepointAppSelfSkill(moved)
	if err != nil || !changed || out == nil || !out.Applied {
		t.Fatalf("repoint after a move: changed %v, outcome %+v, err %v", changed, out, err)
	}
	s, _ = app.Open(root)
	if p := s.Lib.Manifest.Apps[0].Path; p != moved {
		t.Fatalf("[[app]] path = %s, want %s", p, moved)
	}
	for _, dir := range []string{".agents/skills", ".claude/skills"} {
		if _, err := os.Lstat(filepath.Join(home, dir)); os.IsNotExist(err) {
			continue
		}
		if got := readlink(t, filepath.Join(home, dir, "skilldots")); got != moved {
			t.Fatalf("~/%s/skilldots → %s, want %s", dir, got, moved)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "skilldots", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	// Another change waiting to be applied: the entry is rewritten, the
	// links wait for the person's apply with the rest.
	back := bundle(t, filepath.Join(home, "Applications"))
	write(t, filepath.Join(root, "skills", "mine", "SKILL.md"), skillMD("mine"))
	s, _ = app.Open(root)
	changed, out, err = s.RepointAppSelfSkill(back)
	if err != nil || !changed || out != nil {
		t.Fatalf("repoint with other pending changes: changed %v, outcome %+v, err %v", changed, out, err)
	}
	if got := readlink(t, filepath.Join(home, ".agents", "skills", "skilldots")); got != moved {
		t.Fatalf("the link changed before apply: → %s", got)
	}
	s, _ = app.Open(root)
	if p := s.Lib.Manifest.Apps[0].Path; p != "~/Applications/skilldots.app/Contents/Resources/skills/skilldots" {
		t.Fatalf("[[app]] path = %s, want it from ~/", p)
	}
	if _, err := s.Apply(app.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := readlink(t, filepath.Join(home, ".agents", "skills", "skilldots")); got != back {
		t.Fatalf("after apply ~/.agents/skills/skilldots → %s, want %s", got, back)
	}

	// An [[app]] skilldots of the user's own (not inside an app bundle)
	// is left alone.
	own := filepath.Join(home, "own", "skilldots")
	write(t, filepath.Join(own, "SKILL.md"), skillMD("skilldots"))
	s, _ = app.Open(root)
	s.Lib.Manifest.Apps[0].Path = own
	if err := s.Lib.Save(); err != nil {
		t.Fatal(err)
	}
	s, _ = app.Open(root)
	if changed, _, err := s.RepointAppSelfSkill(back); changed || err != nil {
		t.Fatalf("the user's own [[app]] skilldots was repointed (err %v)", err)
	}
}

func TestIsBundledSelfSkill(t *testing.T) {
	t.Setenv("HOME", "/Users/x")
	for p, want := range map[string]bool{
		"/Applications/skilldots.app/Contents/Resources/skills/skilldots":        true,
		"~/Applications/skilldots 2.app/Contents/Resources/skills/skilldots":     true,
		"/Applications/skilldots.app/Contents/Resources/skills/skilldots/../x/y": false,
		"Contents/Resources/skills/skilldots":                                    false,
		".app/Contents/Resources/skills/skilldots":                               false,
		"/Users/x/skills/skilldots":                                              false,
	} {
		if got := app.IsBundledSelfSkill(p); got != want {
			t.Errorf("IsBundledSelfSkill(%q) = %v, want %v", p, got, want)
		}
	}
}
