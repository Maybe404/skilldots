package cli

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
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

// contrast is the WCAG contrast ratio of two #RRGGBB colours.
func contrast(a, b lipgloss.Color) float64 {
	lum := func(c lipgloss.Color) float64 {
		var v [3]float64
		for i := range v {
			n, _ := strconv.ParseUint(string(c)[1+2*i:3+2*i], 16, 8)
			x := float64(n) / 255
			if x <= 0.03928 {
				v[i] = x / 12.92
			} else {
				v[i] = math.Pow((x+0.055)/1.055, 2.4)
			}
		}
		return 0.2126*v[0] + 0.7152*v[1] + 0.0722*v[2]
	}
	la, lb := lum(a), lum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Secondary text (Muted) and the least important text (Faint) stay
// readable — WCAG AA, 4.5:1 — on every background the CLI and the
// dashboard paint them on, and keep their order: Faint is further from
// Text than Muted. Faint is never drawn on the bars, and highlighted rows
// (sel, selDim) draw it as Muted.
func TestPaletteContrast(t *testing.T) {
	faintOn := map[string]bool{"ink": true, "panel": true, "modal": true}
	for name, p := range map[string]palette{"dark": darkPalette, "light": lightPalette} {
		for bgName, bg := range dashboardTheme(p).Backgrounds() {
			for fgName, fg := range map[string]lipgloss.Color{"Text": p.Text, "Muted": p.Muted, "Faint": p.Faint} {
				if fgName == "Faint" && !faintOn[bgName] {
					continue
				}
				if r := contrast(fg, bg); r < 4.5 {
					t.Errorf("%s: %s %s on %s %s: %.2f:1", name, fgName, fg, bgName, bg, r)
				}
			}
		}
		if !(contrast(p.Text, p.Ink) > contrast(p.Muted, p.Ink) && contrast(p.Muted, p.Ink) > contrast(p.Faint, p.Ink)) {
			t.Errorf("%s: Text, Muted, Faint should get quieter: %.2f, %.2f, %.2f on Ink", name,
				contrast(p.Text, p.Ink), contrast(p.Muted, p.Ink), contrast(p.Faint, p.Ink))
		}
	}
}
