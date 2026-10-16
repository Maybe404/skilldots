package app_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

// add --skill on a pack that follows "*" takes the skill out of skip when
// skip names it; a wildcard it cannot take apart is an error that says
// which pattern and how to change it.
func TestAddSkillAgainstSkip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := upstream(t)
	_, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--all")
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Lib.Manifest.FindPack("acme/skills")
	p.Skip = []string{"eng/*", "grill"}
	if err := s.Lib.Save(); err != nil {
		t.Fatal(err)
	}

	add := func(name string) error {
		s, err := app.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		src, err := s.ParseAdd("acme/skills", "")
		if err != nil {
			t.Fatal(err)
		}
		f, err := s.Fetch(src)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.Add(f, app.Selection{Skills: []string{name}})
		return err
	}
	if err := add("grill"); err != nil {
		t.Fatalf("add --skill grill (skipped by name): %v", err)
	}
	err = add("tdd")
	if err == nil {
		t.Fatal("add --skill tdd under skip eng/* succeeded")
	}
	for _, want := range []string{`"eng/*"`, "skills.toml", "skd update acme/skills"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}
