package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// offlineHome isolates HOME, the settings and git, with GitHub leading to
// an empty directory: fetching Maybe404/skilldots fails as it does
// offline. It returns home.
func offlineHome(t *testing.T) string {
	t.Helper()
	home, _ := filepath.EvalSymlinks(t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SKILLDOTS_LIBRARY", "")
	t.Setenv("SKILLDOTS_CACHE", filepath.Join(home, "cache"))
	nowhere := filepath.Join(home, "nowhere")
	if err := os.MkdirAll(nowhere, 0o755); err != nil {
		t.Fatal(err)
	}
	gc := filepath.Join(home, "gitconfig")
	body := "[user]\n\tname = t\n\temail = t@example.com\n[url \"file://" + nowhere + "/\"]\n\tinsteadOf = https://github.com/\n"
	if err := os.WriteFile(gc, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gc)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return home
}

// bundledApp makes <dir>/skilldots.app with its executable and the
// skilldots skill among its resources; it returns the executable.
func bundledApp(t *testing.T, dir string) string {
	t.Helper()
	exe := fakeApp(t, dir, "skilldots")
	skill := filepath.Join(filepath.Dir(filepath.Dir(exe)), "Resources", "skills", "skilldots")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: skilldots\ndescription: d\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return exe
}

// runningFrom makes the app layer see exe as the running app.
func runningFrom(t *testing.T, exe string) {
	t.Helper()
	prev := app.BundledSelfSkill
	app.BundledSelfSkill = func() string { return skillInBundle(exe) }
	t.Cleanup(func() { app.BundledSelfSkill = prev })
}

func TestSkillInBundle(t *testing.T) {
	offlineHome(t)
	dir := t.TempDir()
	exe := bundledApp(t, dir)
	want := filepath.Join(filepath.Dir(filepath.Dir(exe)), "Resources", "skills", "skilldots")
	if got := skillInBundle(exe); got != want {
		t.Fatalf("skillInBundle = %q, want %q", got, want)
	}
	// mygo dev's "skilldots Dev.app", go run's bare binary, and a bundle
	// without the resource have none.
	dev := filepath.Join(dir, "skilldots Dev.app", "Contents", "MacOS", "skilldots Dev")
	for _, exe := range []string{dev, filepath.Join(dir, "skilldots"), fakeApp(t, t.TempDir(), "skilldots")} {
		if got := skillInBundle(exe); got != "" {
			t.Fatalf("skillInBundle(%s) = %q, want none", exe, got)
		}
	}
}

// The packaged app sets a new library up offline: the skill comes from
// the bundle as an [[app]] entry and is linked into the agents.
func TestInitLibraryWithBundledSkillOffline(t *testing.T) {
	home := offlineHome(t)
	exe := bundledApp(t, filepath.Join(home, "Applications"))
	runningFrom(t, exe)
	s := New("", Native{})
	st := invoke(t, s, "initLibrary", filepath.Join(home, "library"), true).(object)
	if st["skillError"] != nil || st["selfSkill"] != true {
		t.Fatalf("offline initLibrary from the app: skillError %v, selfSkill %v", st["skillError"], st["selfSkill"])
	}
	m, err := manifest.Load(filepath.Join(home, "library", "skills.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Apps) != 1 || len(m.Packs) != 0 || m.Apps[0].Path != "~/Applications/skilldots.app/Contents/Resources/skills/skilldots" {
		t.Fatalf("manifest apps %+v packs %d", m.Apps, len(m.Packs))
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "skilldots", "SKILL.md")); err != nil {
		t.Fatalf("not linked: %v", err)
	}
	// Asked again: no second copy.
	invoke(t, s, "installSelfSkill")
	if m, _ = manifest.Load(filepath.Join(home, "library", "skills.toml")); len(m.Apps) != 1 || len(m.Packs) != 0 {
		t.Fatalf("installSelfSkill added a second skilldots: %+v", m)
	}
}

// Switching the app to another library points that library's entry at this
// app's copy right away, and the status says the skill comes with the app.
func TestUseLibraryRepointsSelfSkill(t *testing.T) {
	home := offlineHome(t)
	exe := bundledApp(t, filepath.Join(home, "Applications"))
	runningFrom(t, exe)
	s := New("", Native{})
	other := filepath.Join(home, "other")
	invoke(t, s, "initLibrary", other, true)
	invoke(t, s, "initLibrary", filepath.Join(t.TempDir(), "second"), false)

	apps, _ := filepath.EvalSymlinks(t.TempDir())
	if err := os.Rename(filepath.Join(home, "Applications", "skilldots.app"), filepath.Join(apps, "skilldots.app")); err != nil {
		t.Fatal(err)
	}
	runningFrom(t, filepath.Join(apps, "skilldots.app", "Contents", "MacOS", "skilldots"))
	st := invoke(t, s, "useLibrary", other).(object)
	if st["bundledSkill"] != true {
		t.Fatalf("bundledSkill = %v", st["bundledSkill"])
	}
	want := filepath.Join(apps, "skilldots.app", "Contents", "Resources", "skills", "skilldots")
	m, err := manifest.Load(filepath.Join(other, "skills.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Apps) != 1 || m.Apps[0].Path != want {
		t.Fatalf("[[app]] after useLibrary: %+v, want %s", m.Apps, want)
	}
	if got, _ := os.Readlink(filepath.Join(home, ".agents", "skills", "skilldots")); got != want {
		t.Fatalf("~/.agents/skills/skilldots → %s, want %s", got, want)
	}

	// In development the skill would come from GitHub.
	runningFrom(t, filepath.Join(home, "skilldots Dev.app", "Contents", "MacOS", "skilldots Dev"))
	if st := invoke(t, s, "status").(object); st["bundledSkill"] != false {
		t.Fatalf("development bundledSkill = %v", st["bundledSkill"])
	}
}

// In development there is no bundle: the GitHub source is used, so
// offline the library is made and the status says why the skill is not.
func TestInitLibraryInDevelopmentUsesGitHub(t *testing.T) {
	home := offlineHome(t)
	runningFrom(t, filepath.Join(home, "skilldots Dev.app", "Contents", "MacOS", "skilldots Dev"))
	s := New("", Native{})
	st := invoke(t, s, "initLibrary", filepath.Join(home, "library"), true).(object)
	if st["skillError"] == nil || st["selfSkill"] != false {
		t.Fatalf("development initLibrary offline: skillError %v, selfSkill %v", st["skillError"], st["selfSkill"])
	}
	if !strings.Contains(st["skillError"].(string), "skilldots") {
		t.Fatalf("skillError does not name the repository: %v", st["skillError"])
	}
}

// The app moved from ~/Applications to elsewhere: at launch the library's
// entry and the agents' links follow it.
func TestRepointSelfSkillAfterMove(t *testing.T) {
	home := offlineHome(t)
	exe := bundledApp(t, filepath.Join(home, "Applications"))
	runningFrom(t, exe)
	s := New("", Native{})
	invoke(t, s, "initLibrary", filepath.Join(home, "library"), true)

	apps, _ := filepath.EvalSymlinks(t.TempDir())
	if err := os.Rename(filepath.Join(home, "Applications", "skilldots.app"), filepath.Join(apps, "skilldots.app")); err != nil {
		t.Fatal(err)
	}
	runningFrom(t, filepath.Join(apps, "skilldots.app", "Contents", "MacOS", "skilldots"))
	// A new launch: the root comes from the settings.
	RepointSelfSkill(New("", Native{}))
	want := filepath.Join(apps, "skilldots.app", "Contents", "Resources", "skills", "skilldots")
	m, err := manifest.Load(filepath.Join(home, "library", "skills.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Apps[0].Path != want {
		t.Fatalf("[[app]] path = %s, want %s", m.Apps[0].Path, want)
	}
	if got, _ := os.Readlink(filepath.Join(home, ".agents", "skills", "skilldots")); got != want {
		t.Fatalf("~/.agents/skills/skilldots → %s, want %s", got, want)
	}
}
