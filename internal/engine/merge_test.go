package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/library"
)

func set(kv ...string) FileSet {
	out := FileSet{}
	for i := 0; i < len(kv); i += 2 {
		out[kv[i]] = &File{Data: []byte(kv[i+1])}
	}
	return out
}

const body = "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\n"

func TestMergeFileByFile(t *testing.T) {
	base := set("SKILL.md", body, "keep.md", "k\n", "gone.md", "g\n", "both.md", "a\n", "bin", "x\x00")
	ours := set(
		"SKILL.md", strings.Replace(body, "one", "ONE", 1), // changed at the top
		"keep.md", "k\n",
		"gone.md", "g local\n", // changed here, removed upstream
		"both.md", "a local\n",
		"bin", "y\x00",
		"mine.md", "mine\n", // added here
	)
	theirs := set(
		"SKILL.md", strings.Replace(body, "eight", "EIGHT", 1), // changed at the bottom
		"keep.md", "k upstream\n",
		"both.md", "a upstream\n",
		"bin", "z\x00",
		"new.md", "new\n", // added upstream
	)
	m, err := Merge(base, ours, theirs, "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(strings.Replace(body, "one", "ONE", 1), "eight", "EIGHT", 1)
	if got := string(m.Files["SKILL.md"].Data); got != want {
		t.Errorf("SKILL.md merged to %q, want %q", got, want)
	}
	for p, w := range map[string]string{"keep.md": "k upstream\n", "mine.md": "mine\n", "new.md": "new\n", "gone.md": "g local\n", "bin": "y\x00"} {
		if f := m.Files[p]; f == nil || string(f.Data) != w {
			t.Errorf("%s = %v, want %q", p, f, w)
		}
	}
	both := string(m.Files["both.md"].Data)
	if !strings.Contains(both, "<<<<<<< local\na local\n=======\na upstream\n>>>>>>> upstream 0123456\n") {
		t.Errorf("both.md:\n%s", both)
	}
	got := []string{}
	for _, c := range m.Conflicts {
		got = append(got, c.Path+":"+c.Kind)
	}
	if w := []string{"bin:binary", "both.md:content", "gone.md:deleted-upstream"}; !slices.Equal(got, w) {
		t.Errorf("conflicts %v, want %v", got, w)
	}
	if string(m.Sidecars["bin.upstream"]) != "z\x00" || m.Sidecars["gone.md.upstream-deleted"] == nil {
		t.Errorf("sidecars %v", m.Sidecars)
	}

	// Removed here, changed upstream: stays removed, upstream's beside it.
	m, err = Merge(set("a", "1\n"), set(), set("a", "2\n"), "c")
	if err != nil || len(m.Conflicts) != 1 || m.Conflicts[0].Kind != ConflictDeletedLocal || m.Files["a"] != nil || string(m.Sidecars["a.upstream"]) != "2\n" {
		t.Errorf("delete/modify: %+v %v", m, err)
	}
	// No base: both added, different content.
	m, err = Merge(nil, set("a", "x\n"), set("a", "y\n"), "c")
	if err != nil || len(m.Conflicts) != 1 || m.Conflicts[0].Kind != ConflictAdded {
		t.Errorf("add/add: %+v %v", m, err)
	}
	// The same change on both sides is no conflict.
	m, err = Merge(set("a", "1\n"), set("a", "2\n"), set("a", "2\n"), "c")
	if err != nil || !m.Clean() || string(m.Files["a"].Data) != "2\n" {
		t.Errorf("same change: %+v %v", m, err)
	}
}

func TestMarkers(t *testing.T) {
	for text, want := range map[string][2]bool{
		"<<<<<<< local\nx\n=======\ny\n>>>>>>> upstream\n": {true, true},
		"Title\n=======\n":    {false, true},
		"a <<<<<<< b\n":       {false, false},
		">>>>>>>\n":           {true, true},
		"<<<<<<<<< nine\n":    {false, false},
		"|||||||  base\n":     {false, true},
		"fine\r\n=======\r\n": {false, true},
	} {
		if got := [2]bool{HasMarkers([]byte(text), false), HasMarkers([]byte(text), true)}; got != want {
			t.Errorf("%q: %v, want %v", text, got, want)
		}
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// Conflicts staged in the index look like a git merge's: git status says
// both modified, and git checkout --theirs takes upstream's side.
func TestStageConflicts(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "vendor", "o", "r", "s")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("base\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("b\n"), 0o644)
	gitRepo(t, root)
	lib := &library.Library{Root: root}
	// The user had staged b.md before.
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("b staged\n"), 0o644)
	gitIn(t, root, "add", "vendor/o/r/s/b.md")
	before := gitOut(t, root, "ls-files", "-s")

	conflicts := []Conflict{{"a.md", ConflictContent}, {"b.md", ConflictContent}}
	base := set("a.md", "base\n", "b.md", "b\n")
	ours := set("a.md", "local\n", "b.md", "b staged\n")
	theirs := set("a.md", "upstream\n", "b.md", "b upstream\n")
	if err := StageConflicts(lib, dir, conflicts, base, ours, theirs); err != nil {
		t.Fatal(err)
	}
	st := gitOut(t, root, "status", "--porcelain")
	if !strings.Contains(st, "UU vendor/o/r/s/a.md") || !strings.Contains(st, "UU vendor/o/r/s/b.md") {
		t.Fatalf("status:\n%s", st)
	}
	gitOut(t, root, "checkout", "--theirs", "--", "vendor/o/r/s/a.md")
	if b, _ := os.ReadFile(filepath.Join(dir, "a.md")); string(b) != "upstream\n" {
		t.Errorf("checkout --theirs: %q", b)
	}
	if err := UnstageConflicts(lib, dir, []string{"a.md", "b.md"}); err != nil {
		t.Fatal(err)
	}
	if after := gitOut(t, root, "ls-files", "-s"); after != before {
		t.Errorf("index not restored:\n%s\nwant\n%s", after, before)
	}
}
