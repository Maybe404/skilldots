package app_test

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// agentContents maps "<agent dir>/<entry>" to the content hash of what the
// agent reads there, for every entry of ~/.agents/skills and
// ~/.claude/skills.
func agentContents(t *testing.T, home string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, d := range []string{".agents/skills", ".claude/skills"} {
		ents, err := os.ReadDir(filepath.Join(home, d))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			p := filepath.Join(home, d, e.Name())
			if real, err := filepath.EvalSymlinks(p); err == nil {
				p = real
			}
			h, err := fsutil.HashTree(p)
			if err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			out[d+"/"+e.Name()] = h
		}
	}
	return out
}

// A machine that has a skilldots library already (made before, at another
// path, and a clone of the user's own repository): adopting into a new
// library takes each skill as that library has it. A vendored skill is a
// subscription to its own upstream at the ref the old pack follows, a
// private one is copied into private/ (never pushed), one of skills/ into
// skills/; nothing is a subscription to the old library's repository. A
// link into the old library that keeps the new library's own link out is
// replaced by it. Every agent reads what it read before, and the old
// library is left exactly as it was.
func TestAdoptFromAnotherLibrary(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home, oldRoot, old := restoreSide(t)
	if _, err := old.NewForTargets("secret", true, []string{"agents", "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := old.New("mine", false); err != nil {
		t.Fatal(err)
	}
	// The ref the old library's pack follows: the upstream's branch.
	clone := filepath.Join(home, "cache-probe")
	git(t, home, "clone", "-q", "https://github.com/acme/skills", clone)
	branch, err := exec.Command("git", "-C", clone, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	ref := strings.TrimSpace(string(branch))
	old.Lib.Manifest.FindPack("acme/skills").Ref = ref
	if err := old.Lib.Save(); err != nil {
		t.Fatal(err)
	}
	git(t, oldRoot, "remote", "add", "origin", "https://github.com/me/agent-skills")

	newRoot := filepath.Join(home, "new")
	for _, d := range []string{"skills", "vendor", "private"} {
		os.MkdirAll(filepath.Join(newRoot, d), 0o755)
	}
	if err := manifest.New([]string{"agents", "claude"}).Save(filepath.Join(newRoot, manifest.FileName)); err != nil {
		t.Fatal(err)
	}
	if err := (&lock.Lock{Version: 1, Packs: map[string]*lock.Pack{}, Links: map[string]*lock.Link{}}).Save(filepath.Join(newRoot, lock.FileName)); err != nil {
		t.Fatal(err)
	}
	git(t, newRoot, "init", "-q")

	before := agentContents(t, home)
	oldBefore := libraryContents(t, oldRoot)

	// The new library has a skill of its own named grill already: the old
	// library's link to its grill keeps the new one out, as a library made
	// before keeps out the skilldots skill a new one installs.
	s := reopen(t, newRoot)
	if _, err := s.New("grill", false); err != nil {
		t.Fatal(err)
	}

	s = reopen(t, newRoot)
	rep, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	props := s.Proposals(rep)
	got := map[string]app.Proposal{}
	for _, p := range props {
		if p.Repo == "me/agent-skills" {
			t.Errorf("%s: subscribed to the old library's repository: %+v", p.Item.Name, p)
		}
		if _, ok := got[p.Item.Name]; !ok {
			got[p.Item.Name] = p
		}
	}
	for _, name := range []string{"tdd", "review"} {
		if p := got[name]; p.Kind != "pack" || p.Repo != "acme/skills" || p.Ref != ref || p.From != oldRoot {
			t.Errorf("%s: %+v", name, p)
		}
	}
	if p := got["secret"]; p.Kind != "own" || !p.Private || p.From != oldRoot {
		t.Errorf("secret: %+v", p)
	}
	if p := got["mine"]; p.Kind != "own" || p.Private || p.From != oldRoot {
		t.Errorf("mine: %+v", p)
	}
	if p := got["grill"]; p.Kind != "enable" || p.From != oldRoot {
		t.Errorf("grill: %+v", p)
	}

	if _, err := s.Adopt(props); err != nil {
		t.Fatal(err)
	}

	s = reopen(t, newRoot)
	var repos []string
	for _, p := range s.Lib.Manifest.Packs {
		repos = append(repos, p.Repo)
	}
	if !slices.Equal(repos, []string{"acme/skills"}) || !slices.Equal(s.Lib.Manifest.FindPack("acme/skills").Use, []string{"review", "tdd"}) {
		t.Errorf("packs: %v", repos)
	}
	if p := s.Lib.Manifest.FindPack("acme/skills"); p == nil || p.Ref != ref {
		t.Errorf("acme/skills: %+v, want ref %s", p, ref)
	}
	if !fsutil.Exists(filepath.Join(newRoot, "private", "secret", "SKILL.md")) || fsutil.Lexists(filepath.Join(newRoot, "skills", "secret")) {
		t.Error("secret is not in private/ alone")
	}
	if !fsutil.Exists(filepath.Join(newRoot, "skills", "mine", "SKILL.md")) {
		t.Error("mine is not in skills/")
	}
	for _, d := range []string{".agents/skills", ".claude/skills"} {
		ents, _ := os.ReadDir(filepath.Join(home, d))
		for _, e := range ents {
			to, err := os.Readlink(filepath.Join(home, d, e.Name()))
			if err != nil || !strings.HasPrefix(to, newRoot+string(filepath.Separator)) {
				t.Errorf("%s/%s → %q, want a link into the new library", d, e.Name(), to)
			}
		}
	}
	after := agentContents(t, home)
	// grill is the new library's own now, where the old library linked it.
	for _, k := range []string{".agents/skills/grill", ".claude/skills/grill"} {
		delete(before, k)
		delete(after, k)
	}
	for k, h := range before {
		if after[k] != h {
			t.Errorf("%s: content changed or gone (%q → %q)", k, h, after[k])
		}
	}
	for k := range after {
		if before[k] == "" {
			t.Errorf("%s: new entry", k)
		}
	}
	if oldAfter := libraryContents(t, oldRoot); !maps.Equal(oldBefore, oldAfter) {
		t.Errorf("the old library changed:\n%v\n%v", oldBefore, oldAfter)
	}
}

// libraryContents is what a library holds: its manifest and lock, and
// the content hash of skills/, private/ and vendor/.
func libraryContents(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range []string{manifest.FileName, lock.FileName} {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		out[f] = string(b)
	}
	for _, d := range []string{"skills", "private", "vendor"} {
		h, err := fsutil.HashTree(filepath.Join(root, d))
		if err != nil {
			t.Fatal(err)
		}
		out[d] = h
	}
	return out
}
