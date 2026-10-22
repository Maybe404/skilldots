package app_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/lock"
)

// upstreamMove moves a directory in a bare upstream through a clone, and
// writes files (relative to the repository) in the same commit.
func upstreamMove(t *testing.T, bare, from, to string, files map[string]string) {
	t.Helper()
	w := filepath.Join(t.TempDir(), "w")
	git(t, filepath.Dir(w), "clone", "-q", bare, "w")
	os.MkdirAll(filepath.Dir(filepath.Join(w, filepath.FromSlash(to))), 0o755)
	git(t, w, "mv", from, to)
	for rel, body := range files {
		write(t, filepath.Join(w, filepath.FromSlash(rel)), body)
	}
	commit(t, w, "move "+from+" to "+to)
	git(t, w, "push", "-q", "origin", "HEAD")
}

// nestedMove is a library following differential-review of acme/skills,
// vendored from upstream's directory from (skills/differential-review or
// plugins/differential-review/skills/differential-review: category "" or
// "differential-review"), so the two vendored places nest.
func nestedMove(t *testing.T, from string) (bare, root, home string) {
	t.Helper()
	up := upstream(t)
	repo := filepath.Join(up, "acme", "skills.git")
	write(t, filepath.Join(repo, filepath.FromSlash(from), "SKILL.md"), longSkill("differential-review", "top", "bottom"))
	commit(t, repo, "differential-review")
	bare = bareUpstream(t, repo)
	home, root = side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "differential-review")
	return bare, root, home
}

const (
	flatDR   = "skills/differential-review"
	nestedDR = "plugins/differential-review/skills/differential-review"
)

// checkMoved checks the library after differential-review moved to
// vendor path now: its directory, the lock, the link and the upstream
// snapshot all follow it, and nothing is left behind.
func checkMoved(t *testing.T, root, home, now, category string) {
	t.Helper()
	pd := filepath.Join(root, "vendor", "acme", "skills")
	dir := filepath.Join(pd, filepath.FromSlash(now))
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatalf("not at %s: %v", now, err)
	}
	filepath.WalkDir(pd, func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.Contains(d.Name(), ".skd-") {
			t.Errorf("left behind: %s", p)
		}
		return nil
	})
	if ls := lockSkill(t, root, "differential-review"); ls.Category != category {
		t.Errorf("lock category %q, want %q", ls.Category, category)
	}
	link := filepath.Join(home, ".agents", "skills", "differential-review")
	if got, err := filepath.EvalSymlinks(link); err != nil || got != dir {
		t.Errorf("link %s leads to %q (%v), want %s", link, got, err, dir)
	}
	l, err := lock.Load(filepath.Join(root, lock.FileName))
	if err != nil {
		t.Fatal(err)
	}
	for k, ln := range l.Links {
		if strings.HasSuffix(k, "/differential-review") && ln.Source != filepath.Join("vendor", "acme", "skills", filepath.FromSlash(now)) {
			t.Errorf("link record %s: source %q", k, ln.Source)
		}
	}
	snap, err := engine.ReadGitFileSet(context.Background(), root, "refs/skilldots/upstream/acme/skills", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"LICENSE", now + "/SKILL.md"}; !slices.Equal(snap.Paths(), want) {
		t.Errorf("snapshot %v, want %v", snap.Paths(), want)
	}
}

// Upstream moved a skill into a directory of its own name, so its new
// vendored place is inside the old one (x/ to x/x/), or the other way
// (x/x/ to x/). The directory is renamed in place, not into itself.
func TestUpdateMovesSkillIntoOrOutOfItsOwnDirectory(t *testing.T) {
	for _, tc := range []struct {
		name, from, to, was, now, category string
	}{
		{"into", flatDR, nestedDR, "differential-review", "differential-review/differential-review", "differential-review"},
		{"out of", nestedDR, flatDR, "differential-review/differential-review", "differential-review", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bare, root, home := nestedMove(t, tc.from)
			pd := filepath.Join(root, "vendor", "acme", "skills")
			st0, err := os.Stat(filepath.Join(pd, filepath.FromSlash(tc.was), "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			upstreamMove(t, bare, tc.from, tc.to, nil)

			s, _ := app.Open(root)
			out, err := s.Update(nil, nil)
			if err != nil || len(out.Failed) != 0 || len(out.Reports) != 1 {
				t.Fatalf("update: %v %+v", err, out)
			}
			rep := out.Reports[0]
			if len(rep.Moved) != 1 || !slices.Equal(rep.Unchanged, []string{"differential-review"}) || len(rep.Changed)+len(rep.Backups) != 0 {
				t.Fatalf("moved %v unchanged %v changed %v backups %v", rep.Moved, rep.Unchanged, rep.Changed, rep.Backups)
			}
			st1, err := os.Stat(filepath.Join(pd, filepath.FromSlash(tc.now), "SKILL.md"))
			if err != nil || !os.SameFile(st0, st1) {
				t.Fatal("vendored directory was not renamed in place")
			}
			if _, err := os.Stat(filepath.Join(root, engine.TrashDir)); !os.IsNotExist(err) {
				t.Error("the move put something in the trash")
			}
			checkMoved(t, root, home, tc.now, tc.category)

			// Nothing is left to do.
			rep = update(t, root)
			if len(rep.Moved)+len(rep.Changed)+len(rep.Backups) != 0 {
				t.Errorf("second update: %+v", rep)
			}
		})
	}
}

// The same moves of a skill the user changed, with an upstream change in
// the same commit: the copy moves, and upstream's change is merged in.
func TestUpdateMergesSkillMovedIntoOrOutOfItsOwnDirectory(t *testing.T) {
	for _, tc := range []struct {
		name, from, to, was, now, category string
	}{
		{"into", flatDR, nestedDR, "differential-review", "differential-review/differential-review", "differential-review"},
		{"out of", nestedDR, flatDR, "differential-review/differential-review", "differential-review", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bare, root, home := nestedMove(t, tc.from)
			was := filepath.Join(root, "vendor", "acme", "skills", filepath.FromSlash(tc.was))
			write(t, filepath.Join(was, "SKILL.md"), longSkill("differential-review", "top mine", "bottom"))
			write(t, filepath.Join(was, "notes.md"), "my notes\n")
			upstreamMove(t, bare, tc.from, tc.to, map[string]string{
				tc.to + "/SKILL.md": longSkill("differential-review", "top", "bottom upstream")})

			rep := update(t, root)
			if !slices.Equal(rep.Merged, []string{"differential-review"}) || len(rep.Shelved) != 0 || len(rep.Moved) != 1 {
				t.Fatalf("merged %v shelved %v moved %v", rep.Merged, rep.Shelved, rep.Moved)
			}
			now := filepath.Join(root, "vendor", "acme", "skills", filepath.FromSlash(tc.now))
			if got := read(t, filepath.Join(now, "SKILL.md")); got != longSkill("differential-review", "top mine", "bottom upstream") {
				t.Errorf("merged SKILL.md:\n%s", got)
			}
			if read(t, filepath.Join(now, "notes.md")) != "my notes\n" {
				t.Error("a local file was lost")
			}
			theirs, err := engine.ReadGitFileSet(context.Background(), bare, "HEAD", tc.to)
			if err != nil {
				t.Fatal(err)
			}
			if ls := lockSkill(t, root, "differential-review"); ls.Hash != theirs.Hash() || ls.Shelved != "" {
				t.Errorf("lock %+v", ls)
			}
			checkMoved(t, root, home, tc.now, tc.category)
		})
	}
}
