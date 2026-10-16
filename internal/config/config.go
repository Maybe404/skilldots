// Package config reads and writes the per-user settings file
// (config.toml in the user's config directory): where the library is,
// and the preferences `skd config` and the dashboard's settings page
// change. It is per user and per machine, never part of a library.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/Maybe404/skilldots/internal/fsutil"
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

// Keys are the settings, in display order.
var Keys = []Key{
	{Name: Editor, Help: "command that opens a skill directory (skd edit, o in the dashboard)",
		Default: "$VISUAL, $EDITOR, then VS Code, Cursor, Zed or Sublime Text if installed, else the system's default"},
	{Name: EditorKind, Help: "whether the editor command (setting, $VISUAL or $EDITOR) opens a window or runs in the terminal",
		Values:  []string{"auto", "window", "terminal"},
		Default: "auto: known window editors (code, cursor, zed, subl, …) open a window, any other command runs in the terminal"},
	{Name: Theme, Help: "colours: light, dark or auto (ask the terminal)", Values: []string{"auto", "light", "dark"},
		Default: "auto; --theme and $SKILLDOTS_THEME take precedence"},
	{Name: Lang, Help: "language of skd's own text (reserved: only English exists so far)", Values: []string{"auto", "en", "zh"},
		Default: "auto"},
	{Name: Library, Help: "the library skd uses when none is given", ReadOnly: "skd use <dir>",
		Default: "~/.skilldots"},
}

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
	return Key{}, fmt.Errorf("unknown setting %q (settings: %s)", name, strings.Join(names, ", "))
}

// Path is the settings file.
func Path() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "skilldots", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".skilldots", "config.toml")
}

// Load reads every setting. A missing file is no settings. A file that
// cannot be parsed is an error; Get treats it as empty.
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
		return nil, fmt.Errorf("%s: %w", Path(), err)
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
	return fmt.Errorf("%s %q: use %s", k.Name, value, strings.Join(k.Values, ", "))
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
		return fmt.Errorf("%s is set with `%s`", name, k.ReadOnly)
	}
	value = strings.TrimSpace(value)
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
			return fmt.Errorf("%s cannot be read (%v); fix or remove it first", p, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if value == "" {
		delete(raw, name)
	} else {
		raw[name] = value
	}
	// One document, so the keys of this file stay above any table (a
	// [section] a newer skd or a person added) and the tables keep theirs.
	body, err := toml.Marshal(raw)
	if err != nil {
		return err
	}
	if err := toml.Unmarshal(body, &map[string]any{}); err != nil {
		return fmt.Errorf("%s: would not read back (%v); left as it was", p, err)
	}
	var b strings.Builder
	b.WriteString("# skilldots settings: skd config list | get | set\n")
	b.Write(body)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return fsutil.WriteAtomic(p, []byte(b.String()))
}
