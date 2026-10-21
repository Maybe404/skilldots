package cli

import "testing"

// The multi-select prompts (adopt, update) say "space to toggle"; their
// key hints name space too, not x.
func TestToggleHintIsSpace(t *testing.T) {
	if h := keyMap().MultiSelect.Toggle.Help(); h.Key != "space" {
		t.Fatalf("toggle hint %q", h.Key)
	}
}
