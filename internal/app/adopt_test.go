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
		// Not deleted: skd did not make it. It is in the trash, as it was.
		items, err := s.Trash()
		if err != nil || len(items) != 1 || items[0].Origin != path || items[0].Restorable {
			t.Fatalf("trash %+v %v", items, err)
		}
		if to, err := os.Readlink(items[0].Path); err != nil || to != filepath.Join(home, "gone") {
			t.Fatalf("the link in the trash: %q %v", to, err)
		}
	})
}

// When the agent directory itself is replaced by a link after the scan,
// the broken link the scan found is no longer there: what the path now
// leads to (someone's own dangling link elsewhere) is left alone.
func TestAdoptDropKeepsAnEntryOfAReplacedTargetDirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := upstream(t)
	home, root := side(t, up)
	agents := filepath.Join(home, ".agents", "skills")
	if err := os.Symlink(filepath.Join(home, "gone"), filepath.Join(agents, "ghost")); err != nil {
		t.Fatal(err)
	}
	s, props, logs := dropProposal(t, root, "ghost")
	other := filepath.Join(home, "other")
	os.MkdirAll(other, 0o755)
	os.Symlink(filepath.Join(home, "gone"), filepath.Join(other, "ghost"))
	os.Rename(agents, filepath.Join(home, "agents-was"))
	if err := os.Symlink(other, agents); err != nil {
		t.Fatal(err)
	}
	s.Adopt(props)
	if _, err := os.Lstat(filepath.Join(other, "ghost")); err != nil {
		t.Fatalf("removed through the replaced directory: %v", err)
	}
	if !strings.Contains(strings.Join(*logs, "\n"), "no longer an entry") {
		t.Errorf("no warning; got %q", *logs)
	}
}

// An entry adopted under another name (its SKILL.md names it otherwise)
// is moved aside once the link of its new name is in place, but only an
// entry of the target itself, while the target leads where it did: a
// proposal whose path is elsewhere, or a target directory replaced by a
// link since the library was opened, moves nothing.
func TestAdoptMovesAsideOnlyEntriesOfAnIntactTarget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	up := upstream(t)
	for _, tc := range []string{"moved aside", "elsewhere", "replaced target"} {
		t.Run(tc, func(t *testing.T) {
			home, root := side(t, up)
			agents := filepath.Join(home, ".agents", "skills")
			// The library has newname, linked for agents; the agent
			// directory also holds the same skill as oldname.
			write(t, filepath.Join(root, "skills", "newname", "SKILL.md"), skillMD("newname"))
			s, err := app.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Apply(app.ApplyOptions{}); err != nil {
				t.Fatal(err)
			}
			old := filepath.Join(agents, "oldname")
			write(t, filepath.Join(old, "SKILL.md"), skillMD("newname"))
			s, err = app.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			var logs []string
			s.Log = func(l app.Level, msg string) { logs = append(logs, msg) }
			rep, err := s.Scan()
			if err != nil {
				t.Fatal(err)
			}
			var props []app.Proposal
			for _, p := range s.Proposals(rep) {
				if p.Item.Name == "oldname" {
					props = append(props, p)
				}
			}
			// The same content as newname: it is that skill, installed
			// for agents already; this copy is moved aside.
			if len(props) != 1 || props[0].Kind != "enable" {
				t.Fatalf("proposals %+v", props)
			}
			victim := old
			switch tc {
			case "elsewhere":
				// The proposal names a directory outside the target.
				victim = filepath.Join(home, "Documents", "oldname")
				write(t, filepath.Join(victim, "SKILL.md"), skillMD("newname"))
				props[0].Item.Path = victim
			case "replaced target":
				// The target directory becomes a link to a copy of it.
				moved := filepath.Join(home, "agents-copy")
				if err := os.Rename(agents, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, agents); err != nil {
					t.Fatal(err)
				}
				victim = filepath.Join(moved, "oldname")
			}
			if _, err := s.Adopt(props); err != nil {
				t.Fatal(err)
			}
			_, err = os.Stat(filepath.Join(victim, "SKILL.md"))
			if tc == "moved aside" {
				if err == nil {
					t.Fatal("oldname not moved aside")
				}
				return
			}
			if err != nil {
				t.Fatalf("moved %s: %v", victim, err)
			}
			if !strings.Contains(strings.Join(logs, "\n"), "not moved aside") {
				t.Errorf("no warning: %q", logs)
			}
		})
	}
}
