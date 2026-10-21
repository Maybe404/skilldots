package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
)

// settingsFile is a file beside the settings (config.toml) of the side.
func settingsFile(t *testing.T, name string) string {
	t.Helper()
	d, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(d, "skilldots", name)
}

// An entry deleted for good takes what skd remembered about it along: a
// directory set aside later under the same name, in the same batch, is
// not shown as the removed skill.
func TestDeletedTrashEntryLeavesNoRecord(t *testing.T) {
	home, root, s := restoreSide(t)
	out, err := s.Remove(app.KeyRef("review"))
	if err != nil {
		t.Fatal(err)
	}
	s = reopen(t, root)
	if err := s.DeleteTrashEntry(out.Trash); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(settingsFile(t, "trash-notes.json")); strings.Contains(string(b), "review") {
		t.Fatalf("the note stayed:\n%s", b)
	}
	// The same name set aside again, in the same batch.
	mine := filepath.Join(home, ".claude", "skills", "review")
	write(t, filepath.Join(mine, "SKILL.md"), skillMD("review"))
	to, err := engine.Trash(s.Lib, mine)
	if err != nil {
		t.Fatal(err)
	}
	if id := engine.TrashID(s.Lib, to); id != out.Trash {
		t.Fatalf("not the same name: %s and %s", id, out.Trash)
	}
	items, _ := reopen(t, root).Trash()
	if len(items) != 1 || items[0].Reason == engine.ReasonRemove || items[0].Repo != "" {
		t.Fatalf("the new entry looks like the removed skill: %+v", items)
	}
}

// A skill of your own deleted for good from the trash, or the whole trash
// emptied, leaves no provenance record behind.
func TestDeletedOwnSkillLeavesNoProvenance(t *testing.T) {
	_, root := side(t, t.TempDir())
	s := reopen(t, root)
	for _, n := range []string{"one", "two"} {
		if _, err := s.New(n, false); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Remove(app.KeyRef(n)); err != nil {
			t.Fatal(err)
		}
		s = reopen(t, root)
	}
	items, _ := s.Trash()
	if len(items) != 2 {
		t.Fatalf("%+v", items)
	}
	if err := s.DeleteTrashEntry(items[0].ID); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(settingsFile(t, "local-trash.json"))
	if strings.Contains(string(b), items[0].ID) || !strings.Contains(string(b), items[1].ID) {
		t.Fatalf("local-trash.json after deleting one:\n%s", b)
	}
	if _, err := s.EmptyTrash(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"local-trash.json", "trash-notes.json"} {
		if b, _ := os.ReadFile(settingsFile(t, f)); strings.Contains(string(b), engine.TrashDir) {
			t.Fatalf("%s after emptying:\n%s", f, b)
		}
	}
}
