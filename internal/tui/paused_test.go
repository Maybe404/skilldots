package tui

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

// The agents picker starts from every agent a skill is given: a paused
// agent stays selected, so saving the picker does not drop it.
func TestSnapshotKeepsPausedSelection(t *testing.T) {
	o, root, _, _ := acmeLibrary(t)
	write(t, filepath.Join(root, "skills", "mine", "SKILL.md"), skillMD("mine", "My own."))
	s, err := app.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetDefaultAgents([]string{"agents", "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err = o.PauseAgent("claude", true); err != nil {
		t.Fatal(err)
	}
	snap, err := o.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(snap.Paused, "claude") || len(snap.Mine) != 1 {
		t.Fatalf("snapshot: paused %v mine %d", snap.Paused, len(snap.Mine))
	}
	sk := snap.Mine[0]
	if slices.Contains(sk.Agents, "claude") || !slices.Contains(sk.Selected, "claude") {
		t.Fatalf("mine: agents %v selected %v", sk.Agents, sk.Selected)
	}
	rows, err := o.Targets()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Name == "claude" && (!r.Paused || !r.Default) {
			t.Fatalf("claude row: %+v", r)
		}
	}
}
