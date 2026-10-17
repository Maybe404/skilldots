package i18n

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Env is the environment variable that selects the language.
const Env = "SKILLDOTS_LANG"

// Parse reads a lang value ("" is auto): "auto", "en" or "zh", in any
// case. ok is false for anything else.
func Parse(v string) (code string, ok bool) {
	switch v = strings.ToLower(strings.TrimSpace(v)); v {
	case "", "auto":
		return "auto", true
	case "en", "zh":
		return v, true
	}
	return "", false
}

// Source says which of the ways of choosing a language decided.
type Source string

const (
	FromFlag    Source = "--lang"
	FromEnv     Source = "$" + Env
	FromSetting Source = "the lang setting (skd config set lang)"
	FromSystem  Source = "the system language"
)

// Inputs are what Resolve chooses from, in order of precedence.
type Inputs struct {
	Flag    string // --lang
	Setting string // skd config set lang
}

// Resolve chooses the language: --lang, then $SKILLDOTS_LANG, then the
// lang setting, then (for "auto" or nothing) the system's. A value that
// is none of auto, en, zh is skipped and named in bad (its source), so
// the caller can say so; the choice falls to the next way.
func Resolve(in Inputs) (l Lang, src Source, bad []Source) {
	for _, c := range []struct {
		v   string
		src Source
	}{{in.Flag, FromFlag}, {os.Getenv(Env), FromEnv}, {in.Setting, FromSetting}} {
		if strings.TrimSpace(c.v) == "" {
			continue
		}
		code, ok := Parse(c.v)
		if !ok {
			bad = append(bad, c.src)
			continue
		}
		switch code {
		case "en":
			return EN, c.src, bad
		case "zh":
			return ZH, c.src, bad
		}
		// auto: stop here, the system decides (a lower way does not
		// override an explicit auto).
		return System(), FromSystem, bad
	}
	return System(), FromSystem, bad
}

// System is the language of the system: the first of $LC_ALL,
// $LC_MESSAGES and $LANG that is set decides (zh_CN.UTF-8 is Chinese,
// anything else, "C" included, English); with none set, macOS is asked
// for its preferred languages. When nothing can be told it is English.
func System() Lang {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return fromLocale(v)
		}
	}
	if systemLanguages != nil {
		for _, tag := range systemLanguages() {
			return fromLocale(tag)
		}
	}
	return EN
}

// fromLocale reads a POSIX locale name (zh_CN.UTF-8) or a BCP 47 tag
// (zh-Hans-CN) as a language.
func fromLocale(v string) Lang {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "zh" || strings.HasPrefix(v, "zh_") || strings.HasPrefix(v, "zh-") || strings.HasPrefix(v, "zh.") || strings.HasPrefix(v, "zh@") {
		return ZH
	}
	return EN
}

// systemLanguages lists the user's preferred languages, best first; nil
// where the system has no such list. A variable so tests can replace it.
var systemLanguages = func() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return nil
	}
	return parseAppleLanguages(string(out))
}

// parseAppleLanguages reads the property-list array `defaults` prints:
//
//	(
//	    "zh-Hans-CN",
//	    "en-CN"
//	)
func parseAppleLanguages(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.Trim(strings.TrimSpace(line), `(),`)
		line = strings.Trim(line, `"`)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
