package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

// Theme is the palette the dashboard paints with. The CLI passes in the
// colours from internal/cli/ui.go so the dashboard, the huh menu and the
// plain commands look like one product; tui never defines its own palette.
type Theme struct {
	Accent, Cyan, Muted, Faint, OK, Warn, Err, Text lipgloss.Color
	// Ink is the darkest tone (text on accent buttons in the CLI theme). It
	// anchors the surface shades below.
	Ink lipgloss.Color
}

// surfaces are derived from the palette: Ink tinted towards Faint or
// Accent, so layers read as depth rather than as new colours.
type surfaces struct {
	bar, panel, sel, selDim, modal lipgloss.Color
}

func blend(a, b lipgloss.Color, t float64) lipgloss.Color {
	ca, err1 := colorful.Hex(string(a))
	cb, err2 := colorful.Hex(string(b))
	if err1 != nil || err2 != nil {
		return b
	}
	return lipgloss.Color(ca.BlendLab(cb, t).Clamped().Hex())
}

func (t Theme) surfaces() surfaces {
	return surfaces{
		bar:    blend(t.Ink, t.Faint, 0.85),
		panel:  blend(t.Ink, t.Faint, 0.30),
		sel:    blend(t.Ink, t.Accent, 0.38),
		selDim: blend(t.Ink, t.Faint, 0.70),
		modal:  blend(t.Ink, t.Faint, 0.55),
	}
}

// seg is one run of text with a foreground colour. Rows are built from
// segments and painted onto a background in one pass, so inner styles never
// reset the background half-way across a bar.
type seg struct {
	s    string
	fg   lipgloss.Color
	bold bool
}

func sg(s string, fg lipgloss.Color) seg  { return seg{s: s, fg: fg} }
func sgb(s string, fg lipgloss.Color) seg { return seg{s: s, fg: fg, bold: true} }

func segWidth(segs []seg) (n int) {
	for _, s := range segs {
		n += ansi.StringWidth(s.s)
	}
	return n
}

// paint renders segs on bg into exactly w cells, truncating with an
// ellipsis or padding with bg. An empty bg means the terminal background.
func paint(w int, bg lipgloss.Color, segs ...seg) string {
	if w <= 0 {
		return ""
	}
	base := lipgloss.NewStyle()
	if bg != "" {
		base = base.Background(bg)
	}
	var b strings.Builder
	used := 0
	for _, s := range segs {
		if used >= w {
			break
		}
		text := s.s
		if sw := ansi.StringWidth(text); used+sw > w {
			text = ansi.Truncate(text, w-used, "…")
		}
		st := base.Bold(s.bold)
		if s.fg != "" {
			st = st.Foreground(s.fg)
		}
		b.WriteString(st.Render(text))
		used += ansi.StringWidth(text)
	}
	if used < w {
		b.WriteString(base.Render(strings.Repeat(" ", w-used)))
	}
	return b.String()
}

// spread paints left-aligned and right-aligned segments on one row. The
// right side is dropped when both do not fit.
func spread(w int, bg lipgloss.Color, left, right []seg) string {
	lw, rw := segWidth(left), segWidth(right)
	if lw+rw+1 > w {
		return paint(w, bg, left...)
	}
	row := append([]seg{}, left...)
	row = append(row, sg(strings.Repeat(" ", w-lw-rw), ""))
	row = append(row, right...)
	return paint(w, bg, row...)
}

// gradient spells s one rune at a time, blending from a to b.
func gradient(s string, a, b lipgloss.Color, bold bool) []seg {
	r := []rune(s)
	out := make([]seg, 0, len(r))
	for i, c := range r {
		t := 0.0
		if len(r) > 1 {
			t = float64(i) / float64(len(r)-1)
		}
		out = append(out, seg{s: string(c), fg: blend(a, b, t), bold: bold})
	}
	return out
}

// fit right-pads or truncates plain text to n cells.
func fit(s string, n int) string {
	if n <= 0 {
		return ""
	}
	w := ansi.StringWidth(s)
	if w > n {
		return ansi.Truncate(s, n, "…")
	}
	return s + strings.Repeat(" ", n-w)
}
