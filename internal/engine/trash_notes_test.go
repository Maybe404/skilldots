package engine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// notesPerWriter is how many notes each writer process sets.
const notesPerWriter = 40

// Several skd processes setting notes at once (the desktop app and an
// agent's skd) lose none of them.
func TestTrashNotesAcrossProcesses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	writers := []string{"a", "b", "c", "d"}
	var cmds []*exec.Cmd
	for _, w := range writers {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), "SKD_NOTES_WRITER="+w)
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
	}
	for _, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	m, err := readTrashNotes()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range writers {
		for i := 0; i < notesPerWriter; i++ {
			if m[noteKey(fmt.Sprintf("/trash/%s-%d", w, i))] == nil {
				t.Fatalf("note %s-%d lost (%d of %d kept)", w, i, len(m), len(writers)*notesPerWriter)
			}
		}
	}
}

// A notes file that is not valid JSON is not written over: it is kept
// aside, the error says where, and a new one is started.
func TestBrokenTrashNotesAreKept(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	p := trashNotesPath()
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("{not json"), 0o644)
	err := SetTrashNote("/trash/x", &TrashNote{Reason: ReasonPurge})
	var broken *BrokenNotesError
	if !errors.As(err, &broken) || broken.Backup == "" || !strings.Contains(err.Error(), broken.Backup) {
		t.Fatalf("%v", err)
	}
	if b, err := os.ReadFile(broken.Backup); err != nil || string(b) != "{not json" {
		t.Fatalf("backup: %q %v", b, err)
	}
	if n := TrashNoteFor("/trash/x"); n == nil || n.Reason != ReasonPurge {
		t.Fatalf("new note: %+v", n)
	}
	// Reading alone never moves the file.
	os.WriteFile(p, []byte("{not json"), 0o644)
	if TrashNoteFor("/trash/x") != nil {
		t.Fatal("read a broken file")
	}
	if b, _ := os.ReadFile(p); string(b) != "{not json" {
		t.Fatal("a read changed the file")
	}
}
