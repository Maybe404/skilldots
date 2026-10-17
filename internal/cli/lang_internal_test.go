package cli

import (
	"reflect"
	"testing"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"

	"github.com/Maybe404/skilldots/internal/i18n"
)

func TestScanArgs(t *testing.T) {
	for _, c := range []struct {
		args []string
		lang string
		json bool
	}{
		{nil, "", false},
		{[]string{"list"}, "", false},
		{[]string{"--lang", "zh", "list"}, "zh", false},
		{[]string{"list", "--lang=zh"}, "zh", false},
		{[]string{"list", "--json"}, "", true},
		{[]string{"--json=true", "list"}, "", true},
		{[]string{"--json=false", "list"}, "", false},
		{[]string{"--lang", "zh", "--json"}, "zh", true},
		{[]string{"list", "--", "--json", "--lang", "zh"}, "", false},
		{[]string{"--lang"}, "", false},
		{[]string{"--lang", "en", "--lang", "zh"}, "zh", false},
	} {
		lang, json := scanArgs(c.args)
		if lang != c.lang || json != c.json {
			t.Errorf("scanArgs(%q) = %q, %v; want %q, %v", c.args, lang, json, c.lang, c.json)
		}
	}
}

// --json is English, whatever else was asked: what is in the document
// must not depend on the language.
func TestApplyLangJSONIsEnglish(t *testing.T) {
	defer i18n.Set(i18n.EN)
	t.Setenv(i18n.Env, "zh")
	if bad := applyLang("zh", true); len(bad) != 0 || i18n.Get() != i18n.EN {
		t.Errorf("--json: %v %v", bad, i18n.Get())
	}
	if bad := applyLang("", false); len(bad) != 0 || i18n.Get() != i18n.ZH {
		t.Errorf("env zh: %v %v", bad, i18n.Get())
	}
	if bad := applyLang("en", false); len(bad) != 0 || i18n.Get() != i18n.EN {
		t.Errorf("--lang en over env zh: %v %v", bad, i18n.Get())
	}
	if bad := applyLang("klingon", false); len(bad) != 1 || bad[0] != i18n.FromFlag || i18n.Get() != i18n.ZH {
		t.Errorf("a bad flag falls to the next way: %v %v", bad, i18n.Get())
	}
}

func TestLabelAndPad(t *testing.T) {
	if got := label("error:", "x"); got != "error: x" {
		t.Errorf("%q", got)
	}
	if got := label("错误：", "x"); got != "错误：x" {
		t.Errorf("%q", got)
	}
	defer i18n.Set(i18n.EN)
	// English pads like %-13s always did; Chinese by cells.
	if got := pad("changed", 13); got != "changed      " {
		t.Errorf("%q", got)
	}
	i18n.Set(i18n.ZH)
	if got, want := pad("已变更", 13), "已变更       "; got != want || i18n.Width(got) != 13 {
		t.Errorf("%q (%d cells)", got, i18n.Width(got))
	}
}

// truncate counts cells: a Chinese character is two.
func TestTruncateCountsCells(t *testing.T) {
	if got := truncate("abcdefgh", 5); got != "abcd…" {
		t.Errorf("%q", got)
	}
	got := truncate("一二三四五六", 7)
	if i18n.Width(got) > 7 || got != "一二三…" {
		t.Errorf("%q (%d cells)", got, i18n.Width(got))
	}
	if got := truncate("一二三", 6); got != "一二三" {
		t.Errorf("fits, untouched: %q", got)
	}
}

// The key hints of huh's prompts are in Chinese: every one of them, so a
// hint added by a newer huh shows up here.
func TestPromptKeyHintsAreChinese(t *testing.T) {
	defer i18n.Set(i18n.EN)
	i18n.Set(i18n.ZH)
	var walk func(v reflect.Value, path string)
	walk = func(v reflect.Value, path string) {
		if v.Kind() != reflect.Struct {
			return
		}
		if v.Type() == reflect.TypeOf(key.Binding{}) {
			b := v.Interface().(key.Binding)
			desc := b.Help().Desc
			han := false
			for _, r := range desc {
				han = han || unicode.Is(unicode.Han, r)
			}
			if desc != "" && !han {
				t.Errorf("%s: %q is not translated", path, desc)
			}
			return
		}
		for i := 0; i < v.NumField(); i++ {
			walk(v.Field(i), path+"."+v.Type().Field(i).Name)
		}
	}
	walk(reflect.ValueOf(keyMap()).Elem(), "KeyMap")
	// English is huh's own.
	i18n.Set(i18n.EN)
	if got, want := keyMap().Input.Submit.Help().Desc, huh.NewDefaultKeyMap().Input.Submit.Help().Desc; got != want {
		t.Errorf("English hint changed: %q", got)
	}
}

// The commands that take an argument say so the way cobra does.
func TestArgumentErrors(t *testing.T) {
	defer i18n.Set(i18n.EN)
	for _, c := range []struct {
		lang i18n.Lang
		got  error
		want string
	}{
		{i18n.EN, exactArgs(1)(nil, nil), "accepts 1 arg(s), received 0"},
		{i18n.EN, rangeArgs(1, 2)(nil, nil), "accepts between 1 and 2 arg(s), received 0"},
		{i18n.EN, maximumNArgs(1)(nil, []string{"a", "b"}), "accepts at most 1 arg(s), received 2"},
		{i18n.ZH, exactArgs(1)(nil, []string{"a", "b"}), "需要 1 个参数，收到 2 个"},
		{i18n.ZH, rangeArgs(1, 2)(nil, nil), "需要 1 到 2 个参数，收到 0 个"},
		{i18n.ZH, maximumNArgs(1)(nil, []string{"a", "b"}), "最多接受 1 个参数，收到 2 个"},
	} {
		i18n.Set(c.lang)
		if c.got == nil || c.got.Error() != c.want {
			t.Errorf("%v: %v, want %q", c.lang.Code(), c.got, c.want)
		}
	}
}
