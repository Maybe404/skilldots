package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
)

// restoreSide is a library subscribed to acme/skills (alias acme) with
// tdd, review and grill selected, review linked only into Claude.
func restoreSide(t *testing.T) (home, root string, s *app.Session) {
	t.Helper()
	home, root = side(t, upstream(t))
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	src, err := s.ParseAdd("acme/skills", "acme")
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Fetch(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(f, app.Selection{Skills: []string{"tdd", "review", "grill"}, Alias: "acme"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enable(app.KeyRef("review"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Disable(app.KeyRef("review"), []string{"agents"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enable(app.KeyRef("review"), []string{"claude"}); err != nil {
		t.Fatal(err)
	}
	return home, root, s
}

func reopen(t *testing.T, root string) *app.Session {
	t.Helper()
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// skd remove, then skd trash restore <skill>: the skill is selected again
// in skills.toml with its own targets, recorded in the lock and linked,
// without a separate apply.
func TestRestoreUndoesRemove(t *testing.T) {
	home, root, s := restoreSide(t)
	before := state(t, home, root)
	out, err := s.Remove(app.KeyRef("review"))
	if err != nil {
		t.Fatal(err)
	}
	if out.Trash == "" || out.Purged != nil {
		t.Fatalf("remove: %+v", out)
	}
	s = reopen(t, root)
	items, err := s.Trash()
	if err != nil || len(items) != 1 {
		t.Fatalf("trash: %+v %v", items, err)
	}
	if it := items[0]; it.ID != out.Trash || strings.Join(it.Skills, ",") != "review" || it.Reason != engine.ReasonRemove || it.Repo != "acme/skills" {
		t.Fatalf("trash entry: %+v", it)
	}
	if app.TrashReason(items[0]) == "" {
		t.Fatal("no reason shown")
	}
	r, err := s.RestoreEntry("review")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Selected || r.Unused || r.Apply == nil || r.Hint != "" {
		t.Fatalf("restore: %+v", r)
	}
	if after := state(t, home, root); !sameState(before, after) {
		t.Fatalf("restore did not undo the remove:\nbefore %v\nafter  %v", before, after)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "review")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "review")); !os.IsNotExist(err) {
		t.Fatalf("review reached agents: %v", err)
	}
}

// Removing a pack's last skill drops the pack; restoring it brings the
// pack back with its alias, selection and links.
func TestRestoreUndoesRemoveOfTheLastSkill(t *testing.T) {
	home, root, s := restoreSide(t)
	for _, n := range []string{"tdd", "grill"} {
		if _, err := s.Remove(app.KeyRef(n)); err != nil {
			t.Fatal(err)
		}
	}
	s = reopen(t, root)
	before := state(t, home, root)
	out, err := s.Remove(app.KeyRef("review"))
	if err != nil {
		t.Fatal(err)
	}
	if out.Purged == nil || out.Trash == "" || out.Trash != out.Purged.Trash {
		t.Fatalf("remove: %+v", out)
	}
	// The links it takes out are listed: not "0 links".
	if strings.Join(out.Purged.Unlinked, ",") != "claude/review" {
		t.Fatalf("unlinked: %v", out.Purged.Unlinked)
	}
	if !strings.HasSuffix(out.Trash, "/acme-skills") {
		t.Fatalf("trash id %q is not named after the repository", out.Trash)
	}
	s = reopen(t, root)
	if s.Lib.Manifest.FindPack("acme/skills") != nil {
		t.Fatal("pack not dropped")
	}
	r, err := s.RestoreEntry(out.Trash)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Selected || r.Unused {
		t.Fatalf("restore: %+v", r)
	}
	if after := state(t, home, root); !sameState(before, after) {
		t.Fatalf("restore did not undo the remove:\nbefore %v\nafter  %v", before, after)
	}
	if p := reopen(t, root).Lib.Manifest.FindPack("acme"); p == nil || p.Repo != "acme/skills" {
		t.Fatalf("alias not restored: %+v", p)
	}
}

// skd purge, then restoring its entry: the pack comes back as it was.
func TestRestoreUndoesPurge(t *testing.T) {
	home, root, s := restoreSide(t)
	before := state(t, home, root)
	out, err := s.Purge("acme/skills")
	if err != nil {
		t.Fatal(err)
	}
	s = reopen(t, root)
	items, _ := s.Trash()
	if len(items) != 1 || items[0].ID != out.Trash || items[0].Reason != engine.ReasonPurge || strings.Join(items[0].Skills, ",") != "grill,review,tdd" {
		t.Fatalf("trash: %+v", items)
	}
	// Any one of its skills finds it.
	if _, err := s.RestoreEntry("grill"); err != nil {
		t.Fatal(err)
	}
	if after := state(t, home, root); !sameState(before, after) {
		t.Fatalf("restore did not undo the purge:\nbefore %v\nafter  %v", before, after)
	}
}

// A skill name held by several entries is not guessed: the error lists
// the IDs.
func TestRestoreByNameIsNeverAGuess(t *testing.T) {
	_, root, s := restoreSide(t)
	if _, err := s.Remove(app.KeyRef("tdd")); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, root)
	if _, err := s.RestoreEntry("tdd"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Remove(app.KeyRef("tdd")); err != nil {
		t.Fatal(err)
	}
	// A second entry holding tdd, set aside some other way.
	s = reopen(t, root)
	dir := filepath.Join(root, engine.TrashDir, "20200101-000000", "tdd")
	write(t, filepath.Join(dir, "SKILL.md"), skillMD("tdd"))
	write(t, dir+".origin", filepath.Join(root, "vendor", "acme", "skills", "eng", "tdd")+"\n")
	_, err := s.RestoreEntry("tdd")
	if err == nil || !strings.Contains(err.Error(), "20200101-000000/tdd") {
		t.Fatalf("restore by an ambiguous name: %v", err)
	}
	if _, err := s.RestoreEntry("nothing-like-it"); err == nil {
		t.Fatal("restored a name nothing holds")
	}
}

// An entry set aside before skd kept notes comes back into vendor/ but
// nothing selects it: the outcome says what to run.
func TestRestoreWithoutANoteSaysWhatToRun(t *testing.T) {
	home, root, s := restoreSide(t)
	out, err := s.Remove(app.KeyRef("tdd"))
	if err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(home, ".config", "skilldots", "trash-notes.json")
	if _, err := os.Stat(notes); err != nil {
		// macOS keeps settings in Library/Application Support.
		notes = filepath.Join(home, "Library", "Application Support", "skilldots", "trash-notes.json")
	}
	if err := os.Remove(notes); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, root)
	r, err := s.RestoreEntry(out.Trash)
	if err != nil {
		t.Fatal(err)
	}
	if r.Selected || !r.Unused || !strings.Contains(r.Hint, "skd add") {
		t.Fatalf("restore: %+v", r)
	}
}

// A skill of your own comes back linked, without a separate apply.
func TestRestoreLinksAnOwnSkillAgain(t *testing.T) {
	home, root := side(t, t.TempDir())
	s := reopen(t, root)
	if _, err := s.New("mine", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Remove(app.KeyRef("mine")); err != nil {
		t.Fatal(err)
	}
	s = reopen(t, root)
	r, err := s.RestoreEntry("mine")
	if err != nil {
		t.Fatal(err)
	}
	if r.Unused || r.Apply == nil {
		t.Fatalf("restore: %+v", r)
	}
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "mine")); err != nil {
		t.Fatalf("not linked again: %v", err)
	}
}

func sameState(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
