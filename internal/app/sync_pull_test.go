package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

const mineMD = "---\nname: mine\ndescription: mine skill\n---\none\ntwo\nthree\nfour\nfive\nsix\n"

// twoMachines: library a, pushed to a bare remote with skd sync, and b,
// a clone of that remote, both with skills/mine.
func twoMachines(t *testing.T) (a, b, remote string) {
	t.Helper()
	up := upstream(t)
	home, a := side(t, up)
	write(t, filepath.Join(a, "skills", "mine", "SKILL.md"), mineMD)
	remote = filepath.Join(home, "remote.git")
	git(t, home, "init", "-q", "--bare", remote)
	s, _ := app.Open(a)
	if out, err := s.Backup(remote, ""); err != nil || !out.Pushed {
		t.Fatalf("first sync: %+v %v", out, err)
	}
	b = filepath.Join(home, "b")
	git(t, home, "clone", "-q", remote, b)
	return a, b, remote
}

func syncLib(t *testing.T, root string) *app.BackupOutcome {
	t.Helper()
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Backup("", "")
	if err != nil {
		t.Fatalf("sync %s: %v", root, err)
	}
	return out
}

// Both changed, different files: b pulls a's change, merges and pushes.
func TestSyncPullsAndMerges(t *testing.T) {
	a, b, remote := twoMachines(t)
	write(t, filepath.Join(a, "skills", "mine", "SKILL.md"), strings.Replace(mineMD, "one", "one from a", 1))
	syncLib(t, a)
	write(t, filepath.Join(b, "skills", "other", "SKILL.md"), skillMD("other"))
	out := syncLib(t, b)
	if !out.Pulled || !out.Pushed || len(out.Conflicts) != 0 || out.Apply == nil {
		t.Fatalf("%+v", out)
	}
	if got := read(t, filepath.Join(b, "skills", "mine", "SKILL.md")); !strings.Contains(got, "one from a") {
		t.Errorf("not pulled:\n%s", got)
	}
	if files := gitOut(t, remote, "ls-tree", "-r", "--name-only", "HEAD"); !strings.Contains(files, "skills/other/SKILL.md") {
		t.Errorf("not pushed:\n%s", files)
	}
	// What was pulled is linked.
	if _, err := os.Lstat(filepath.Join(os.Getenv("HOME"), ".agents", "skills", "mine")); err != nil {
		t.Errorf("mine not linked: %v", err)
	}
}

// conflict makes a and b change the same line, a syncs first.
func conflict(t *testing.T) (a, b, remote string) {
	a, b, remote = twoMachines(t)
	write(t, filepath.Join(a, "skills", "mine", "SKILL.md"), strings.Replace(mineMD, "three", "three from a", 1))
	syncLib(t, a)
	write(t, filepath.Join(b, "skills", "mine", "SKILL.md"), strings.Replace(mineMD, "three", "three from b", 1))
	return a, b, remote
}

// The same line on both sides: the sync is shelved, b stays as it was and
// nothing is pushed.
func TestSyncConflictShelves(t *testing.T) {
	_, b, remote := conflict(t)
	remoteHead := gitOut(t, remote, "rev-parse", "HEAD")
	out := syncLib(t, b)
	if len(out.Conflicts) != 1 || out.Conflicts[0] != "skills/mine/SKILL.md" || out.Pushed || out.Pulled {
		t.Fatalf("%+v", out)
	}
	if got := read(t, filepath.Join(b, "skills", "mine", "SKILL.md")); !strings.Contains(got, "three from b") || strings.Contains(got, "<<<<<<<") {
		t.Errorf("b changed:\n%s", got)
	}
	if s, _ := app.Open(b); s.Syncing() {
		t.Error("a merge was left under way")
	}
	if st := gitOut(t, b, "status", "--porcelain"); st != "" {
		t.Errorf("status:\n%s", st)
	}
	if gitOut(t, remote, "rev-parse", "HEAD") != remoteHead {
		t.Error("pushed")
	}
}

func TestSyncResolveContinue(t *testing.T) {
	_, b, remote := conflict(t)
	syncLib(t, b)
	s, _ := app.Open(b)
	res, err := s.SyncResolve()
	if err != nil || len(res.Files) != 1 || len(res.Files[0].Blocks) != 1 || res.Files[0].Path != "skills/mine/SKILL.md" {
		t.Fatalf("resolve %+v %v", res, err)
	}
	if !s.Syncing() || !strings.Contains(gitOut(t, b, "status", "--porcelain"), "UU skills/mine/SKILL.md") {
		t.Fatalf("not in a merge:\n%s", gitOut(t, b, "status", "--porcelain"))
	}
	if _, err := s.Backup("", ""); err == nil {
		t.Error("synced while resolving")
	}
	_, err = s.SyncContinue()
	var ue *app.SyncUnresolvedError
	if !errors.As(err, &ue) || len(ue.Files) != 1 {
		t.Fatalf("continue with markers: %v", err)
	}
	if err := s.SyncPick("skills/mine/SKILL.md", 0, app.PickBoth); err != nil {
		t.Fatal(err)
	}
	if files, _ := s.SyncConflictFiles(); len(files) != 1 || !files[0].Resolved {
		t.Errorf("state %+v", files)
	}
	out, err := s.SyncContinue()
	if err != nil || !out.Pushed || !out.Pulled {
		t.Fatalf("continue %+v %v", out, err)
	}
	if s.Syncing() {
		t.Error("still merging")
	}
	got := gitOut(t, remote, "show", "HEAD:skills/mine/SKILL.md")
	if !strings.Contains(got, "three from b") || !strings.Contains(got, "three from a") {
		t.Errorf("remote:\n%s", got)
	}
	if parents := gitOut(t, remote, "log", "-1", "--format=%P", "HEAD"); len(strings.Fields(parents)) != 2 {
		t.Errorf("not a merge commit: %q", parents)
	}
}

func TestSyncResolveAbort(t *testing.T) {
	_, b, _ := conflict(t)
	syncLib(t, b)
	head := gitOut(t, b, "rev-parse", "HEAD")
	s, _ := app.Open(b)
	if _, err := s.SyncResolve(); err != nil {
		t.Fatal(err)
	}
	if err := s.SyncAbort(); err != nil {
		t.Fatal(err)
	}
	if s.Syncing() || gitOut(t, b, "rev-parse", "HEAD") != head {
		t.Error("not back")
	}
	if got := read(t, filepath.Join(b, "skills", "mine", "SKILL.md")); !strings.Contains(got, "three from b") || strings.Contains(got, "<<<<<<<") {
		t.Errorf("after abort:\n%s", got)
	}
}
