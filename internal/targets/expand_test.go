package targets

import (
	"slices"
	"testing"
)

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
