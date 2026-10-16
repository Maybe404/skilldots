package pack

import (
	"os"
	"path/filepath"
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

func TestScanPluginsLayout(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "plugins/differential-review/skills/differential-review", "plugins/modern-python/skills/modern-python")
	got, _, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range got {
		if s.Category != "" {
			t.Errorf("%s: category %q want empty", s.Name, s.Category)
		}
	}
	if len(got) != 2 {
		t.Fatalf("got %d skills", len(got))
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
