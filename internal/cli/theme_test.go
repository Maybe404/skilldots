package cli

import (
	"testing"
	"time"
)

func TestSetupTheme(t *testing.T) {
	asked := 0
	detected := true
	detectBackground = func(time.Duration) (bool, bool) { asked++; return detected, true }
	stdoutIsTerminal = func() bool { return true }
	t.Cleanup(func() { usePalette(darkPalette); themeDone = "-" })
	set := func(flag, env, noColor string) error {
		t.Helper()
		themeDone = "-"
		t.Setenv(EnvTheme, env)
		t.Setenv("NO_COLOR", noColor)
		t.Setenv("TERM", "xterm-256color")
		return setupTheme(flag)
	}
	cases := []struct {
		flag, env, noColor string
		light              bool
		asks               int
		err                bool
	}{
		{flag: "", light: true, asks: 1},           // auto, terminal says light
		{flag: "dark", light: false},               // forced
		{flag: "LIGHT", light: true},               // case-insensitive
		{env: "light", light: true},                // env when no flag
		{flag: "dark", env: "light", light: false}, // flag wins
		{flag: "auto", noColor: "1", light: false}, // NO_COLOR: never ask
		{flag: "blue", err: true},                  // invalid flag
		{env: "sepia", err: true},                  // invalid env
	}
	for _, c := range cases {
		asked = 0
		err := set(c.flag, c.env, c.noColor)
		if (err != nil) != c.err {
			t.Errorf("%+v: err %v", c, err)
			continue
		}
		if c.err {
			continue
		}
		if pal.Light != c.light || asked != c.asks {
			t.Errorf("%+v: light %v asked %d", c, pal.Light, asked)
		}
		if sAccent.GetForeground() != pal.Accent {
			t.Errorf("%+v: styles not rebuilt", c)
		}
	}
	// A terminal that does not answer: dark.
	detectBackground = func(time.Duration) (bool, bool) { return false, false }
	if err := set("", "", ""); err != nil || pal.Light {
		t.Fatalf("no answer: light %v err %v", pal.Light, err)
	}
	// Asked once per process, even when the menu runs commands in-process.
	asked = 0
	detectBackground = func(time.Duration) (bool, bool) { asked++; return true, true }
	themeDone = "-"
	setupTheme("")
	setupTheme("")
	if asked != 1 {
		t.Fatalf("asked %d times", asked)
	}
}
