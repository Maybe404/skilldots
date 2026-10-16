package app_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/pack"
)

func TestParseAddText(t *testing.T) {
	cases := []struct {
		in            string
		source        string
		skills, paths []string
		from          string
	}{
		{in: "mattpocock/skills", source: "mattpocock/skills"},
		{in: "  acme  ", source: "acme"}, // an alias
		{in: "owner/repo@v1.2", source: "owner/repo@v1.2"},
		{in: "git@github.com:o/r.git", source: "git@github.com:o/r.git"},
		{in: "npx skills add vercel-labs/agent-skills", source: "vercel-labs/agent-skills", from: "command"},
		{in: "npx skills add vercel-labs/agent-skills --skill frontend-design", source: "vercel-labs/agent-skills",
			skills: []string{"frontend-design"}, from: "command"},
		{in: "$ npx -y skills@latest add https://github.com/o/r -s a,b -s c --skill=d -g -y", source: "https://github.com/o/r",
			skills: []string{"a", "b", "c", "d"}, from: "command"},
		{in: "npx skills add --skill one two o/r --agent claude-code cursor", source: "o/r", skills: []string{"one", "two"}, from: "command"},
		{in: "bunx skills add o/r -s x", source: "o/r", skills: []string{"x"}, from: "command"},
		{in: "pnpm dlx skills add o/r", source: "o/r", from: "command"},
		{in: "pnpx skills i o/r", source: "o/r", from: "command"},
		{in: "yarn dlx skills add o/r", source: "o/r", from: "command"},
		{in: "npx add-skill o/r --skill 'x'", source: "o/r", skills: []string{"x"}, from: "command"},
		{in: "skd add o/r -s x", source: "o/r", skills: []string{"x"}, from: "command"},
		{in: "npx skills add o/r -s x && rm -rf ~", source: "o/r", skills: []string{"x"}, from: "command"},
		{in: "npx skills add o/r\nsecond line", source: "o/r", from: "command"},
		{in: "https://skills.sh/anthropics/skills/frontend-design", source: "anthropics/skills", skills: []string{"frontend-design"}, from: "skills.sh"},
		{in: "skills.sh/anthropics/skills", source: "anthropics/skills", from: "skills.sh"},
		{in: "https://github.com/o/r/tree/main/skills/eng/tdd", source: "https://github.com/o/r", paths: []string{"skills/eng/tdd"}, from: "github link"},
		{in: "https://github.com/o/r/blob/abc123/skills/eng/tdd/SKILL.md", source: "https://github.com/o/r", paths: []string{"skills/eng/tdd"}, from: "github link"},
		{in: "github.com/o/r/tree/main", source: "https://github.com/o/r", from: "github link"},
		{in: "https://github.com/o/r", source: "https://github.com/o/r"},
		{in: "npx skills add https://github.com/o/r/tree/main/skills/x", source: "https://github.com/o/r", paths: []string{"skills/x"}, from: "command"},
	}
	for _, c := range cases {
		h, err := app.ParseAddText(c.in)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if h.Source != c.source || !slices.Equal(h.Skills, c.skills) || !slices.Equal(h.Paths, c.paths) || h.From != c.from {
			t.Errorf("%q: got %+v", c.in, h)
		}
		// Whatever comes out is a source skd can fetch.
		if _, err := pack.ParseSource(h.Source); err != nil && c.source != "acme" {
			t.Errorf("%q: source %q: %v", c.in, h.Source, err)
		}
	}
	for _, bad := range []string{"", "npx skills remove o/r", "npx skills add", "npx skills add o/r -s ../x", "npx skills add 'o/r", "skills.sh/x", "npx cowsay o/r",
		// --skill names are skill names: no path, no hidden name, no spaces.
		"npx skills add o/r --skill=.hidden", "npx skills add o/r -s 'a b'", "npx skills add o/r --skill=..", "npx skills add o/r -s x,.y",
		"https://skills.sh/o/r/.git-x"} {
		if h, err := app.ParseAddText(bad); err == nil {
			t.Errorf("%q accepted: %+v", bad, h)
		}
	}
}

// The ref in a GitHub link is not pinned: the pack follows the default
// branch, like owner/repo.
func TestParseAddTextDoesNotPin(t *testing.T) {
	h, err := app.ParseAddText("https://github.com/o/r/tree/v2.0/skills/x")
	if err != nil {
		t.Fatal(err)
	}
	src, err := pack.ParseSource(h.Source)
	if err != nil || src.Ref != "" || src.Repo != "o/r" {
		t.Fatalf("source %+v err %v", src, err)
	}
}

func TestPreselect(t *testing.T) {
	clone := t.TempDir()
	sk := func(rel, name, cat string) pack.Skill {
		return pack.Skill{Name: name, Category: cat, Dir: filepath.Join(clone, filepath.FromSlash(rel))}
	}
	f := &app.Fetched{Clone: clone, Skills: []pack.Skill{
		sk("skills/eng/tdd", "tdd", "eng"),
		sk("skills/eng/review", "code-review", "eng"), // name differs from its directory
		sk("skills/prod/grill", "grill", "prod"),
	}}
	cases := []struct {
		h             app.AddHint
		names, missed []string
	}{
		{app.AddHint{Skills: []string{"grill", "tdd"}}, []string{"tdd", "grill"}, nil},
		{app.AddHint{Skills: []string{"review"}}, []string{"code-review"}, nil},
		{app.AddHint{Skills: []string{"nope"}}, nil, []string{"nope"}},
		{app.AddHint{Paths: []string{"skills/eng/review"}}, []string{"code-review"}, nil},
		{app.AddHint{Paths: []string{"skills/eng/tdd/references/x.md"}}, []string{"tdd"}, nil},
		{app.AddHint{Paths: []string{"skills/eng"}}, []string{"tdd", "code-review"}, nil},
		// A ref with a slash leaves its tail in front of the path.
		{app.AddHint{Paths: []string{"x/skills/prod/grill"}}, []string{"grill"}, nil},
		{app.AddHint{Paths: []string{"docs"}}, nil, []string{"docs"}},
	}
	for _, c := range cases {
		names, missed := f.Preselect(c.h)
		if !slices.Equal(names, c.names) || !slices.Equal(missed, c.missed) {
			t.Errorf("%+v: names %v missed %v", c.h, names, missed)
		}
	}
}

func TestSplitCommandNeverExpands(t *testing.T) {
	w, err := app.SplitCommand(`code --wait "$HOME/x" 'a b' c\ d $(rm)`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"code", "--wait", "$HOME/x", "a b", "c d", "$(rm)"}
	if !slices.Equal(w, want) {
		t.Fatalf("got %q", strings.Join(w, "|"))
	}
}

func TestAddArgs(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
		ok   bool
	}{
		{nil, "", true},
		{[]string{"o/r"}, "o/r", true},
		{[]string{"npx", "skills", "add", "o/r"}, "npx skills add o/r", true},
		{[]string{"o/r", "extra"}, "", false},
		{[]string{"https://github.com/o/r", "--skill"}, "", false},
	} {
		got, err := app.AddArgs(c.args)
		if (err == nil) != c.ok || (c.ok && got != c.want) {
			t.Errorf("%q: %q %v", c.args, got, err)
		}
	}
	// A path split at a space: the advice fits a path, not only a pasted
	// install command.
	_, err := app.AddArgs([]string{"./up", "sp"})
	if err == nil || !strings.Contains(err.Error(), "path or URL with spaces needs quotes") || !strings.Contains(err.Error(), "pasted install command") {
		t.Errorf("./up sp: %v", err)
	}
}
