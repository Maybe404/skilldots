package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// linkOut replaces the library's dir (skills or private) with a link to
// an empty directory outside the library, as a cloned library can, and
// returns that directory.
func linkOut(t *testing.T, home, root, dir string) string {
	t.Helper()
	out := filepath.Join(home, "outside-"+dir)
	os.MkdirAll(out, 0o755)
	os.RemoveAll(filepath.Join(root, dir))
	if err := os.Symlink(out, filepath.Join(root, dir)); err != nil {
		t.Fatal(err)
	}
	return out
}

func checkEmpty(t *testing.T, dir string) {
	t.Helper()
	if m, _ := filepath.Glob(filepath.Join(dir, "*")); len(m) != 0 {
		t.Fatalf("wrote outside the library: %v", m)
	}
}

// skd new writes nothing through skills/ or private/ when it is a link
// out of the library.
func TestNewRefusesALinkedSkillsDir(t *testing.T) {
	for _, private := range []bool{false, true} {
		home, root := side(t, t.TempDir())
		dir := "skills"
		if private {
			dir = "private"
		}
		out := linkOut(t, home, root, dir)
		s, err := app.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.New("fresh", private); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("private %v: %v", private, err)
		}
		checkEmpty(t, out)
	}
}

// Forking copies into skills/; through a link that would put the copy
// (someone's vendored files) anywhere. A skill whose own directory goes
// through a link inside the library is not copied either.
func TestForkRefusesLinksInTheLibrary(t *testing.T) {
	home, root := side(t, t.TempDir())
	appDir := filepath.Join(home, "Apps", "X.app", "skills", "x")
	write(t, filepath.Join(appDir, "SKILL.md"), skillMD("x"))
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	s.Lib.Manifest.Apps = append(s.Lib.Manifest.Apps, &manifest.App{Name: "x", Path: appDir})
	out := linkOut(t, home, root, "skills")
	if _, err := s.Fork(app.Ref{Key: "x"}, false, ""); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("fork through skills/ link: %v", err)
	}
	checkEmpty(t, out)

	// A vendored skill reached through vendor/acme -> elsewhere.
	home, root = side(t, t.TempDir())
	elsewhere := filepath.Join(home, "elsewhere")
	write(t, filepath.Join(elsewhere, "skills", "v", "SKILL.md"), skillMD("v"))
	os.Symlink(elsewhere, filepath.Join(root, "vendor", "acme"))
	s, err = app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	s.Lib.Manifest.Packs = append(s.Lib.Manifest.Packs, &manifest.Pack{Repo: "acme/skills", Use: []string{"*"}})
	s.Lib.Lock.Packs["acme/skills"] = &lock.Pack{Skills: map[string]*lock.Skill{"v": {Category: "", Hash: "x"}}}
	entries, _ := s.Resolve()
	var e *engine.Entry
	for i := range entries {
		if entries[i].Name == "v" {
			e = &entries[i]
		}
	}
	if e == nil || e.LinkedVia == "" {
		t.Fatalf("setup: %+v", entries)
	}
	if _, err := s.Fork(app.Ref{Key: "v"}, false, "mine"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("fork of a linked vendor skill: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "skills", "mine")); err == nil {
		t.Fatal("copied")
	}
}

// adopt copies an agent's own skill into skills/; never through a link.
func TestAdoptRefusesALinkedSkillsDir(t *testing.T) {
	home, root := side(t, t.TempDir())
	write(t, filepath.Join(home, ".agents", "skills", "mine", "SKILL.md"), skillMD("mine"))
	out := linkOut(t, home, root, "skills")
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	var own []app.Proposal
	for _, p := range s.Proposals(rep) {
		if p.Kind == "own" {
			own = append(own, p)
		}
	}
	if len(own) != 1 {
		t.Fatalf("proposals %+v", own)
	}
	if _, err := s.Adopt(own); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("adopt: %v", err)
	}
	checkEmpty(t, out)
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "mine", "SKILL.md")); err != nil {
		t.Fatal("the agent's skill moved")
	}
}

// An [[app]] that is not a skill (no SKILL.md, say ~/.ssh) is not forked
// into the library either.
func TestForkRefusesAnAppThatIsNotASkill(t *testing.T) {
	home, root := side(t, t.TempDir())
	keys := filepath.Join(home, ".ssh")
	write(t, filepath.Join(keys, "id_ed25519"), "KEY")
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	s.Lib.Manifest.Apps = append(s.Lib.Manifest.Apps, &manifest.App{Name: "keys", Path: keys})
	if _, err := s.Fork(app.Ref{Key: "keys"}, false, ""); err == nil {
		t.Fatal("forked")
	}
	if _, err := os.Lstat(filepath.Join(root, "skills", "keys")); err == nil {
		t.Fatal("copied")
	}
}
