package tui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A custom target whose directory is not named skills is approved in the
// dashboard only by typing its name after a warning: y and enter alone
// do not.
func TestTargetsViewTypesTheNameOfAnUnusualDirectory(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 40)
	f.targets = []TargetRow{
		{Name: "agents", Path: "/h/.agents/skills", Raw: "/h/.agents/skills", Mode: "link", Builtin: true, Active: true},
		{Name: "ext", Raw: "~/.my-editor/plugins", Path: "/h/.my-editor/plugins", Mode: "link", Approvable: true, Problem: "not approved"},
	}
	var cmd tea.Cmd
	m, cmd = press(t, m, "T")
	m = settle(t, m, cmd)
	m, _ = press(t, m, "a")
	if !strings.Contains(m.view(), "not named skills") || !strings.Contains(m.view(), "/h/.my-editor/plugins") {
		t.Fatalf("no warning:\n%s", m.view())
	}
	m, _ = press(t, m, "enter")
	if m.ov == nil || m.ov.kind != ovConfirm {
		t.Fatal("enter approved or closed the warning")
	}
	m, _ = press(t, m, "y")
	if m.ov == nil || m.ov.kind != ovInput {
		t.Fatalf("y does not ask for the name:\n%s", m.view())
	}
	for _, wrong := range []string{"y", "Ext", "yes"} {
		m = typeText(t, m, wrong)
		m, _ = press(t, m, "enter")
		if m.ov == nil || m.ov.err == "" {
			t.Fatalf("%q accepted", wrong)
		}
		for range wrong {
			m, _ = press(t, m, "backspace")
		}
	}
	m = typeText(t, m, "ext")
	m, cmd = press(t, m, "enter")
	settle(t, m, cmd)
	if !slices.Contains(f.calls, "trust ext /h/.my-editor/plugins link typed=ext") {
		t.Fatalf("calls: %v", f.calls)
	}
	if n := len(slices.DeleteFunc(slices.Clone(f.calls), func(c string) bool { return !strings.HasPrefix(c, "trust ") })); n != 1 {
		t.Fatalf("trusted %d times: %v", n, f.calls)
	}
}
