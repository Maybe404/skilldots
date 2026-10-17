// Package config reads and writes the per-user settings file
// (config.toml in the user's config directory): where the library is,
// and the preferences `skd config` and the dashboard's settings page
// change. It is per user and per machine, never part of a library.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/safetext"
)

// Setting names.
const (
	Library    = "library"
	Editor     = "editor"
	EditorKind = "editor-kind"
	Theme      = "theme"
	Lang       = "lang"
)

// Key describes one setting.
type Key struct {
	Name string
	// Help is a one-line description for `skd config list` and the
	// dashboard's settings page.
	Help string
	// Values are the accepted values; empty accepts anything. An empty
	// value always unsets the key.
	Values []string
	// ReadOnly keys are shown but changed by another command.
	ReadOnly string
	// Default describes what applies when the key is unset.
	Default string
}

// Keys are the settings, in display order. Help and Default are English
// messages (i18n.M): show them with HelpText and DefaultText.
var Keys = []Key{
	{Name: Editor, Help: i18n.M("command that opens a skill directory (skd edit, o in the dashboard)"),
		Default: i18n.M("$VISUAL, $EDITOR, then VS Code, Cursor, Zed or Sublime Text if installed, else the system's default")},
	{Name: EditorKind, Help: i18n.M("whether the editor command (setting, $VISUAL or $EDITOR) opens a window or runs in the terminal"),
		Values:  []string{"auto", "window", "terminal"},
		Default: i18n.M("auto: known window editors (code, cursor, zed, subl, …) open a window, any other command runs in the terminal")},
	{Name: Theme, Help: i18n.M("colours: light, dark or auto (ask the terminal)"), Values: []string{"auto", "light", "dark"},
		Default: i18n.M("auto; --theme and $SKILLDOTS_THEME take precedence")},
	{Name: Lang, Help: i18n.M("language of skd's own text: en, zh (简体中文) or auto (the system's)"), Values: []string{"auto", "en", "zh"},
		Default: i18n.M("auto; --lang and $SKILLDOTS_LANG take precedence")},
	{Name: Library, Help: i18n.M("the library skd uses when none is given"), ReadOnly: "skd use <dir>",
		Default: "~/.skilldots"},
}

// HelpText is Help in the language in use.
func (k Key) HelpText() string { return i18n.T(k.Help) }

// DefaultText is Default in the language in use.
func (k Key) DefaultText() string { return i18n.T(k.Default) }

// Lookup returns the key named name.
func Lookup(name string) (Key, error) {
	for _, k := range Keys {
		if k.Name == name {
			return k, nil
		}
	}
	var names []string
	for _, k := range Keys {
		names = append(names, k.Name)
	}
	return Key{}, i18n.Errorf("unknown setting %q (settings: %s)", name, strings.Join(names, ", "))
}

// Path is the settings file.
func Path() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "skilldots", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".skilldots", "config.toml")
}

// InvalidError is a settings file that is not valid TOML.
type InvalidError struct {
	Path string
	Err  error
}

func (e *InvalidError) Error() string {
	return i18n.Sprintf("the settings file %s is not valid TOML: %v", e.Path, e.Err)
}

func (e *InvalidError) Unwrap() error { return e.Err }

// Load reads every setting. A missing file is no settings. A file that
// cannot be parsed is an *InvalidError; Get treats it as empty.
func Load() (map[string]string, error) {
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	raw := map[string]any{}
	if err := toml.Unmarshal(b, &raw); err != nil {
		return nil, &InvalidError{Path(), err}
	}
	out := map[string]string{}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out, nil
}

// Get returns one setting, "" when unset or unreadable.
func Get(name string) string {
	all, err := Load()
	if err != nil {
		return ""
	}
	return all[name]
}

// Check validates value for key k.
func (k Key) Check(value string) error {
	if value == "" || len(k.Values) == 0 {
		return nil
	}
	for _, v := range k.Values {
		if v == value {
			return nil
		}
	}
	return i18n.Errorf("%s %q: use %s", k.Name, value, strings.Join(k.Values, ", "))
}

// Set changes one setting (an empty value unsets it) and keeps the others,
// including keys this version does not know. Read-only keys are refused;
// SetLibrary is how `skd use`, `init` and `clone` record the library.
func Set(name, value string) error {
	k, err := Lookup(name)
	if err != nil {
		return err
	}
	if k.ReadOnly != "" {
		return i18n.Errorf("%s is set with `%s`", name, k.ReadOnly)
	}
	value = strings.TrimSpace(value)
	if safetext.Has(value) {
		return i18n.Errorf("%s %q has control characters", name, value)
	}
	if err := k.Check(value); err != nil {
		return err
	}
	return write(name, value)
}

// SetLibrary records the library skd uses when none is given.
func SetLibrary(root string) error { return write(Library, filepath.ToSlash(root)) }

func write(name, value string) error {
	p := Path()
	raw := map[string]any{}
	if b, err := os.ReadFile(p); err == nil {
		if err := toml.Unmarshal(b, &raw); err != nil {
			return i18n.Errorf("%w; fix or remove it first", &InvalidError{p, err})
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if value == "" {
		delete(raw, name)
	} else {
		raw[name] = value
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// Change the key's own line, keeping the rest of the file (comments
	// included) as it is. When the line cannot be found, or the result
	// would not read back as exactly the settings wanted, write the whole
	// file again instead.
	if old, err := os.ReadFile(p); err == nil {
		if b, ok := editLine(old, name, value); ok {
			got := map[string]any{}
			if toml.Unmarshal(b, &got) == nil && reflect.DeepEqual(got, raw) {
				return fsutil.WriteAtomic(p, b)
			}
		}
	}
	// One document, so the keys of this file stay above any table (a
	// [section] a newer skd or a person added) and the tables keep theirs.
	body, err := toml.Marshal(raw)
	if err != nil {
		return err
	}
	if err := toml.Unmarshal(body, &map[string]any{}); err != nil {
		return i18n.Errorf("%s: would not read back (%v); left as it was", p, err)
	}
	var b strings.Builder
	b.WriteString("# skilldots settings: skd config list | get | set\n")
	b.Write(body)
	out := b.String()
	// A file with Windows line endings keeps them.
	if old, err := os.ReadFile(p); err == nil && lineEnd(old) == "\r\n" {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return fsutil.WriteAtomic(p, []byte(out))
}

// lineEnd is the line ending a file uses: "\r\n" when its first line
// ends so, else "\n".
func lineEnd(src []byte) string {
	if i := strings.IndexByte(string(src), '\n'); i > 0 && src[i-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// comment is the comment that ends a key's line (with the space before
// it), "" when there is none: the text from the first "#" up to which the
// line reads as that key alone. A "#" inside the value does not count.
func comment(line, name string) string {
	body := strings.TrimRight(line, "\r\n")
	for i := strings.IndexByte(body, '#'); i >= 0; {
		m := map[string]any{}
		if toml.Unmarshal([]byte(body[:i]), &m) == nil && len(m) == 1 && m[name] != nil {
			pre := strings.TrimRight(body[:i], " \t")
			return body[len(pre):]
		}
		j := strings.IndexByte(body[i+1:], '#')
		if j < 0 {
			break
		}
		i += j + 1
	}
	return ""
}

// editLine sets name to value (removes it when value is "") in the
// top-level part of a settings file, before any [table], touching only
// that key's line: comments and the other lines stay as they are, so does
// a comment at the end of that line (unless the key is removed: the line
// goes with it), and a new or changed line ends like the file's lines.
// ok is false when the file holds the key in a form this does not handle
// (a value over several lines, say); write checks the result anyway.
func editLine(src []byte, name, value string) (out []byte, ok bool) {
	eol := lineEnd(src)
	lines := strings.SplitAfter(string(src), "\n")
	top := len(lines) // index of the first table header
	at := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			top = i
			break
		}
		k, _, isKey := strings.Cut(t, "=")
		if !isKey {
			continue
		}
		if k = strings.Trim(strings.TrimSpace(k), `"'`); k == name {
			if at >= 0 {
				return nil, false
			}
			at = i
		}
	}
	var line string
	if value != "" {
		b, err := toml.Marshal(map[string]string{name: value})
		if err != nil {
			return nil, false
		}
		line = strings.TrimSuffix(string(b), "\n") + eol
	}
	switch {
	case at >= 0 && line != "":
		old := lines[at]
		end := old[len(strings.TrimRight(old, "\r\n")):] // as it ended, or not at all
		lines[at] = strings.TrimSuffix(line, eol) + comment(old, name) + end
	case at >= 0:
		lines[at] = line
	case value == "":
		return src, true
	default:
		// A new key goes after the last top-level line that is not blank
		// (with a table below, not a comment either: that introduces the
		// table).
		ins := top
		for ins > 0 {
			t := strings.TrimSpace(lines[ins-1])
			if t != "" && (top == len(lines) || !strings.HasPrefix(t, "#")) {
				break
			}
			ins--
		}
		if ins > 0 && !strings.HasSuffix(lines[ins-1], "\n") {
			lines[ins-1] += eol
		}
		lines = append(lines[:ins], append([]string{line}, lines[ins:]...)...)
	}
	return []byte(strings.Join(lines, "")), true
}
