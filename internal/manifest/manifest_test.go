package manifest

import "testing"

func TestPrefix(t *testing.T) {
	cases := map[string]string{
		"mattpocock/skills":           "skills",
		"gitlab.com/group/sub/repo":   "repo",
		"/home/me/upstream/evil":      "evil",
		"/home/me/upstream/evil.git/": "evil",
		"example.com/x/my repo":       "my-repo",
		"/":                           "pack",
		`C:\Users\me\repos\winrepo`:   "winrepo",
	}
	for repo, want := range cases {
		if got := (&Pack{Repo: repo}).Prefix(); got != want {
			t.Errorf("%q: got %q want %q", repo, got, want)
		}
	}
	if got := (&Pack{Repo: "o/r", Alias: "a/b"}).Prefix(); got != "a-b" {
		t.Errorf("alias: got %q", got)
	}
}

func TestSelectedSurvivesCategoryChange(t *testing.T) {
	p := &Pack{Use: []string{"engineering/tdd", "pr"}}
	for _, c := range []struct {
		name, cat string
		want      bool
	}{{"tdd", "engineering", true}, {"tdd", "productivity", true}, {"pr", "x", true}, {"other", "engineering", false}, {"dd", "", false}} {
		if got := p.Selected(c.name, c.cat); got != c.want {
			t.Errorf("%s in %q: %v", c.name, c.cat, got)
		}
	}
}

// An alias that spells another pack's repo (written by hand, or by an
// older skd) never shadows that pack: a repo match wins.
func TestFindPackPrefersRepo(t *testing.T) {
	m := New(nil)
	a := &Pack{Repo: "evil/x", Alias: "acme/skills"}
	b := &Pack{Repo: "acme/skills"}
	m.Packs = []*Pack{a, b}
	if got := m.FindPack("acme/skills"); got != b {
		t.Errorf("FindPack(acme/skills) = %+v", got)
	}
	if got := m.FindPack("evil/x"); got != a {
		t.Errorf("FindPack(evil/x) = %+v", got)
	}
	m.Packs[1].Alias = "short"
	if got := m.FindPack("short"); got != b {
		t.Errorf("FindPack(short) = %+v", got)
	}
}

func TestCheckAlias(t *testing.T) {
	m := New(nil)
	m.Packs = []*Pack{{Repo: "acme/skills"}, {Repo: "local"}}
	for _, bad := range []string{"acme/skills", "ACME/Skills", "a/b", "x:y", "local", "LOCAL"} {
		if err := m.CheckAlias(bad); err == nil {
			t.Errorf("alias %q accepted", bad)
		}
	}
	for _, good := range []string{"", "acme", "my-skills"} {
		if err := m.CheckAlias(good); err != nil {
			t.Errorf("alias %q: %v", good, err)
		}
	}
}
