package cli_test

import (
	"strings"
	"testing"
)

// What skd prints for a person can be read without a legend: the apply
// summary in words, a disable that changes nothing says why, a skill
// named where a pack is expected says how to look at it, and paths in
// the home directory are written ~/… everywhere.
func TestOutputSaysWhatItMeans(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	e.skill("up/skills/grill-me", "grill-me")
	up := e.repo("up")
	for _, lang := range []string{"en", "zh"} {
		e.setLang(lang)
		want := map[string][]string{
			"en": {"applied · linked 1 · kept 0", "grill-me was not installed for codex; nothing to take off",
				"grill-me was already installed for agents", "no pack \"grill-me\"; grill-me is a skill of", "skd path grill-me"},
			"zh": {"已应用 · 新链接 1 个 · 保持 0 个", "grill-me 本来就没有装在 codex 上，没有可卸下的",
				"grill-me 已经装在 agents 上了", "没有来源 \"grill-me\"；grill-me 是", "skd path grill-me"},
		}[lang]
		if lang == "zh" {
			e.ok(lib, "purge", up, "--yes")
		}
		out := e.ok(lib, "add", up, "--skill", "grill-me")
		if !strings.Contains(out, want[0]) || strings.Contains(out, "↻") {
			t.Errorf("%s: add:\n%s", lang, out)
		}
		if out := e.ok(lib, "disable", "grill-me", "--agent", "codex"); !strings.Contains(out, want[1]) {
			t.Errorf("%s: disable:\n%s", lang, out)
		}
		if out := e.ok(lib, "enable", "grill-me", "--agent", "agents"); !strings.Contains(out, want[2]) {
			t.Errorf("%s: enable:\n%s", lang, out)
		}
		_, errOut, code := e.skd(lib, "list", "grill-me")
		if code == 0 || !strings.Contains(errOut, want[3]) || !strings.Contains(errOut, want[4]) {
			t.Errorf("%s: list grill-me: exit %d\n%s", lang, code, errOut)
		}
		if out := e.ok(lib, "targets"); strings.Contains(out, e.home) || !strings.Contains(out, "~/.agents/skills") {
			t.Errorf("%s: targets:\n%s", lang, out)
		}
		if out := e.ok(lib, "list", "--agent", "agents"); !strings.HasPrefix(out, "agents ~/.agents/skills\n") {
			t.Errorf("%s: list --agent:\n%s", lang, out)
		}
	}
}
