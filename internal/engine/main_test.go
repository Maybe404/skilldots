package engine

import (
	"fmt"
	"os"
	"testing"
)

// Every test runs with a home directory of its own (tests that need one
// set theirs too): skd keeps records beside the settings file
// (placed.json, local-trash.json, trash-notes.json), which must never be
// the real user's.
func TestMain(m *testing.M) {
	// A writer process of TestTrashNotesAcrossProcesses.
	if prefix := os.Getenv("SKD_NOTES_WRITER"); prefix != "" {
		for i := 0; i < notesPerWriter; i++ {
			if err := SetTrashNote(fmt.Sprintf("/trash/%s-%d", prefix, i), &TrashNote{Reason: ReasonRemove}); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		os.Exit(0)
	}
	home, err := os.MkdirTemp("", "skd-engine-home")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", home+"/.config")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
