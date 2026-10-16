package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mk(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		dir := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(dir)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o644)
	}
}

func names(skills []Skill) map[string]string {
	out := map[string]string{}
	for _, s := range skills {
		out[s.Name] = s.Category
	}
	return out
}

func TestScanCategorised(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "skills/engineering/tdd", "skills/engineering/pr", "skills/productivity/handoff", "skills/misc/x")
	got, _, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"tdd": "engineering", "pr": "engineering", "handoff": "productivity", "x": "misc"}
	if n := names(got); len(n) != len(want) {
		t.Fatalf("got %v want %v", n, want)
	} else {
		for k, v := range want {
			if n[k] != v {
				t.Errorf("%s: category %q want %q", k, n[k], v)
			}
		}
	}
}

// trailofbits/skills layout: plugins/<plugin>/skills/<skill>. The plugin
// is the category, including for the skill named like its plugin, so
// `--category code-improver` selects code-improver itself.
func TestScanPluginsLayout(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "plugins/differential-review/skills/differential-review",
		"plugins/code-improver/skills/code-improver", "plugins/code-improver/skills/pr-improver",
		"plugins/code-improver/tests/fixtures/review-panel/skills/panel-review")
	got, _, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"differential-review": "differential-review", "code-improver": "code-improver", "pr-improver": "code-improver"}
	if n := names(got); len(n) != len(want) {
		t.Fatalf("got %v want %v", n, want)
	} else {
		for k, v := range want {
			if n[k] != v {
				t.Errorf("%s: category %q want %q", k, n[k], v)
			}
		}
	}
}

// Adding skills or directories upstream never changes the category of a
// skill that did not move.
func TestScanCategoryStable(t *testing.T) {
	layouts := [][]string{
		{"skills/engineering/a", "skills/engineering/b"},
		{"plugins/p/skills/p"},
		{"a"},
		{"src/skills/x/a"},
	}
	additions := []string{"skills/misc/c", "plugins/q/skills/q", "other/deep/d", "e", ".agents/skills/f"}
	for _, base := range layouts {
		root := t.TempDir()
		mk(t, root, base...)
		before, _, err := Scan(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, add := range additions {
			mk(t, root, add)
			after, _, _ := Scan(root)
			an := names(after)
			for _, s := range before {
				if an[s.Name] != s.Category {
					t.Errorf("layout %v + %s: %s category %q → %q", base, add, s.Name, s.Category, an[s.Name])
				}
			}
		}
	}
}

// A repository that ships its skills only in hidden agent directories is
// still found; elsewhere hidden directories are ignored.
func TestScanHiddenOnly(t *testing.T) {
	root := t.TempDir()
	mk(t, root, ".claude/skills/one", ".cursor/skills/two")
	os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o755)
	got, _, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if n := names(got); len(n) != 2 || n["one"] != "" || n["two"] != "" {
		t.Fatalf("got %v", n)
	}
	mk(t, root, "skills/three")
	got, _, _ = Scan(root)
	if n := names(got); len(n) != 1 {
		t.Fatalf("with a visible skill, hidden ones should be ignored: %v", n)
	}
}

// Same name, different content: the canonical copy wins and the others
// are listed as variants, not silently treated as identical.
func TestScanVariants(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "skills/x", "plugin/skills/x", "other/x")
	os.WriteFile(filepath.Join(root, "other", "x", "SKILL.md"), []byte("---\nname: x\n---\ndifferent\n"), 0o644)
	got, _, err := Scan(root)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v err %v", got, err)
	}
	if len(got[0].AltDirs) != 1 || len(got[0].Variants) != 1 || !strings.HasSuffix(got[0].Variants[0], filepath.Join("other", "x")) {
		t.Fatalf("alt %v variants %v", got[0].AltDirs, got[0].Variants)
	}
}

func TestScanDuplicatesCollapse(t *testing.T) {
	root := t.TempDir()
	mk(t, root, ".agents/skills/impeccable", ".claude/skills/impeccable", "plugin/skills/impeccable",
		"cursor-plugin/skills/impeccable", "tests/oracle/.claude/skills/impeccable")
	got, _, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d skills, want 1: %+v", len(got), got)
	}
	s := got[0]
	if rel, _ := filepath.Rel(root, s.Dir); filepath.ToSlash(rel) != ".agents/skills/impeccable" {
		t.Errorf("kept %s, want .agents/skills/impeccable", rel)
	}
	if s.Category != "" {
		t.Errorf("category %q want empty", s.Category)
	}
	if len(s.AltDirs) != 2 {
		t.Errorf("alt dirs %v, want 2 (plugin, cursor-plugin)", s.AltDirs)
	}
}

func TestScanRootIsSkill(t *testing.T) {
	root := t.TempDir()
	mk(t, root, ".")
	got, _, err := Scan(root)
	if err != nil || len(got) != 1 || got[0].Dir != root {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestParseSource(t *testing.T) {
	cases := map[string]Source{
		"mattpocock/skills":                          {Repo: "mattpocock/skills", URL: "https://github.com/mattpocock/skills.git"},
		"github:kepano/obsidian-skills@v2":           {Repo: "kepano/obsidian-skills", URL: "https://github.com/kepano/obsidian-skills.git", Ref: "v2"},
		"https://github.com/pbakaus/impeccable":      {Repo: "pbakaus/impeccable", URL: "https://github.com/pbakaus/impeccable.git"},
		"https://github.com/pbakaus/impeccable.git":  {Repo: "pbakaus/impeccable", URL: "https://github.com/pbakaus/impeccable.git"},
		"git@github.com:trailofbits/skills.git":      {Repo: "trailofbits/skills", URL: "git@github.com:trailofbits/skills.git"},
		"https://gitlab.com/group/sub/repo.git@main": {Repo: "gitlab.com/group/sub/repo", URL: "https://gitlab.com/group/sub/repo.git", Ref: "main"},
	}
	for in, want := range cases {
		got, err := ParseSource(in)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", in, got, want)
		}
	}
}
