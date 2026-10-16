package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

// dropProposal scans the side's agent directories and returns the
// session and the "drop" proposal for the broken link at name.
func dropProposal(t *testing.T, root, name string) (*app.Session, []app.Proposal, *[]string) {
	t.Helper()
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	var logs []string
	s.Log = func(l app.Level, msg string) {
		if l == app.Warn {
			logs = append(logs, msg)
		}
	}
	rep, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	var drops []app.Proposal
	for _, p := range s.Proposals(rep) {
		if p.Kind == "drop" && p.Item.Name == name {
			drops = append(drops, p)
		}
	}
	if len(drops) != 1 {
		t.Fatalf("want one drop proposal for %s, got %+v", name, drops)
	}
	return s, drops, &logs
}

// A broken link the scan found may be replaced by real data before the
// user confirms; adopt must not delete what is there now.
func TestAdoptDropKeepsWhatReplacedTheBrokenLink(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := upstream(t)
	for _, tc := range []struct {
		name    string
		replace func(t *testing.T, path, home string)
	}{
		{"directory", func(t *testing.T, path, _ string) { write(t, filepath.Join(path, "notes.md"), "mine\n") }},
		{"empty directory", func(t *testing.T, path, _ string) {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"file", func(t *testing.T, path, _ string) { write(t, path, "mine\n") }},
		{"live link", func(t *testing.T, path, home string) {
			target := filepath.Join(home, "elsewhere")
			write(t, filepath.Join(target, "notes.md"), "mine\n")
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, root := side(t, up)
			path := filepath.Join(home, ".agents", "skills", "ghost")
			if err := os.Symlink(filepath.Join(home, "gone"), path); err != nil {
				t.Fatal(err)
			}
			s, props, logs := dropProposal(t, root, "ghost")
			// Between the scan and the confirmation the user puts
			// something real at the path.
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			tc.replace(t, path, home)
			if _, err := s.Adopt(props); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("%s was deleted: %v", path, err)
			}
			if !strings.Contains(strings.Join(*logs, "\n"), path) {
				t.Errorf("no warning about %s; got %q", path, *logs)
			}
		})
	}
	t.Run("still broken", func(t *testing.T) {
		home, root := side(t, up)
		path := filepath.Join(home, ".agents", "skills", "ghost")
		if err := os.Symlink(filepath.Join(home, "gone"), path); err != nil {
			t.Fatal(err)
		}
		s, props, _ := dropProposal(t, root, "ghost")
		if _, err := s.Adopt(props); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("broken link %s still there (err %v)", path, err)
		}
	})
}
