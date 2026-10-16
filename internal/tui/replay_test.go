package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestReplayKeys(t *testing.T) {
	var got []string
	for _, m := range replayKeys([]byte("?jq \r\t\x7f\x1b[A\x1bOB\x1b[5~\x1b[Z\x1bx\x03é\x1b[99X/\x1b")) {
		got = append(got, m.(tea.KeyMsg).String())
	}
	want := []string{"?", "j", "q", " ", "enter", "tab", "backspace", "up", "down", "pgup", "shift+tab", "alt+x", "ctrl+c", "é", "/", "esc"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}
