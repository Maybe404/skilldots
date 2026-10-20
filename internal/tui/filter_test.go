package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/Maybe404/skilldots/internal/pack"
)

// The add picker of a repository with many skills filters with /: what
// is typed narrows the list (name, description or category), space
// ticks what is shown, enter keeps the filter, esc clears it, and
// whatever was ticked is added, hidden or not.
func TestAddPickerFilters(t *testing.T) {
	m, f := newModel(t, sampleSnapshot(), 120, 36)
	var skills []pack.Skill
	for i := 0; i < 38; i++ {
		cat := []string{"engineering", "productivity", "writing"}[i%3]
		skills = append(skills, pack.Skill{Name: fmt.Sprintf("skill-%02d", i), Category: cat, Description: "does thing " + fmt.Sprint(i)})
	}
	skills = append(skills, pack.Skill{Name: "grill-me", Category: "productivity", Description: "interview the user"})
	f.browse = &Browse{Repo: "mattpocock/skills", Commit: "abcdef0123", Have: map[string]bool{}, Skills: skills}
	m, _ = press(t, m, "a")
	m = typeText(t, m, "mattpocock/skills")
	m, cmd := press(t, m, "enter")
	m = drain(t, m, cmd)
	if m.ov == nil || m.ov.kind != ovPick {
		t.Fatalf("no picker: %+v", m.ov)
	}
	if v := ansi.Strip(m.View()); strings.Contains(v, "grill-me") || !strings.Contains(v, "/ filter") {
		t.Fatalf("grill-me shown before filtering, or no hint:\n%s", v)
	}
	// q and j are text while the filter is typed, not keys.
	m, _ = press(t, m, "/")
	m = typeText(t, m, "gril")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "grill-me") || strings.Contains(v, "skill-01") || !strings.Contains(v, "/ gril") || !strings.Contains(v, "1 of 39 shown") {
		t.Fatalf("filtered view:\n%s", v)
	}
	if it := m.ov.items[m.ov.cur]; it.label != "grill-me" {
		t.Fatalf("cursor on %+v", it)
	}
	m, _ = press(t, m, "enter", "space")
	if !m.ov.items[m.ov.cur].on {
		t.Fatal("space did not tick the filtered row")
	}
	// esc clears the filter first; the dialog stays open.
	m, _ = press(t, m, "esc")
	if m.ov == nil || m.ov.filter != "" || !strings.Contains(ansi.Strip(m.View()), "skill-01") {
		t.Fatalf("esc did not clear the filter: %+v", m.ov)
	}
	// A filter by description, a category heading ticks only what it shows.
	m, _ = press(t, m, "/")
	m = typeText(t, m, "thing 1")
	m, _ = press(t, m, "enter", "home")
	if it := m.ov.items[m.ov.cur]; !it.header {
		t.Fatalf("cursor on %+v, want a heading", it)
	}
	m, _ = press(t, m, "space")
	ticked := 0
	for _, it := range m.ov.items {
		if it.on {
			ticked++
		}
	}
	// grill-me, and skill-01 / 10–19 under the first heading shown.
	if ticked < 2 || ticked > 12 {
		t.Fatalf("heading ticked %d", ticked)
	}
	m, _ = press(t, m, "/")
	m = typeText(t, m, "nothing-like-this")
	if !strings.Contains(ansi.Strip(m.View()), "nothing matches the filter") {
		t.Fatal("no empty note")
	}
	m, _ = press(t, m, "esc", "enter")
	if m.ov == nil || m.ov.title != "Alias" {
		t.Fatalf("enter did not confirm: %+v", m.ov)
	}
	m, cmd = press(t, m, "enter", "enter")
	m = drain(t, m, cmd)
	if got := lastCall(f); !strings.Contains(got, "grill-me") || !strings.HasPrefix(got, "add mattpocock/skills ") {
		t.Fatalf("add call = %q", got)
	}
}
