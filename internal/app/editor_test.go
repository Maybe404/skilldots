package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
)

// Edit refuses a vendored skill with *OverwrittenError unless told to
// fork or force, and never forks when the editor could not be started
// anyway (not installed, or a terminal editor with no terminal): the
// library is left exactly as it was.
func TestEditRefusesBeforeChanging(t *testing.T) {
	up := upstream(t)
	home, root := side(t, up)
	runCLI(t, root, "add", "acme/skills", "--skill", "tdd")
	vim := filepath.Join(home, "bin", "vim")
	write(t, vim, "#!/bin/sh\nexit 0\n")
	os.Chmod(vim, 0o755)
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", vim)
	before := state(t, home, root)
	unchanged := func(what string) {
		t.Helper()
		after := state(t, home, root)
		for k, v := range before {
			if after[k] != v {
				t.Fatalf("%s changed %s:\n%s\n→\n%s", what, k, v, after[k])
			}
		}
	}

	s, _ := app.Open(root)
	_, err := s.Edit(app.KeyRef("tdd"), app.EditOptions{})
	var over *app.OverwrittenError
	if !errors.As(err, &over) || over.Entry.LinkName != "tdd" {
		t.Fatalf("vendored without fork or force: %v", err)
	}
	unchanged("refused edit")

	s, _ = app.Open(root)
	if _, err := s.Edit(app.KeyRef("tdd"), app.EditOptions{Fork: true, NoTerminal: true}); err == nil {
		t.Fatal("terminal editor without a terminal: no error")
	}
	unchanged("--fork with vim and no terminal")

	t.Setenv("VISUAL", filepath.Join(home, "bin", "no-such-editor"))
	s, _ = app.Open(root)
	if _, err := s.Edit(app.KeyRef("tdd"), app.EditOptions{Fork: true}); err == nil {
		t.Fatal("missing editor: no error")
	}
	unchanged("--fork with a missing editor")

	// With a terminal the same fork goes ahead and opens the copy.
	t.Setenv("VISUAL", vim)
	s, _ = app.Open(root)
	out, err := s.Edit(app.KeyRef("tdd"), app.EditOptions{Fork: true})
	if err != nil || out.Fork == nil || out.Dir != filepath.Join(root, "skills", "tdd") || !out.Editor.Terminal {
		t.Fatalf("fork at a terminal: %v %+v", err, out)
	}
}
