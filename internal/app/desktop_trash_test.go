package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

func TestTrashUsageAndDeleteEntry(t *testing.T) {
	_, root := side(t, t.TempDir())
	for _, n := range []string{"one", "two"} {
		runCLI(t, root, "new", n)
	}
	ses, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"one", "two"} {
		if _, err := ses.Remove(app.KeyRef("skills:" + n)); err != nil {
			t.Fatal(err)
		}
	}
	u, err := ses.TrashUsage()
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Entries) != 2 || u.Total <= 0 {
		t.Fatalf("usage: %+v", u)
	}
	var sum int64
	for _, e := range u.Entries {
		if u.Sizes[e.ID] <= 0 {
			t.Fatalf("no size for %s", e.ID)
		}
		sum += u.Sizes[e.ID]
	}
	if u.Total < sum {
		t.Fatalf("total %d < entries %d", u.Total, sum)
	}

	for _, bad := range []string{"", "../x", "nope/one", u.Entries[0].ID + "/.."} {
		if err := ses.DeleteTrashEntry(bad); err == nil {
			t.Fatalf("deleted %q", bad)
		}
	}
	first := u.Entries[0]
	if err := ses.DeleteTrashEntry(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(first.Path); !os.IsNotExist(err) {
		t.Fatal("entry still there")
	}
	if _, err := os.Lstat(first.Path + ".origin"); !os.IsNotExist(err) {
		t.Fatal("record still there")
	}
	left, _ := ses.Trash()
	if len(left) != 1 || left[0].ID == first.ID {
		t.Fatalf("left: %+v", left)
	}
}

// A batch reached through a symbolic link is not deleted through it.
func TestDeleteTrashEntryRefusesLinkedBatch(t *testing.T) {
	_, root := side(t, t.TempDir())
	runCLI(t, root, "new", "kept")
	ses, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ses.Remove(app.KeyRef("skills:kept")); err != nil {
		t.Fatal(err)
	}
	entries, _ := ses.Trash()
	if len(entries) != 1 {
		t.Fatalf("%+v", entries)
	}
	batch := filepath.Dir(entries[0].Path)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Rename(batch, elsewhere); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, batch); err != nil {
		t.Fatal(err)
	}
	if err := ses.DeleteTrashEntry(entries[0].ID); err == nil {
		t.Fatal("deleted through a link")
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "kept", "SKILL.md")); err != nil {
		t.Fatal("the linked directory lost its content")
	}
}
