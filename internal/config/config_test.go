package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func sandbox(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

func TestSetKeepsOtherKeys(t *testing.T) {
	sandbox(t)
	// A file written by an older skd: a comment and the library.
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(Path(), []byte("# Written by skd init/clone.\nlibrary = \"/x/lib\"\nfuture = \"kept\"\n"), 0o644)
	if err := Set(Editor, "code --wait"); err != nil {
		t.Fatal(err)
	}
	if err := Set(Theme, "light"); err != nil {
		t.Fatal(err)
	}
	all, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"library": "/x/lib", "future": "kept", "editor": "code --wait", "theme": "light"}
	for k, v := range want {
		if all[k] != v {
			t.Errorf("%s = %q, want %q (file:\n%s)", k, all[k], v, read(t))
		}
	}
	if err := Set(Theme, ""); err != nil || Get(Theme) != "" {
		t.Fatalf("unset theme: %v %q", err, Get(Theme))
	}
	if err := SetLibrary("/y/lib"); err != nil || Get(Library) != "/y/lib" || Get(Editor) != "code --wait" {
		t.Fatalf("SetLibrary: %v\n%s", err, read(t))
	}
}

func TestSetRefuses(t *testing.T) {
	sandbox(t)
	for _, c := range [][2]string{{"theme", "blue"}, {"lang", "fr"}, {"nope", "x"}, {"library", "/z"}} {
		if err := Set(c[0], c[1]); err == nil {
			t.Errorf("set %s %s: no error", c[0], c[1])
		}
	}
	if _, err := os.Stat(Path()); err == nil {
		t.Fatalf("a refused set wrote the file:\n%s", read(t))
	}
	// A corrupt file is not overwritten.
	os.MkdirAll(filepath.Dir(Path()), 0o755)
	os.WriteFile(Path(), []byte("library = [unterminated\n"), 0o644)
	if err := Set(Editor, "vim"); err == nil || !strings.Contains(err.Error(), "fix or remove") {
		t.Fatalf("set on a corrupt file: %v", err)
	}
	if Get(Library) != "" {
		t.Fatal("corrupt file read as a value")
	}
}

func read(t *testing.T) string {
	b, _ := os.ReadFile(Path())
	return string(b)
}

// A file with tables (a newer skd's, or a person's): settings stay
// top-level keys, the tables keep theirs, and the file reads back after
// every change.
func TestSetKeepsTables(t *testing.T) {
	sandbox(t)
	os.MkdirAll(filepath.Dir(Path()), 0o755)
	os.WriteFile(Path(), []byte("library = \"/x/lib\"\nzeta = \"top\"\n\n[extra]\nkey = 1\nname = \"in table\"\n\n[extra.deep]\nlist = [1, 2]\n"), 0o644)
	for _, c := range [][2]string{{Theme, "light"}, {Editor, "vim"}, {Theme, "dark"}, {Lang, "en"}} {
		if err := Set(c[0], c[1]); err != nil {
			t.Fatalf("set %s: %v\n%s", c[0], err, read(t))
		}
	}
	var raw map[string]any
	if err := toml.Unmarshal([]byte(read(t)), &raw); err != nil {
		t.Fatalf("file does not parse: %v\n%s", err, read(t))
	}
	for k, v := range map[string]string{"library": "/x/lib", "zeta": "top", "theme": "dark", "editor": "vim", "lang": "en"} {
		if raw[k] != v {
			t.Errorf("top-level %s = %v, want %q\n%s", k, raw[k], v, read(t))
		}
	}
	extra, _ := raw["extra"].(map[string]any)
	if len(extra) != 3 || extra["name"] != "in table" || extra["key"] != int64(1) {
		t.Errorf("table changed: %v\n%s", extra, read(t))
	}
	if deep, _ := extra["deep"].(map[string]any); deep == nil || len(deep["list"].([]any)) != 2 {
		t.Errorf("nested table changed: %v", extra["deep"])
	}
}
