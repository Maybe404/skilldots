package manifest

import "testing"

// A pack is found by every way a person names it: its key, the URL it was
// added from, a local path, an older skd's key for file://, and the last
// segment of its repository when no other pack has it.
func TestFindPackBySpelling(t *testing.T) {
	m := &Manifest{Packs: []*Pack{
		{Repo: "acme/skills"},
		{Repo: "file////private/tmp/demo-skills"}, // written by an older skd
		{Repo: "/home/me/local-skills"},
		{Repo: "gitlab.com/group/tools"},
		{Repo: "other/skills"},
	}}
	for key, want := range map[string]string{
		"acme/skills":                        "acme/skills",
		"https://github.com/acme/skills":     "acme/skills",
		"https://github.com/acme/skills.git": "acme/skills",
		"git@github.com:acme/skills.git":     "acme/skills",
		"file////private/tmp/demo-skills":    "file////private/tmp/demo-skills",
		"file:///private/tmp/demo-skills":    "file////private/tmp/demo-skills",
		"/private/tmp/demo-skills":           "file////private/tmp/demo-skills",
		"demo-skills":                        "file////private/tmp/demo-skills",
		"file:///home/me/local-skills":       "/home/me/local-skills",
		"local-skills":                       "/home/me/local-skills",
		"https://gitlab.com/group/tools.git": "gitlab.com/group/tools",
		"tools":                              "gitlab.com/group/tools",
		"skills":                             "", // two packs end in it: not a guess
		"nothing":                            "",
	} {
		got := ""
		if p := m.FindPack(key); p != nil {
			got = p.Repo
		}
		if got != want {
			t.Errorf("FindPack(%q) = %q, want %q", key, got, want)
		}
	}
}
