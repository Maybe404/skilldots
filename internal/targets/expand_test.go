package targets

import (
	"slices"
	"testing"
)

// An agent that reads ~/.agents/skills too is not linked into its own
// directory by "*" when the list has the shared directory: it would load
// the skill twice. Named, or with nothing linked into the shared
// directory, it is.
func TestExpandNamesSkipsSharedReaders(t *testing.T) {
	present := []Target{
		{Name: "agents", Builtin: true},
		{Name: "claude", Builtin: true},
		{Name: "codex", Builtin: true, ReadsShared: true},
		{Name: "kiro", Builtin: true},
	}
	cases := []struct{ names, defaults, want []string }{
		{[]string{"*"}, []string{"agents"}, []string{"agents", "claude", "kiro"}},
		{[]string{"agents", "claude", "*"}, []string{"agents", "claude"}, []string{"agents", "claude", "kiro"}},
		{[]string{"codex", "*"}, []string{"agents"}, []string{"codex", "agents", "claude", "kiro"}},
		{[]string{"*"}, []string{"agents", "codex"}, []string{"agents", "codex", "claude", "kiro"}},
		{[]string{"*"}, []string{"claude"}, []string{"claude", "agents", "codex", "kiro"}},
	}
	for _, c := range cases {
		if got := ExpandNames(c.names, c.defaults, present); !slices.Equal(got, c.want) {
			t.Errorf("ExpandNames(%v, %v) = %v, want %v", c.names, c.defaults, got, c.want)
		}
	}
	if got := ExpandNamesUnshared([]string{"*"}, []string{"agents"}, present); !slices.Equal(got, []string{"agents", "claude", "codex", "kiro"}) {
		t.Errorf("unshared: %v", got)
	}
	// The registry marks the agents documented to read it.
	reg, err := NewRegistry("", nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"codex": true, "gemini": true, "cursor": true, "claude": false, "kiro": false, "agents": false, "trae": false} {
		if tg, err := reg.Get(name); err != nil || tg.ReadsShared != want {
			t.Errorf("%s: ReadsShared %v (%v), want %v", name, tg.ReadsShared, err, want)
		}
	}
}

func TestExpandNames(t *testing.T) {
	present := []Target{
		{Name: "claude", Builtin: true},
		{Name: "codex", Builtin: true},
		{Name: "team", Builtin: false}, // a custom target is never implied
	}
	cases := []struct{ names, want []string }{
		{[]string{"claude"}, []string{"claude"}},
		{[]string{}, []string{}},
		{[]string{"*"}, []string{"agents", "claude", "codex"}},
		{[]string{"agents", "*"}, []string{"agents", "claude", "codex"}},
		{[]string{"team", "*", "claude"}, []string{"team", "agents", "claude", "codex"}},
	}
	for _, c := range cases {
		if got := ExpandNames(c.names, []string{"agents"}, present); !slices.Equal(got, c.want) {
			t.Errorf("ExpandNames(%v) = %v, want %v", c.names, got, c.want)
		}
	}
}
