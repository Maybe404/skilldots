package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

func TestOwnSkillTrashRestoreRoundTrip(t *testing.T) {
	for _, private := range []bool{false, true} {
		t.Run(map[bool]string{false: "skills", true: "private"}[private], func(t *testing.T) {
			home, root := side(t, t.TempDir())
			s, err := app.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			created, err := s.New("mine", private)
			if err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(created.Path, "notes.txt"), "irreplaceable")
			if _, err := s.Remove(app.EntryRef(app.NewKind(private), "", "mine")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "mine")); !os.IsNotExist(err) {
				t.Fatalf("link remains: %v", err)
			}
			entries, err := s.Trash()
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || !entries[0].Restorable {
				t.Fatalf("trash: %+v", entries)
			}
			if _, err := s.Restore(entries[0].ID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Apply(app.ApplyOptions{}); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "mine", "notes.txt"))
			if err != nil || string(b) != "irreplaceable" {
				t.Fatalf("restore lost files: %q %v", b, err)
			}
		})
	}
}

func TestNewForTargetsDoesNotReachDefaultAgent(t *testing.T) {
	for _, names := range [][]string{{}, {"claude"}} {
		t.Run("selected-"+strings.Join(names, "-"), func(t *testing.T) {
			home, root := side(t, t.TempDir())
			s, err := app.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.NewForTargets("secret", true, names); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "secret")); !os.IsNotExist(err) {
				t.Fatalf("reached default agent: %v", err)
			}
			_, err = os.Lstat(filepath.Join(home, ".claude", "skills", "secret"))
			if len(names) > 0 && err != nil {
				t.Fatal(err)
			}
			if len(names) == 0 && !os.IsNotExist(err) {
				t.Fatalf("empty selection linked: %v", err)
			}
		})
	}
}
