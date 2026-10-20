package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

// adoptUpstreamSide is a side whose agent directories hold, from
// acme/skills: npx copies of tdd (changed here) and grill (as upstream
// has it), and links into a clone of the repository to plan (changed
// in the clone) and review (as upstream has it).
func adoptUpstreamSide(t *testing.T) (home, root, clone string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := upstream(t)
	home, root = side(t, up)
	repo := filepath.Join(up, "acme", "skills.git")
	cp := func(from, to string) {
		t.Helper()
		if out, err := exec.Command("cp", "-R", from, to).CombinedOutput(); err != nil {
			t.Fatalf("cp: %v %s", err, out)
		}
	}
	cp(filepath.Join(repo, "skills", "eng", "tdd"), filepath.Join(home, ".claude", "skills", "tdd"))
	write(t, filepath.Join(home, ".claude", "skills", "tdd", "SKILL.md"), skillMD("tdd")+"my own rule\n")
	cp(filepath.Join(repo, "skills", "prod", "grill"), filepath.Join(home, ".claude", "skills", "grill"))
	write(t, filepath.Join(home, ".agents", ".skill-lock.json"),
		`{"skills":{"tdd":{"source":"acme/skills","skillPath":"skills/eng/tdd/SKILL.md"},"grill":{"source":"acme/skills","skillPath":"skills/prod/grill/SKILL.md"}}}`)
	clone = filepath.Join(home, "clone")
	git(t, home, "clone", "-q", "https://github.com/acme/skills", clone)
	write(t, filepath.Join(clone, "skills", "prod", "plan", "SKILL.md"), skillMD("plan")+"changed in the clone\n")
	for name, dir := range map[string]string{"plan": "prod/plan", "review": "eng/review"} {
		if err := os.Symlink(filepath.Join(clone, "skills", dir), filepath.Join(home, ".agents", "skills", name)); err != nil {
			t.Fatal(err)
		}
	}
	return home, root, clone
}

// A copy or a link whose content differs from what its upstream has is
// proposed as a skill of your own, with the upstream named, never as a
// subscription that would put upstream's version in its place; one that
// is what upstream has is still a subscription. Adopting keeps every
// change: the changed skills are copied into skills/ and linked.
func TestAdoptNeverReplacesLocalChangesWithUpstream(t *testing.T) {
	home, root, clone := adoptUpstreamSide(t)
	s := reopen(t, root)
	rep, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	props := s.Proposals(rep)
	got := map[string]app.Proposal{}
	for _, p := range props {
		if p.Kind != "enable" {
			got[p.Item.Name] = p
		}
	}
	for name, want := range map[string]struct {
		kind    string
		differs bool
	}{"tdd": {"own", true}, "plan": {"own", true}, "grill": {"pack", false}, "review": {"pack", false}} {
		p := got[name]
		if p.Kind != want.kind || p.Differs != want.differs || p.Upstream != "acme/skills" {
			t.Errorf("%s: %+v", name, p)
		}
		if want.differs && (!strings.Contains(p.Label, "differs from acme/skills") || !strings.Contains(p.Label, "skd add acme/skills --skill "+name)) {
			t.Errorf("%s: label %q", name, p.Label)
		}
	}
	if _, err := s.Adopt(props); err != nil {
		t.Fatal(err)
	}
	for name, mark := range map[string]string{"tdd": "my own rule", "plan": "changed in the clone"} {
		b, err := os.ReadFile(filepath.Join(root, "skills", name, "SKILL.md"))
		if err != nil || !strings.Contains(string(b), mark) {
			t.Errorf("skills/%s: %q %v", name, b, err)
		}
	}
	for _, link := range []string{".agents/skills/plan", ".claude/skills/tdd"} {
		if to, err := os.Readlink(filepath.Join(home, link)); err != nil || !strings.HasPrefix(to, filepath.Join(root, "skills")) {
			t.Errorf("%s → %q %v", link, to, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(clone, "skills", "prod", "plan", "SKILL.md")); !strings.Contains(string(b), "changed in the clone") {
		t.Error("the clone was changed")
	}
	p := reopen(t, root).Lib.Manifest.FindPack("acme/skills")
	if p == nil || strings.Join(p.Use, ",") != "grill,review" {
		t.Fatalf("pack: %+v", p)
	}
}

// A link the proposals took for upstream's content that changed before
// the user confirmed is left in place, not replaced by upstream's.
func TestAdoptLeavesALinkThatChangedSinceTheScan(t *testing.T) {
	home, root, clone := adoptUpstreamSide(t)
	s := reopen(t, root)
	var logs []string
	s.Log = func(l app.Level, msg string) {
		if l == app.Warn {
			logs = append(logs, msg)
		}
	}
	rep, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	var review []app.Proposal
	for _, p := range s.Proposals(rep) {
		if p.Item.Name == "review" {
			review = append(review, p)
		}
	}
	if len(review) != 1 || review[0].Kind != "pack" {
		t.Fatalf("%+v", review)
	}
	write(t, filepath.Join(clone, "skills", "eng", "review", "SKILL.md"), skillMD("review")+"changed after the scan\n")
	if _, err := s.Adopt(review); err != nil {
		t.Fatal(err)
	}
	if to, err := os.Readlink(filepath.Join(home, ".agents", "skills", "review")); err != nil || to != filepath.Join(clone, "skills", "eng", "review") {
		t.Fatalf("review → %q %v", to, err)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "differs from the library's review") {
		t.Errorf("no warning: %q", logs)
	}
}
