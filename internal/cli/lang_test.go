package cli_test

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Maybe404/skilldots/internal/cli"
	"github.com/Maybe404/skilldots/internal/i18n"
)

// langs are the languages the gates and the output tests run in.
var langs = []string{"en", "zh"}

// words are what the tests look for in the output, where it is not
// English.
var words = map[string]map[string]string{
	"zh": {"cannot be restored": "无法恢复", "control characters": "控制字符"},
}

// word is w as skd says it in the language of the environment.
func (e *env) word(w string) string {
	if x, ok := words[e.lang][w]; ok {
		return x
	}
	return w
}

var update = flag.Bool("update", false, "rewrite the golden files of the Chinese output")

func hasHan(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0
}

// langProbe says which language skd speaks: `skd trash` of a library with
// nothing set aside.
func langProbe(t *testing.T, e *env, lib string, args ...string) string {
	t.Helper()
	out, errOut, code := e.skd(lib, append([]string{"--library", lib, "trash"}, args...)...)
	if code != 0 {
		t.Fatalf("skd trash %v: exit %d\n%s%s", args, code, out, errOut)
	}
	switch strings.TrimSpace(out) {
	case "trash is empty":
		return "en"
	case "回收站是空的":
		return "zh"
	}
	t.Fatalf("skd trash %v: neither language:\n%s", args, out)
	return ""
}

// --lang, then $SKILLDOTS_LANG, then the lang setting, then the language
// of the system (the first of LC_ALL, LC_MESSAGES and LANG that is set).
func TestLanguageSelectionOrder(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	for _, c := range []struct {
		name                    string
		flag, env, setting      string
		lcAll, lcMessages, lang string
		want                    string
	}{
		{"flag beats env, setting and locale", "zh", "en", "en", "", "", "en_US.UTF-8", "zh"},
		{"flag en beats all zh", "en", "zh", "zh", "zh_CN.UTF-8", "", "zh_CN.UTF-8", "en"},
		{"env beats setting and locale", "", "zh", "en", "", "", "en_US.UTF-8", "zh"},
		{"env en beats setting zh", "", "en", "zh", "", "", "zh_CN.UTF-8", "en"},
		{"setting beats locale", "", "", "zh", "", "", "en_US.UTF-8", "zh"},
		{"setting en beats locale zh", "", "", "en", "", "", "zh_CN.UTF-8", "en"},
		{"setting auto follows the locale", "", "", "auto", "", "", "zh_CN.UTF-8", "zh"},
		{"locale zh", "", "", "", "", "", "zh_CN.UTF-8", "zh"},
		{"locale zh_TW", "", "", "", "", "", "zh_TW.UTF-8", "zh"},
		{"locale en", "", "", "", "", "", "en_US.UTF-8", "en"},
		{"LC_ALL before LANG", "", "", "", "en_US.UTF-8", "", "zh_CN.UTF-8", "en"},
		{"LC_ALL zh before LANG en", "", "", "", "zh_CN.UTF-8", "", "en_US.UTF-8", "zh"},
		{"LC_MESSAGES before LANG", "", "", "", "", "zh_CN.UTF-8", "en_US.UTF-8", "zh"},
		{"C is English", "", "", "", "", "", "C", "en"},
		{"flag auto is the locale", "auto", "zh", "zh", "", "", "en_US.UTF-8", "en"},
		{"flag is case-insensitive", "ZH", "", "", "", "", "en_US.UTF-8", "zh"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e.noLang()
			e.setLang("en") // while the setting is written
			if c.setting == "" {
				e.ok("", "config", "set", "lang")
			} else {
				e.ok("", "config", "set", "lang", c.setting)
			}
			e.noLang()
			e.setenv("SKILLDOTS_LANG", c.env)
			e.setenv("LC_ALL", c.lcAll)
			e.setenv("LC_MESSAGES", c.lcMessages)
			e.setenv("LANG", c.lang)
			var args []string
			if c.flag != "" {
				args = []string{"--lang", c.flag}
			}
			if got := langProbe(t, e, lib, args...); got != c.want {
				t.Errorf("speaks %s, want %s", got, c.want)
			}
		})
	}
}

// A value that is not a language is refused as a bad --theme is; a bad
// setting only warns, and the language falls back to the system's.
func TestBadLanguageValues(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.noLang()
	e.setenv("LANG", "en_US.UTF-8")
	if _, errOut, code := e.skd(lib, "--library", lib, "--lang", "klingon", "trash"); code == 0 || !strings.Contains(errOut, `--lang "klingon": use auto, en or zh`) {
		t.Errorf("--lang klingon: exit %d\n%s", code, errOut)
	}
	e.setenv("SKILLDOTS_LANG", "klingon")
	if _, errOut, code := e.skd(lib, "--library", lib, "trash"); code == 0 || !strings.Contains(errOut, `$SKILLDOTS_LANG "klingon": use auto, en or zh`) {
		t.Errorf("$SKILLDOTS_LANG=klingon: exit %d\n%s", code, errOut)
	}
	// In Chinese the refusal is in Chinese.
	e.setenv("LANG", "zh_CN.UTF-8")
	if _, errOut, code := e.skd(lib, "--library", lib, "--lang", "klingon", "trash"); code == 0 || !strings.Contains(errOut, "请使用 auto、en 或 zh") {
		t.Errorf("--lang klingon in zh: exit %d\n%s", code, errOut)
	}
	// config set refuses it too, and never stores it.
	e.noLang()
	e.setLang("en")
	if _, errOut, code := e.skd(lib, "config", "set", "lang", "klingon"); code == 0 || !strings.Contains(errOut, "use auto, en, zh") {
		t.Errorf("config set lang klingon: exit %d\n%s", code, errOut)
	}
	// A setting somebody wrote by hand.
	b, _ := os.ReadFile(e.configPath())
	os.WriteFile(e.configPath(), append(b, []byte("lang = \"klingon\"\n")...), 0o644)
	e.noLang()
	e.setenv("LANG", "zh_CN.UTF-8")
	out, errOut, code := e.skd(lib, "--library", lib, "trash")
	if code != 0 || strings.TrimSpace(out) != "回收站是空的" || !strings.Contains(errOut, `lang 设置 "klingon" 不是 auto、en 或 zh`) {
		t.Errorf("bad lang setting: exit %d\n%s\n%s", code, out, errOut)
	}
}

// jsonFixture is a library with a pack, an own skill and an agent
// directory with strangers in it, for commands whose output is data. The
// commits carry fixed dates, so the same fixture has the same hashes in
// every sandbox.
func jsonFixture(t *testing.T, e *env) (lib, up string) {
	t.Helper()
	e.setenv("GIT_AUTHOR_DATE", "2024-01-02T03:04:05")
	e.setenv("GIT_COMMITTER_DATE", "2024-01-02T03:04:05")
	lib = e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.write("up/skills/eng/tdd/SKILL.md", "---\nname: tdd\ndescription: 测试驱动开发 red, green, refactor\n---\n")
	e.write("up/skills/eng/review/SKILL.md", "---\nname: review\ndescription: Review carefully\n---\n")
	up = e.repo("up")
	e.ok(lib, "add", up, "--all")
	e.ok(lib, "new", "mine")
	e.write(".agents/skills/stranger/SKILL.md", "---\nname: stranger\ndescription: not ours\n---\n")
	return lib, up
}

var (
	timeRE  = regexp.MustCompile(`\d{4}-\d\d-\d\dT[\d:.]+(?:Z|[+-]\d\d:\d\d)`)
	batchRE = regexp.MustCompile(`\d{8}-\d{6}`)
)

// jsonRun runs `skd --library lib args… --json` in a fresh sandbox, in
// language lang through the environment (or through --lang when byFlag),
// and returns what it printed with what differs between sandboxes (the
// directory, times) made alike.
func jsonRun(t *testing.T, lang string, byFlag bool, build func(lib, up string) []string) (out, errOut string, code int) {
	t.Helper()
	e := newEnv(t)
	lib, up := jsonFixture(t, e)
	args := build(lib, up)
	pre := []string{"--library", lib}
	if byFlag {
		e.setLang("en")
		pre = append(pre, "--lang", lang)
	} else {
		e.setLang(lang)
	}
	out, errOut, code = e.skd(lib, append(append(pre, args...), "--json")...)
	norm := func(s string) string {
		s = strings.ReplaceAll(s, e.home, "$HOME")
		return batchRE.ReplaceAllString(timeRE.ReplaceAllString(s, "TIME"), "BATCH")
	}
	return norm(out), norm(errOut), code
}

// What --json prints is the same in every language, byte for byte, on
// stdout and on stderr (the messages that travel in it, the errors, the
// progress): agents parse it.
func TestJSONIsTheSameInEveryLanguage(t *testing.T) {
	commands := [][]string{
		{"list"}, {"list", "--all"}, {"list", "--flat"}, {"list", "nope"}, {"list", "--agent", "agents"},
		{"apply", "-n"}, {"apply"}, {"targets"}, {"targets", "trust", "nope"}, {"scan"}, {"adopt", "-n"}, {"adopt", "--yes"},
		{"config", "list"}, {"config", "get", "lang"}, {"config", "get", "nope"}, {"config", "set", "lang", "klingon"},
		{"config", "set", "lang", "zh"}, {"show", "@up"}, {"outdated"}, {"history"}, {"trash"},
		{"trash", "restore", "nope"}, {"path", "tdd"}, {"path", "nope"}, {"enable", "tdd", "--agent", "claude"},
		{"disable", "tdd", "--agent", "claude"}, {"add", "@up", "--skill", "nope"}, {"add", "bad source"}, {"update"},
		{"nope"}, {"edit", "nope"}, {"fork", "tdd", "--as", "mytdd"}, {"remove", "review"}, {"new", "bad name"},
		{"history", "-n", "x"}, {"use", "@nowhere"}, {"sync"}, {"purge", "@up", "--yes"},
	}
	for _, args := range commands {
		build := func(lib, up string) []string {
			out := make([]string, len(args))
			for i, a := range args {
				out[i] = strings.NewReplacer("@up", up, "@nowhere", filepath.Join(filepath.Dir(lib), "nowhere")).Replace(a)
			}
			return out
		}
		name := strings.Join(args, " ")
		out, errOut, code := jsonRun(t, "en", false, build)
		if strings.Contains(out+errOut, "\x1b") {
			t.Errorf("skd %s --json: an escape sequence:\n%s%s", name, out, errOut)
		}
		for _, byFlag := range []bool{false, true} {
			zOut, zErr, zCode := jsonRun(t, "zh", byFlag, build)
			if zOut != out || zErr != errOut || zCode != code {
				t.Errorf("skd %s --json differs between en and zh (by flag %v):\nen  exit %d\n%s\n%s\nzh  exit %d\n%s\n%s", name, byFlag, code, out, errOut, zCode, zOut, zErr)
			}
			if hasHan(zErr) && !strings.Contains(name, "klingon") {
				t.Errorf("skd %s --json: Chinese on stderr:\n%s", name, zErr)
			}
		}
	}
}

// Without --json the same commands speak Chinese, and say what they said.
func TestChineseOutput(t *testing.T) {
	e := newEnv(t)
	lib, up := jsonFixture(t, e)
	e.setLang("zh")
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"list"}, []string{"1 个来源", "3 个 skill", "已装上 3 个", "SKILL 数", "全部 2", "默认"}},
		{[]string{"list", "--all"}, []string{"分类", "描述", "skills/", "你自己的，随资料库一起发布", "测试驱动开发"}},
		{[]string{"list", "--agent", "agents"}, []string{"来源"}},
		{[]string{"list", "nope"}, []string{"错误：没有来源 \"nope\""}},
		{[]string{"apply", "-n"}, []string{"已是最新，共 3 个链接"}},
		{[]string{"outdated"}, []string{"已是最新", "一切都是最新的"}},
		{[]string{"show", up}, []string{"2 个 skill", "引入：skd add", "测试驱动开发"}},
		{[]string{"targets"}, []string{"TARGET", "路径", "默认"}},
		{[]string{"scan"}, []string{"扫描", "个条目", "已管理", "未管理", "stranger", "副本", "用 skd adopt 收编这些"}},
		{[]string{"adopt", "-n"}, []string{"复制到 skills/，作为本地 skill", "去掉 --dry-run 运行即可应用"}},
		{[]string{"history"}, []string{"还没有记录过更新"}},
		{[]string{"trash"}, []string{"回收站是空的"}},
		{[]string{"trash", "restore", "x"}, []string{"没有回收站条目"}},
		{[]string{"config", "list"}, []string{"设置项", "未设置", "用来打开 skill 目录的命令"}},
		{[]string{"config", "set", "lang", "zh"}, []string{"✓ lang = zh"}},
		{[]string{"config", "set", "nope", "x"}, []string{"未知的设置"}},
		{[]string{"path", "nope"}, []string{"没有 skill \"nope\""}},
		{[]string{"new", "other"}, []string{"已创建", "已应用 · 新链接 1 个"}},
		{[]string{"disable", "tdd", "--agent", "agents"}, []string{"已应用", "− agents"}},
		{[]string{"enable", "tdd"}, []string{"已应用", "+ agents"}},
		{[]string{"fork", "tdd", "--as", "mytdd"}, []string{"已 fork tdd"}},
		{[]string{"remove", "review"}, []string{"已应用", "− agents"}},
		{[]string{"sync"}, []string{"已在", "没有配置远程仓库"}},
		{[]string{"bogus"}, []string{"未知命令 \"bogus\""}},
		{[]string{"list", "--bogus"}, []string{"未知的选项：--bogus"}},
		{[]string{"path"}, []string{"需要 1 个参数，收到 0 个"}},
		{[]string{"update", "--yes"}, []string{"已是最新"}},
	} {
		out, errOut, _ := e.skd(lib, append([]string{"--library", lib}, c.args...)...)
		all := out + errOut
		for _, want := range c.want {
			if !strings.Contains(all, want) {
				t.Errorf("skd %v: no %q in\n%s", c.args, want, all)
			}
		}
	}
}

// The output of the commands has no English phrase left in Chinese:
// every sentence skd says is one of the messages the catalogue has.
func TestChineseOutputHasNoEnglishSentences(t *testing.T) {
	e := newEnv(t)
	lib, up := jsonFixture(t, e)
	e.setLang("zh")
	english := []string{
		"vendored", "up to date", "linked", "skipped", "nothing", "not found", "no pack", "no skill", "dry run", "applied",
		"warning:", "error:", "is empty", "could not", "cannot", "already", "fetching", "committed", "pushed", "created",
		"unknown", "use auto", "permanently", "managed", "unmanaged", "broken", "description", "default", "moved aside",
	}
	for _, args := range [][]string{
		{"list"}, {"list", "--all"}, {"apply", "-n"}, {"apply"}, {"outdated"}, {"show", up}, {"targets"}, {"scan"},
		{"adopt", "-n"}, {"history"}, {"trash"}, {"config", "list"}, {"config", "set", "lang", "zh"}, {"path", "nope"},
		{"list", "nope"}, {"new", "other"}, {"enable", "tdd", "--agent", "claude"}, {"disable", "tdd", "--agent", "claude"},
		{"fork", "tdd", "--as", "mytdd"}, {"sync"}, {"update", "--yes"}, {"add", up, "--skill", "review"}, {"bogus"},
		{"--help"}, {"add", "--help"}, {"trash", "restore", "--help"}, {"completion", "--help"}, {"completion", "bash", "--help"},
	} {
		out, errOut, _ := e.skd(lib, append([]string{"--library", lib}, args...)...)
		for _, l := range strings.Split(out+errOut, "\n") {
			low := strings.ToLower(l)
			for _, w := range english {
				// A line without a Chinese character but with an English
				// phrase is a message that was not translated.
				if strings.Contains(low, w) && !hasHan(l) {
					t.Errorf("skd %v: English %q left:\n%s", args, w, l)
				}
			}
		}
	}
}

// Table columns line up in Chinese, with Chinese in the data, and rows
// keep their width: a Chinese character is two cells.
func TestChineseTablesAlign(t *testing.T) {
	e := newEnv(t)
	lib, up := jsonFixture(t, e)
	e.write("up/skills/eng/长名字技能/SKILL.md", "---\nname: long-cjk\ndescription: 这是一个很长很长的中文描述，用来确认截断是按显示宽度而不是按字符数来算的，不然表格会错位\n---\n")
	e.git(up, "add", "-A")
	e.git(up, "commit", "-qm", "cjk")
	e.ok(lib, "update", "--yes")
	e.ok(lib, "add", up, "--skill", "long-cjk")
	for _, lang := range []string{"en", "zh"} {
		e.setLang(lang)
		for _, args := range [][]string{{"list", "--flat"}, {"show", up}} {
			out, _, code := e.skd(lib, append([]string{"--library", lib}, args...)...)
			if code != 0 {
				t.Fatalf("skd %v: exit %d", args, code)
			}
			var table []string
			for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
				if strings.HasPrefix(l, "install") || strings.HasPrefix(l, "引入") || strings.Contains(l, " @ ") {
					continue
				}
				table = append(table, l)
			}
			if len(table) < 3 {
				t.Fatalf("%s skd %v: no table:\n%s", lang, args, out)
			}
			w := ansi.StringWidth(table[0])
			for _, l := range table {
				if ansi.StringWidth(l) != w {
					t.Errorf("%s skd %v: rows are not the same width (%d, want %d): %q\n%s", lang, args, ansi.StringWidth(l), w, l, out)
				}
			}
			// The description was cut by cells: the long one is shortened
			// and the table is no wider than the longest description may be.
			if !strings.Contains(out, "…") {
				t.Errorf("%s skd %v: the long description is not truncated:\n%s", lang, args, out)
			}
		}
	}
}

// Every command, every flag: its help is in the language in use. Chinese
// has Chinese in the short and long text of every command and in the
// usage of every flag, English none.
func TestHelpTextIsInTheLanguage(t *testing.T) {
	check := func(lang i18n.Lang, wantHan bool) {
		i18n.Set(lang)
		defer i18n.Set(i18n.EN)
		var walk func(c *cobra.Command)
		walk = func(c *cobra.Command) {
			if got := hasHan(c.Short); got != wantHan {
				t.Errorf("%s: %q (short): Chinese=%v, want %v", c.CommandPath(), c.Short, got, wantHan)
			}
			if c.Long != "" && hasHan(c.Long) != wantHan {
				t.Errorf("%s: %q (long): Chinese=%v, want %v", c.CommandPath(), c.Long, hasHan(c.Long), wantHan)
			}
			c.LocalFlags().VisitAll(func(f *pflag.Flag) {
				if hasHan(f.Usage) != wantHan {
					t.Errorf("%s --%s: %q: Chinese=%v, want %v", c.CommandPath(), f.Name, f.Usage, hasHan(f.Usage), wantHan)
				}
			})
			for _, sub := range c.Commands() {
				walk(sub)
			}
		}
		root := cli.Root()
		root.InitDefaultHelpCmd()
		root.InitDefaultCompletionCmd()
		walk(root)
	}
	check(i18n.ZH, true)
	check(i18n.EN, false)
}

// The usage text cobra writes, and the key hints of the prompts, are in
// Chinese too.
func TestCobraChromeIsChinese(t *testing.T) {
	e := newEnv(t)
	e.setLang("zh")
	out, _, _ := e.skd("", "list", "--help")
	for _, want := range []string{"用法：", "选项：", "全局选项：", "list 的帮助"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
	out, _, _ = e.skd("", "--version")
	if !strings.Contains(out, "版本") {
		t.Errorf("--version: %q", out)
	}
	out, _, _ = e.skd("", "help", "trash")
	if !strings.Contains(out, "用法：") || strings.Contains(out, "Usage:") {
		t.Errorf("help trash:\n%s", out)
	}
	for _, lang := range []string{"en"} {
		e.setLang(lang)
		out, _, _ := e.skd("", "list", "--help")
		if !strings.Contains(out, "Usage:") || !strings.Contains(out, "help for list") || hasHan(out) {
			t.Errorf("%s: list --help:\n%s", lang, out)
		}
	}
}

// goldens are the help texts and a few outputs checked in full.
func TestChineseGoldens(t *testing.T) {
	e := newEnv(t)
	e.setLang("zh")
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	for name, args := range map[string][]string{
		"help":          {"--help"},
		"add-help":      {"add", "--help"},
		"update-help":   {"update", "--help"},
		"targets-help":  {"targets", "trust", "--help"},
		"completion":    {"completion", "--help"},
		"bash":          {"completion", "bash", "--help"},
		"trash-empty":   {"trash"},
		"history-empty": {"history"},
		"unknown":       {"nope"},
		"list-empty":    {"list"},
	} {
		out, errOut, _ := e.skd(lib, append([]string{"--library", lib}, args...)...)
		got := strings.NewReplacer(e.home, "$HOME").Replace(out + errOut)
		path := filepath.Join("testdata", "zh", name+".golden")
		if *update {
			os.MkdirAll(filepath.Dir(path), 0o755)
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (go test -update writes the golden files)", name, err)
		}
		if string(want) != got {
			t.Errorf("%s changed (go test -update rewrites it):\n--- want\n%s\n--- got\n%s", name, want, got)
		}
	}
}

// What skd writes to files is the same in every language: the library's
// files, the settings, the history, commit messages, the scaffold of a
// new skill, the note in a fork. Nothing in Chinese is stored.
func TestFilesAreWrittenInEnglish(t *testing.T) {
	e := newEnv(t)
	e.setLang("zh")
	e.setenv("GIT_AUTHOR_DATE", "2024-01-02T03:04:05")
	e.setenv("GIT_COMMITTER_DATE", "2024-01-02T03:04:05")
	lib := e.p("lib")
	e.ok("", "init", "--no-skill", lib)
	e.write("up/skills/eng/tdd/SKILL.md", "---\nname: tdd\ndescription: test first\n---\nbody\n")
	e.write("up/skills/eng/review/SKILL.md", "---\nname: review\ndescription: review carefully\n---\nbody\n")
	up := e.repo("up")
	e.ok(lib, "add", up, "--skill", "tdd", "--alias", "acme")
	e.ok(lib, "new", "mine")
	e.ok(lib, "new", "secret", "--private")
	e.ok(lib, "fork", "tdd", "--as", "mytdd")
	e.ok(lib, "config", "set", "theme", "dark")
	e.ok(lib, "config", "set", "lang", "zh")
	e.write("up/skills/eng/tdd/more.md", "more\n")
	e.git(up, "add", "-A")
	e.git(up, "commit", "-qm", "again")
	e.ok(lib, "add", up, "--skill", "review")
	e.ok(lib, "update", "--yes", "--commit")
	e.ok(lib, "remove", "review")
	e.ok(lib, "sync", "-m", "my backup")
	var found []string
	scan := func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		b, _ := os.ReadFile(path)
		if hasHan(string(b)) {
			found = append(found, path)
		}
		return nil
	}
	filepath.WalkDir(lib, scan)
	filepath.WalkDir(filepath.Dir(e.configPath()), scan)
	for _, f := range found {
		b, _ := os.ReadFile(f)
		t.Errorf("%s holds Chinese:\n%s", strings.TrimPrefix(f, e.home), b)
	}
	if log := e.git(lib, "log", "--format=%an %s%n%b"); hasHan(log) || !strings.Contains(log, "my backup") {
		t.Errorf("git log holds Chinese:\n%s", log)
	}
	// And the settings did take: the language was Chinese all along.
	if out := e.ok(lib, "--library", lib, "trash"); !strings.Contains(out, "原位置") {
		t.Errorf("the lang setting was not used:\n%s", out)
	}
}

// The real dashboard, in a terminal: the settings page switches the
// language of the running dashboard at once and saves the setting.
func TestDashboardSwitchesLanguageFromTheSettingsPage(t *testing.T) {
	e := newEnv(t)
	lib, _ := jsonFixture(t, e)
	e.noLang()
	e.setenv("LANG", "en_US.UTF-8")
	keys := []typed{
		{wait: 2 * time.Second},
		{wait: 700 * time.Millisecond, keys: ","},
		{wait: 700 * time.Millisecond, keys: "j"},
		{wait: 300 * time.Millisecond, keys: "j"},
		{wait: 300 * time.Millisecond, keys: "j"},
		{wait: 400 * time.Millisecond, keys: "\r"},
		{wait: 400 * time.Millisecond, keys: "zh"},
		{wait: 400 * time.Millisecond, keys: "\r"},
		{wait: 1500 * time.Millisecond, keys: "?"},
		{wait: 700 * time.Millisecond, keys: "x"},
		{wait: 400 * time.Millisecond, keys: "\x03"},
	}
	out := e.interactiveShell(lib, keys, "stty rows 40 cols 120; skd --library "+lib)
	for _, want := range []string{"a add", "lang = zh", "a 添加", "u 更新", "有未提交的改动", "按键"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in the dashboard's output:\n%q", want, out)
		}
	}
	b, _ := os.ReadFile(e.configPath())
	if !regexp.MustCompile(`(?m)^lang = ['"]zh['"]$`).Match(b) {
		t.Errorf("the setting was not saved:\n%s", b)
	}
}
