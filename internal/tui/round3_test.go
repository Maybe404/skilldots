package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/targets"
)

// The agents dialog says which agents read the skill through
// ~/.agents/skills; the skill's details call d what it does and the
// agents line a list of where it is installed; the trash shows each
// entry's reason and where it was, at 150 columns, in full.
func TestRoundThreeDashboard(t *testing.T) {
	s := sampleSnapshot()
	s.Agents = append(s.Agents, targets.Target{Name: "codex", Path: "/h/.codex/skills", Builtin: true, ReadsShared: true})
	for i := range s.Packs[0].Skills {
		s.Packs[0].Skills[i].ViaShared = []string{"codex"}
	}
	m, f := newModel(t, s, 150, 40)
	m, _ = press(t, m, "e")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "through agents (~/.agents/skills)") {
		t.Fatalf("agents dialog:\n%s", v)
	}
	m, _ = press(t, m, "esc", "enter")
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "d remove") || strings.Contains(v, "d unlink") || !strings.Contains(v, "installed for") {
		t.Errorf("details:\n%s", v)
	}

	when := time.Date(2026, 10, 21, 10, 0, 0, 0, time.Local)
	f.trash = []engine.TrashEntry{{ID: "20261021-100000/grill-me", Name: "grill-me", Skills: []string{"grill-me"},
		Reason: engine.ReasonInTheWay, Origin: "/h/.claude/skills/grill-me-with-a-rather-long-name", When: when, Restorable: true}}
	m, cmd := press(t, m, "t")
	m = drain(t, m, cmd)
	v = ansi.Strip(m.View())
	for _, want := range []string{"in the way in an agent directory", "/h/.claude/skills/grill-me-with-a-rather-long-name"} {
		if !strings.Contains(v, want) {
			t.Errorf("trash dialog lacks %q:\n%s", want, v)
		}
	}
}

// In Chinese too, at 150 columns, the trash dialog shows the reason and
// where the entry was in full.
func TestTrashDialogFitsInChinese(t *testing.T) {
	useLanguage(t, i18n.ZH)
	m, f := newModel(t, sampleSnapshot(), 150, 40)
	when := time.Date(2026, 10, 21, 10, 0, 0, 0, time.Local)
	f.trash = []engine.TrashEntry{{ID: "20261021-100000/grill-me", Name: "grill-me", Skills: []string{"grill-me"},
		Reason: engine.ReasonRemove, Repo: "mattpocock/skills", Origin: "/h/lib/vendor/mattpocock/skills/productivity/grill-me", When: when, Restorable: true}}
	m, cmd := press(t, m, "t")
	m = drain(t, m, cmd)
	v := ansi.Strip(m.View())
	for _, want := range []string{"skd remove（移除）", "/h/lib/vendor/mattpocock/skills/productivity/grill-me"} {
		if !strings.Contains(v, want) {
			t.Errorf("trash dialog lacks %q:\n%s", want, v)
		}
	}
}
