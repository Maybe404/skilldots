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
