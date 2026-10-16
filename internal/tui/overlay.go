package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type ovKind int

const (
	ovInput ovKind = iota
	ovPick
	ovConfirm
	ovHelp
)

type pickItem struct {
	label, detail, value string
	on, locked           bool
}

type choice struct {
	key, label string
	job        job
}

// overlay is a modal drawn over the body. It owns its keys until closed.
type overlay struct {
	kind  ovKind
	title string
	desc  string
	lines []string

	input textinput.Model

	items    []pickItem
	cur      int
	allowAll bool
	all      bool
	onPick   func(values []string, all bool) *job

	choices []choice
}

func inputOverlay(title, desc, placeholder string, t Theme, s surfaces) *overlay {
	in := textinput.New()
	in.Placeholder = placeholder
	in.Prompt = "❯ "
	in.CharLimit = 200
	bg := lipgloss.NewStyle().Background(s.modal)
	in.PromptStyle = bg.Foreground(t.Accent)
	in.TextStyle = bg.Foreground(t.Text)
	in.PlaceholderStyle = bg.Foreground(t.Muted)
	in.Cursor.Style = lipgloss.NewStyle().Foreground(t.Accent)
	in.Cursor.TextStyle = bg.Foreground(t.Text)
	in.Focus()
	return &overlay{kind: ovInput, title: title, desc: desc, input: in}
}

func pickOverlay(title, desc string, items []pickItem, allowAll bool, onPick func([]string, bool) *job) *overlay {
	return &overlay{kind: ovPick, title: title, desc: desc, items: items, allowAll: allowAll, onPick: onPick}
}

func confirmOverlay(title string, lines []string, choices ...choice) *overlay {
	return &overlay{kind: ovConfirm, title: title, lines: lines, choices: choices}
}

func helpOverlay() *overlay {
	return &overlay{kind: ovHelp, title: "Keys", lines: []string{
		"↑↓ / j k      move            pgup pgdn g G   page, top, end",
		"tab / ← →     switch column   enter           open pack",
		"space         enable / disable the skill (installs an upstream one)",
		"a             add a repository and pick skills",
		"u / U         update this pack / every pack",
		"e             choose agents for the skill or pack",
		"f             fork a vendored skill into your library",
		"d             remove skill (to trash) · unlink yours · purge pack",
		"s             back up: commit and push the library",
		"A             apply pending link changes      r  reload",
		"/             filter this column (esc clears)",
		"q / esc       quit",
		"",
		"Nothing is deleted outright: removed files go to <library>/.trash",
		"and `skd trash restore <id>` brings them back.",
	}}
}

func (o *overlay) values() []string {
	var out []string
	for _, it := range o.items {
		if it.on {
			out = append(out, it.value)
		}
	}
	return out
}

// update handles a key. It returns the overlay to keep (nil closes it) and
// a job to start, if the user confirmed one.
func (o *overlay) update(k tea.KeyMsg) (*overlay, *job, tea.Cmd) {
	key := k.String()
	if key == "esc" {
		return nil, nil, nil
	}
	switch o.kind {
	case ovHelp:
		return nil, nil, nil
	case ovConfirm:
		if key == "n" || key == "q" {
			return nil, nil, nil
		}
		for i, c := range o.choices {
			if key == c.key || (i == 0 && key == "enter") {
				j := c.job
				return nil, &j, nil
			}
		}
		return o, nil, nil
	case ovInput:
		if key == "enter" {
			src := strings.TrimSpace(o.input.Value())
			if src == "" {
				return nil, nil, nil
			}
			return nil, &job{label: "fetching " + src, run: func(ops Ops) result {
				b, err := ops.Browse(src)
				return result{browse: b, err: err, src: src}
			}}, nil
		}
		var cmd tea.Cmd
		o.input, cmd = o.input.Update(k)
		return o, nil, cmd
	case ovPick:
		switch key {
		case "up", "k":
			if o.cur > 0 {
				o.cur--
			}
		case "down", "j":
			if o.cur < len(o.items)-1 {
				o.cur++
			}
		case "pgup":
			o.cur = max(0, o.cur-10)
		case "pgdown":
			o.cur = min(len(o.items)-1, o.cur+10)
		case " ", "x":
			if o.cur < len(o.items) && !o.items[o.cur].locked {
				o.items[o.cur].on = !o.items[o.cur].on
			}
		case "*":
			if o.allowAll {
				o.all = !o.all
			}
		case "enter":
			j := o.onPick(o.values(), o.all)
			return nil, j, nil
		case "q":
			return nil, nil, nil
		}
		return o, nil, nil
	}
	return o, nil, nil
}

// render draws the modal at width w with at most h rows.
func (o *overlay) render(w, h int, t Theme, s surfaces) []string {
	bg := s.modal
	row := func(segs ...seg) string {
		return paint(w, bg, append([]seg{sg("  ", "")}, segs...)...)
	}
	inner := w - 4
	lines := []string{row(), row(sgb(o.title, t.Text))}
	if o.desc != "" {
		lines = append(lines, row(sg(o.desc, t.Muted)))
	}
	lines = append(lines, row())
	var foot []seg
	switch o.kind {
	case ovHelp:
		for _, l := range o.lines {
			lines = append(lines, row(sg(l, t.Text)))
		}
		foot = []seg{sg("any key closes", t.Muted)}
	case ovConfirm:
		for _, l := range o.lines {
			lines = append(lines, row(sg(l, t.Text)))
		}
		lines = append(lines, row())
		for _, c := range o.choices {
			k := c.key
			if k == o.choices[0].key {
				k += "/enter"
			}
			foot = append(foot, sgb(k, t.Accent), sg(" "+c.label+"   ", t.Text))
		}
		foot = append(foot, sgb("esc", t.Accent), sg(" cancel", t.Text))
	case ovInput:
		v := o.input.View()
		vw := ansi.StringWidth(v)
		if vw > inner {
			v = ansi.Truncate(v, inner, "")
			vw = inner
		}
		lines = append(lines, paint(2, bg)+v+paint(w-2-vw, bg))
		foot = []seg{sgb("enter", t.Accent), sg(" fetch   ", t.Text), sgb("esc", t.Accent), sg(" cancel", t.Text)}
	case ovPick:
		avail := h - len(lines) - 4
		if o.allowAll {
			avail--
		}
		avail = max(1, avail)
		off := 0
		if o.cur >= avail {
			off = o.cur - avail + 1
		}
		labelW := 0
		for _, it := range o.items {
			labelW = max(labelW, ansi.StringWidth(it.label))
		}
		labelW = min(labelW, max(12, inner/2))
		for i := off; i < len(o.items) && i < off+avail; i++ {
			it := o.items[i]
			rbg := bg
			mark := sg("  ", "")
			if i == o.cur {
				rbg = s.sel
				mark = sgb("❯ ", t.Accent)
			}
			box := sg("○ ", t.Faint)
			switch {
			case it.locked:
				box = sg("◉ ", t.Muted)
			case it.on || o.all:
				box = sg("◉ ", t.OK)
			}
			detail := it.detail
			if it.locked {
				detail = "installed"
			}
			lines = append(lines, paint(2, bg)+paint(w-2, rbg, mark, box, sg(fit(it.label, labelW)+"  ", t.Text), sg(detail, t.Muted)))
		}
		if n := len(o.items); n > avail {
			lines = append(lines, row(sg(ansiRange(off, avail, n), t.Muted)))
		}
		if o.allowAll {
			mark := sg("○ ", t.Faint)
			if o.all {
				mark = sg("◉ ", t.OK)
			}
			lines = append(lines, row(mark, sg("follow everything, including skills added upstream later", t.Text)))
		}
		foot = []seg{sgb("space", t.Accent), sg(" toggle   ", t.Text)}
		if o.allowAll {
			foot = append(foot, sgb("*", t.Accent), sg(" all   ", t.Text))
		}
		foot = append(foot, sgb("enter", t.Accent), sg(" confirm   ", t.Text), sgb("esc", t.Accent), sg(" cancel", t.Text))
	}
	lines = append(lines, row(), row(foot...), row())
	if len(lines) > h {
		lines = append(lines[:h-1], lines[len(lines)-1])
	}
	return lines
}

// ansiRange describes a scroll window, e.g. "4–15 of 38".
func ansiRange(off, h, n int) string {
	end := min(off+h, n)
	return itoa(off+1) + "–" + itoa(end) + " of " + itoa(n)
}
