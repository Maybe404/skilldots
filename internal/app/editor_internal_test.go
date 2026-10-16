package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Maybe404/skilldots/internal/config"
)

func TestFindEditorOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	path := map[string]string{}
	lookPath = func(f string) (string, error) {
		if p, ok := path[f]; ok {
			return p, nil
		}
		return "", exec.ErrNotFound
	}
	realDirs, realGOOS := appDirs, goos
	appDirs = func() []string { return []string{filepath.Join(home, "Applications")} }
	t.Cleanup(func() { lookPath, goos, appDirs = exec.LookPath, realGOOS, realDirs })
	check := func(want []string, terminal bool, from string) {
		t.Helper()
		e, err := FindEditor()
		if err != nil || !slices.Equal(e.Argv, want) || e.Terminal != terminal || e.From != from {
			t.Fatalf("got %+v %v, want %v terminal=%v from %s", e, err, want, terminal, from)
		}
	}
	goos = "linux"
	if _, err := FindEditor(); err == nil {
		t.Fatal("no xdg-open and nothing else: want an error")
	}
	path["xdg-open"] = "/usr/bin/xdg-open"
	check([]string{"xdg-open"}, false, "system default")
	goos = "darwin"
	path["open"] = "/usr/bin/open"
	check([]string{"open"}, false, "system default")
	os.MkdirAll(filepath.Join(home, "Applications", "Zed.app"), 0o755)
	check([]string{"open", "-a", filepath.Join(home, "Applications", "Zed.app")}, false, "found on this machine")
	path["subl"] = "/bin/subl"
	check([]string{"/bin/subl"}, false, "found on this machine")
	path["cursor"] = "/bin/cursor"
	check([]string{"/bin/cursor"}, false, "found on this machine") // cursor before subl
	t.Setenv("EDITOR", "nano -w")
	check([]string{"nano", "-w"}, true, "$EDITOR")
	t.Setenv("VISUAL", "code --wait")
	check([]string{"code", "--wait"}, false, "$VISUAL")
	if err := config.Set(config.Editor, "'/opt/my editor/vim'"); err != nil {
		t.Fatal(err)
	}
	check([]string{"/opt/my editor/vim"}, true, "editor setting")
	config.Set(config.Editor, `"unterminated`)
	if _, err := FindEditor(); err == nil || errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("bad setting: %v", err)
	}
}

// Editor commands split as a POSIX shell would, and on Windows a
// backslash is part of a path, never an escape.
func TestSplitCommandQuoting(t *testing.T) {
	realGOOS := goos
	t.Cleanup(func() { goos = realGOOS })
	cases := []struct {
		goos, in string
		want     []string
	}{
		{"darwin", `code --wait "a\"b" "c\d" "e\\f" "g\$h" 'i\j' k\ l`, []string{"code", "--wait", `a"b`, `c\d`, `e\f`, `g$h`, `i\j`, "k l"}},
		{"linux", `vim "a\` + "`" + `b"`, []string{"vim", "a`b"}},
		{"windows", `"C:\Program Files\Microsoft VS Code\bin\code.cmd" --wait`, []string{`C:\Program Files\Microsoft VS Code\bin\code.cmd`, "--wait"}},
		{"windows", `C:\tools\nvim.exe -p`, []string{`C:\tools\nvim.exe`, "-p"}},
		{"windows", `notepad++.exe "D:\x y\"`, []string{"notepad++.exe", `D:\x y\`}},
	}
	for _, c := range cases {
		goos = c.goos
		got, err := SplitCommand(c.in)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s %s: got %q %v, want %q", c.goos, c.in, got, err, c.want)
		}
	}
}
