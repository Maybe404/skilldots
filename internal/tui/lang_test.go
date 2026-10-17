package tui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/i18n"
)

// useLanguage makes l the language for the test and puts English back.
func useLanguage(t *testing.T, l i18n.Lang) {
	t.Helper()
	prev := i18n.Get()
	i18n.Set(l)
	t.Cleanup(func() { i18n.Set(prev) })
}

// cjkSnapshot is sampleSnapshot with names and descriptions in Chinese,
// the width the dashboard must measure in cells.
func cjkSnapshot() *Snapshot {
	s := sampleSnapshot()
	s.Packs[0].Alias = "测试驱动"
	s.Packs[0].Skills[0].Description = "把一个很长的想法追问到每个细节都清楚为止，再动手写代码，这样返工会少很多很多"
	s.Packs[0].Skills[1].Description = "测试驱动开发：先写失败的测试，再让它通过，最后重构 (red, green, refactor)"
	s.Mine[0].Description = "派生子任务的助手，负责把活儿拆开交给别的 agent 去做，并把结果收回来汇总给你看"
	s.Mine[0].LinkName = "子任务助手"
	s.Name = "技能库"
	return s
}

// checkFits fails when the view is not exactly h rows of w cells.
func checkFits(t *testing.T, what string, m Model, w, h int) {
	t.Helper()
	lines := plain(m)
	if len(lines) != h {
		t.Errorf("%s: %d lines, want %d", what, len(lines), h)
	}
	for i, l := range lines {
		if got := ansi.StringWidth(l); got != w {
			t.Errorf("%s: line %d is %d cells wide, want %d: %q", what, i, got, w, l)
		}
	}
}

// Chinese takes two cells a character: every screen of the dashboard in
// Chinese is still exactly the size of the terminal, at the widths the
// dashboard is offered on and above, with Chinese in the data too.
func TestChineseDashboardFitsTheScreen(t *testing.T) {
	lipgloss.SetColorProfile(0)
	useLanguage(t, i18n.ZH)
	for _, size := range [][2]int{{90, 24}, {100, 30}, {120, 36}, {200, 60}} {
		w, h := size[0], size[1]
		for _, snap := range []*Snapshot{sampleSnapshot(), cjkSnapshot()} {
			m, f := newModel(t, snap, w, h)
			f.history = []app.UpdateRecord{{Repo: "mattpocock/skills", From: "aaaaaaa1", To: "bbbbbbb2", Changed: []string{"tdd"}, Added: []string{"pr"}}}
			f.trash = []engine.TrashEntry{{ID: "20240101-000000/tdd", Name: "tdd", Origin: "/h/.agents/skills/tdd", Restorable: true}}
			checkFits(t, "start", m, w, h)
			// Each sequence opens one thing; the next closes it first.
			for _, keys := range [][]string{
				{"?"}, {"/", "t"}, {"j", "j", "e"}, {","}, {"T"}, {"h"}, {"t"}, {"u"}, {"S"}, {"a"}, {"n"}, {"s"},
				{"tab", "j", "f"}, {"o"}, {"d"}, {"tab", "j", "j", "d"},
			} {
				for i := 0; (m.ov != nil || m.filtering) && i < 4; i++ {
					m, _ = press(t, m, "esc")
				}
				var c tea.Cmd
				m, c = press(t, m, keys...)
				m = drain(t, m, c)
				checkFits(t, "after "+strings.Join(keys, " "), m, w, h)
				if t.Failed() {
					return
				}
			}
		}
	}
}

// The chrome is in Chinese and says what it said: keys, header, footer.
func TestChineseDashboardIsChinese(t *testing.T) {
	lipgloss.SetColorProfile(0)
	useLanguage(t, i18n.ZH)
	m, _ := newModel(t, sampleSnapshot(), 120, 36)
	v := strings.Join(plain(m), "\n")
	for _, want := range []string{"添加", "更新", "切换", "退出", "帮助", "已同步", "PACKS", "我的", "私有", "应用", "agent", "未安装", "SKILL", "分类"} {
		if !strings.Contains(v, want) {
			t.Errorf("no %q in the Chinese dashboard:\n%s", want, v)
		}
	}
	for _, english := range []string{" add ", " quit", "synced", "not installed", "MINE", "CATEGORY"} {
		if strings.Contains(v, english) {
			t.Errorf("English %q left in the Chinese dashboard:\n%s", english, v)
		}
	}
	// Help lists every key in Chinese.
	m, _ = press(t, m, "?")
	h := strings.Join(plain(m), "\n")
	for _, want := range []string{"按键", "移动", "用编辑器打开 skill"} {
		if !strings.Contains(h, want) {
			t.Errorf("no %q in the Chinese help:\n%s", want, h)
		}
	}
}

// Choosing a language on the settings page applies at once, to the whole
// dashboard, and is the setting that was saved.
func TestSettingsPageSwitchesLanguageAtOnce(t *testing.T) {
	lipgloss.SetColorProfile(0)
	useLanguage(t, i18n.EN)
	t.Setenv(i18n.Env, "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "en_US.UTF-8")
	m, f := newModel(t, sampleSnapshot(), 120, 40)
	m, cmd := press(t, m, ",")
	m = drain(t, m, cmd)
	// editor, editor-kind, theme, then lang.
	m, _ = press(t, m, "down", "down", "down", "enter")
	if m.ov == nil || m.ov.title != "Set lang" {
		t.Fatalf("enter on lang: %+v", m.ov)
	}
	m = typeText(t, m, "fr")
	m, _ = press(t, m, "enter")
	if m.ov == nil || !strings.Contains(m.ov.err, "use auto, en, zh") {
		t.Fatalf("fr accepted: %+v", m.ov)
	}
	m, _ = press(t, m, "backspace", "backspace")
	m = typeText(t, m, "zh")
	m, cmd = press(t, m, "enter")
	m = drain(t, m, cmd)
	if !slices.Contains(f.calls, "set lang=zh") {
		t.Fatalf("calls: %v", f.calls)
	}
	if i18n.Get() != i18n.ZH {
		t.Fatal("the language did not change")
	}
	v := strings.Join(plain(m), "\n")
	if !strings.Contains(v, "lang = zh") || !strings.Contains(v, "添加") || strings.Contains(v, " quit") {
		t.Errorf("the dashboard is not in Chinese:\n%s", v)
	}
	// And back, with auto following the system.
	m, cmd = press(t, m, ",")
	m = drain(t, m, cmd)
	m, _ = press(t, m, "down", "down", "down", "enter")
	if m.ov == nil || m.ov.title != "设置 lang" {
		t.Fatalf("settings in Chinese: %+v", m.ov)
	}
	m = typeText(t, m, "auto")
	m, cmd = press(t, m, "enter")
	m = drain(t, m, cmd)
	if i18n.Get() != i18n.EN {
		t.Error("auto with an English system must be English")
	}
	if v := strings.Join(plain(m), "\n"); !strings.Contains(v, "add") || strings.Contains(v, "添加") {
		t.Errorf("the dashboard is not back in English:\n%s", v)
	}
}

// The environment variable wins over the setting at the next start, and
// the status line says so.
func TestSettingsPageNamesTheEnvironmentOverride(t *testing.T) {
	useLanguage(t, i18n.EN)
	t.Setenv(i18n.Env, "en")
	if got := settingMsg("lang", "zh"); !strings.Contains(got, "$SKILLDOTS_LANG overrides it") {
		t.Errorf("%q", got)
	}
	t.Setenv(i18n.Env, "")
	if got := settingMsg("lang", "zh"); strings.Contains(got, "overrides") {
		t.Errorf("%q", got)
	}
}

// fit gives back exactly n cells, also when a double-width character
// would be cut in two.
func TestFitIsExactWithDoubleWidthText(t *testing.T) {
	for _, s := range []string{"abc", "一二三四五", "a一b二c三d", "测试驱动开发", ""} {
		for n := 0; n <= 12; n++ {
			if got := ansi.StringWidth(fit(s, n)); got != n {
				t.Errorf("fit(%q, %d) is %d cells: %q", s, n, got, fit(s, n))
			}
		}
	}
	if got := fit("abcdefgh", 5); got != "abcd…" {
		t.Errorf("English truncation changed: %q", got)
	}
}
