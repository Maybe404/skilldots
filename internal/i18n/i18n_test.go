package i18n

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func withLang(t *testing.T, l Lang) {
	t.Helper()
	prev := Get()
	Set(l)
	t.Cleanup(func() { Set(prev) })
}

func withCatalog(t *testing.T, m map[string]string) {
	t.Helper()
	old := zh
	zh = m
	t.Cleanup(func() { zh = old })
}

func TestTranslate(t *testing.T) {
	withCatalog(t, map[string]string{"hello %s": "你好 %s", "gone": "没了"})
	withLang(t, EN)
	if got := Sprintf("hello %s", "x"); got != "hello x" {
		t.Errorf("en: %q", got)
	}
	Set(ZH)
	if got := Sprintf("hello %s", "x"); got != "你好 x" {
		t.Errorf("zh: %q", got)
	}
	if got := T("missing"); got != "missing" {
		t.Errorf("a message with no translation reads in English: %q", got)
	}
	if got := M("gone"); got != "gone" {
		t.Errorf("M marks, it does not translate: %q", got)
	}
	var b strings.Builder
	Fprintf(&b, "hello %s", "y")
	if b.String() != "你好 y" {
		t.Errorf("Fprintf: %q", b.String())
	}
	_ = io.Discard
}

func TestCount(t *testing.T) {
	withCatalog(t, map[string]string{"%d packs": "%d 个 pack"})
	withLang(t, EN)
	for n, want := range map[int]string{0: "0 packs", 1: "1 pack", 2: "2 packs"} {
		if got := Count(n, "%d pack", "%d packs"); got != want {
			t.Errorf("en %d: %q", n, got)
		}
	}
	Set(ZH)
	for _, n := range []int{0, 1, 2} {
		if got := Count(n, "%d pack", "%d packs"); !strings.HasSuffix(got, " 个 pack") {
			t.Errorf("zh %d: %q", n, got)
		}
	}
}

func TestErrors(t *testing.T) {
	withCatalog(t, map[string]string{"cannot read %s: %w": "无法读取 %s：%w", "no tty": "没有终端", "inner %s": "内层 %s"})
	withLang(t, EN)
	sentinel := New("no tty")
	base := os.ErrNotExist
	err := Errorf("cannot read %s: %w", "a", base)
	if err.Error() != "cannot read a: file does not exist" {
		t.Errorf("en: %q", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Error("an Errorf with %w must wrap")
	}
	if errors.Unwrap(err) != base {
		t.Error("Unwrap")
	}
	wrapped := Errorf("cannot read %s: %w", "b", Errorf("inner %s", "c"))
	Set(ZH)
	if got := err.Error(); got != "无法读取 a：file does not exist" {
		t.Errorf("zh: %q", got)
	}
	if got := wrapped.Error(); got != "无法读取 b：内层 c" {
		t.Errorf("zh nested: %q", got)
	}
	if got := sentinel.Error(); got != "没有终端" {
		t.Errorf("sentinel: %q", got)
	}
	// What skd writes to files stays English, whatever language is in use.
	if got := English(wrapped); got != "cannot read b: inner c" {
		t.Errorf("English: %q", got)
	}
	if got := English(base); got != base.Error() {
		t.Errorf("English of a foreign error: %q", got)
	}
	if English(nil) != "" {
		t.Error("English(nil)")
	}
	// A sentinel is the same error whatever the language.
	if sentinel != sentinel || !errors.Is(Errorf("x: %w", sentinel), sentinel) {
		t.Error("sentinel identity")
	}
}

func TestNewWithPercent(t *testing.T) {
	if got := New("100% sure").Error(); got != "100% sure" {
		t.Errorf("%q", got)
	}
}

func TestParse(t *testing.T) {
	for in, want := range map[string]string{"": "auto", "auto": "auto", " ZH ": "zh", "en": "en", "EN": "en"} {
		if got, ok := Parse(in); !ok || got != want {
			t.Errorf("Parse(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"fr", "zh-CN", "chinese", "1"} {
		if _, ok := Parse(in); ok {
			t.Errorf("Parse(%q) accepted", in)
		}
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG", Env} {
		t.Setenv(k, "")
	}
}

func TestSystem(t *testing.T) {
	prev := systemLanguages
	t.Cleanup(func() { systemLanguages = prev })
	systemLanguages = func() []string { return nil }
	clearEnv(t)
	if System() != EN {
		t.Error("nothing known must be English")
	}
	for _, c := range []struct {
		env  map[string]string
		os   []string
		want Lang
		why  string
	}{
		{map[string]string{"LANG": "zh_CN.UTF-8"}, nil, ZH, "LANG"},
		{map[string]string{"LANG": "zh_TW.UTF-8"}, nil, ZH, "zh_TW"},
		{map[string]string{"LANG": "en_US.UTF-8"}, nil, EN, "LANG en"},
		{map[string]string{"LANG": "C"}, []string{"zh-Hans-CN"}, EN, "C is decisive: the system list is not asked"},
		{map[string]string{"LC_ALL": "en_US.UTF-8", "LANG": "zh_CN.UTF-8"}, nil, EN, "LC_ALL first"},
		{map[string]string{"LC_ALL": "zh_CN.UTF-8", "LANG": "en_US.UTF-8"}, nil, ZH, "LC_ALL first"},
		{map[string]string{"LC_MESSAGES": "zh_CN.UTF-8", "LANG": "en_US.UTF-8"}, nil, ZH, "LC_MESSAGES before LANG"},
		{map[string]string{"LC_ALL": "en_US.UTF-8", "LC_MESSAGES": "zh_CN.UTF-8"}, nil, EN, "LC_ALL before LC_MESSAGES"},
		{nil, []string{"zh-Hans-CN", "en-CN"}, ZH, "system list"},
		{nil, []string{"en-CN", "zh-Hans-CN"}, EN, "system list, first wins"},
		{nil, []string{"zh-Hant-TW"}, ZH, "traditional"},
		{nil, nil, EN, "nothing"},
	} {
		clearEnv(t)
		for k, v := range c.env {
			t.Setenv(k, v)
		}
		systemLanguages = func() []string { return c.os }
		if got := System(); got != c.want {
			t.Errorf("%s: got %v", c.why, got.Code())
		}
	}
}

func TestParseAppleLanguages(t *testing.T) {
	got := parseAppleLanguages("(\n    \"zh-Hans-CN\",\n    \"en-CN\"\n)\n")
	if len(got) != 2 || got[0] != "zh-Hans-CN" || got[1] != "en-CN" {
		t.Errorf("%q", got)
	}
	if got := parseAppleLanguages(""); len(got) != 0 {
		t.Errorf("%q", got)
	}
	if got := parseAppleLanguages("(\n    en\n)"); len(got) != 1 || got[0] != "en" {
		t.Errorf("unquoted: %q", got)
	}
}

// --lang beats $SKILLDOTS_LANG beats the setting beats the system.
func TestResolveOrder(t *testing.T) {
	prev := systemLanguages
	t.Cleanup(func() { systemLanguages = prev })
	systemLanguages = func() []string { return nil }
	for _, c := range []struct {
		flag, env, setting, locale string
		want                       Lang
		src                        Source
	}{
		{"zh", "en", "en", "en_US.UTF-8", ZH, FromFlag},
		{"en", "zh", "zh", "zh_CN.UTF-8", EN, FromFlag},
		{"", "zh", "en", "en_US.UTF-8", ZH, FromEnv},
		{"", "en", "zh", "zh_CN.UTF-8", EN, FromEnv},
		{"", "", "zh", "en_US.UTF-8", ZH, FromSetting},
		{"", "", "en", "zh_CN.UTF-8", EN, FromSetting},
		{"", "", "auto", "zh_CN.UTF-8", ZH, FromSystem},
		{"", "", "", "zh_CN.UTF-8", ZH, FromSystem},
		{"", "", "", "en_US.UTF-8", EN, FromSystem},
		{"", "", "", "", EN, FromSystem},
		{"auto", "", "zh", "en_US.UTF-8", EN, FromSystem}, // an explicit auto is a choice
		{"", "auto", "zh", "en_US.UTF-8", EN, FromSystem},
		{" ZH ", "", "", "en_US.UTF-8", ZH, FromFlag},
	} {
		clearEnv(t)
		t.Setenv("LANG", c.locale)
		t.Setenv(Env, c.env)
		got, src, bad := Resolve(Inputs{Flag: c.flag, Setting: c.setting})
		if got != c.want || src != c.src || len(bad) != 0 {
			t.Errorf("%+v: got %s from %s (bad %v)", c, got.Code(), src, bad)
		}
	}
}

// A value that is not a language is skipped, named, and the next way decides.
func TestResolveSkipsBadValues(t *testing.T) {
	prev := systemLanguages
	t.Cleanup(func() { systemLanguages = prev })
	systemLanguages = func() []string { return nil }
	clearEnv(t)
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv(Env, "klingon")
	l, src, bad := Resolve(Inputs{Flag: "fr", Setting: "zh"})
	if l != ZH || src != FromSetting || len(bad) != 2 || bad[0] != FromFlag || bad[1] != FromEnv {
		t.Errorf("got %s from %s, bad %v", l.Code(), src, bad)
	}
}

func TestWidth(t *testing.T) {
	if Width("中文a") != 5 {
		t.Error("Width")
	}
	if got := PadRight("中", 4); got != "中  " {
		t.Errorf("PadRight %q", got)
	}
	if got := PadRight("long", 2); got != "long" {
		t.Errorf("PadRight shorter %q", got)
	}
	if got := Truncate("中文字符", 5); Width(got) > 5 || !strings.HasSuffix(got, "…") {
		t.Errorf("Truncate %q", got)
	}
	if got := Truncate("abc", 5); got != "abc" {
		t.Errorf("Truncate short %q", got)
	}
	if Truncate("abc", 0) != "" {
		t.Error("Truncate 0")
	}
}

func TestEnglishOfJoin(t *testing.T) {
	withCatalog(t, map[string]string{"first": "第一", "second": "第二"})
	withLang(t, ZH)
	err := errors.Join(New("first"), New("second"), os.ErrNotExist)
	if got := English(err); got != "first\nsecond\nfile does not exist" {
		t.Errorf("%q", got)
	}
	if got := err.Error(); got != "第一\n第二\nfile does not exist" {
		t.Errorf("%q", got)
	}
}
