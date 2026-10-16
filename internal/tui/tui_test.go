package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestQuietGitKeepsUserSSHCommand(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home, _ := sandbox(t)
	root := newLibrary(t, home)
	gc := os.Getenv("GIT_CONFIG_GLOBAL")
	base, err := os.ReadFile(gc)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name          string
		global, local string
		env, envSSH   string
		want          string
	}{
		{name: "nothing set: batch mode", want: "ssh -o BatchMode=yes"},
		{name: "global core.sshCommand", global: "ssh -i ~/.ssh/work", want: ""},
		{name: "library core.sshCommand", local: "ssh -F /dev/null", want: ""},
		{name: "GIT_SSH_COMMAND set", env: "ssh -v", want: "ssh -v"},
		{name: "GIT_SSH set", envSSH: "/usr/bin/ssh", want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("GIT_TERMINAL_PROMPT", "")
			t.Setenv("GIT_SSH_COMMAND", c.env)
			t.Setenv("GIT_SSH", c.envSSH)
			cfg := string(base)
			if c.global != "" {
				cfg += "[core]\n\tsshCommand = " + c.global + "\n"
			}
			if err := os.WriteFile(gc, []byte(cfg), 0o644); err != nil {
				t.Fatal(err)
			}
			local := filepath.Join(root, ".git", "config")
			orig, _ := os.ReadFile(local)
			defer os.WriteFile(local, orig, 0o644)
			if c.local != "" {
				run(t, root, "config", "core.sshCommand", c.local)
			}
			quietGit(root)
			if got := os.Getenv("GIT_SSH_COMMAND"); got != c.want {
				t.Errorf("GIT_SSH_COMMAND = %q, want %q", got, c.want)
			}
			if os.Getenv("GIT_TERMINAL_PROMPT") != "0" {
				t.Error("GIT_TERMINAL_PROMPT not 0")
			}
		})
	}
	os.WriteFile(gc, base, 0o644)
}
