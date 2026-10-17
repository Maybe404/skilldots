package i18n

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

// The Chinese catalogue is plain text, one file per area (zh/*.txt):
//
//	# a comment
//	@ the English message, exactly as the source has it
//	= 它的中文翻译
//
// In both lines a backslash starts an escape: \n is a line break, \t a
// tab, \\ a backslash. A pair is separated from the next by a blank line.
// The tests hold every message of the source to an entry here and every
// entry to a message.
//
//go:embed zh/*.txt
var zhFiles embed.FS

func init() {
	files, err := zhFiles.ReadDir("zh")
	if err != nil {
		panic(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name())
	}
	sort.Strings(names)
	for _, n := range names {
		b, err := zhFiles.ReadFile("zh/" + n)
		if err != nil {
			panic(err)
		}
		m, err := parseCatalog(n, string(b))
		if err != nil {
			panic(err)
		}
		register(m)
	}
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Escape is the form of s in a catalogue file.
func Escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return strings.ReplaceAll(s, "\t", `\t`)
}

func parseCatalog(name, src string) (map[string]string, error) {
	out := map[string]string{}
	var key string
	haveKey := false
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
			if haveKey {
				return nil, fmt.Errorf("i18n: %s:%d: %q has no translation", name, i+1, key)
			}
		case strings.HasPrefix(line, "@ "):
			if haveKey {
				return nil, fmt.Errorf("i18n: %s:%d: %q has no translation", name, i+1, key)
			}
			key, haveKey = unescape(line[2:]), true
		case strings.HasPrefix(line, "= "):
			if !haveKey {
				return nil, fmt.Errorf("i18n: %s:%d: translation without a message", name, i+1)
			}
			if _, dup := out[key]; dup {
				return nil, fmt.Errorf("i18n: %s:%d: %q is translated twice", name, i+1, key)
			}
			out[key], haveKey = unescape(line[2:]), false
		default:
			return nil, fmt.Errorf("i18n: %s:%d: expected '@ ', '= ' or '#'", name, i+1)
		}
	}
	if haveKey {
		return nil, fmt.Errorf("i18n: %s: %q has no translation", name, key)
	}
	return out, nil
}
