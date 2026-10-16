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
	ovList
)

type pickItem struct {
	label, detail, value string
	on, locked           bool
	// note replaces detail on a locked item ("installed").
	note string
	// header marks a group heading; space on it toggles the whole group.
	header bool
	group  string
}

// step is what a key in an overlay leads to: another overlay, a job, or
// (both nil) closing.
type step struct {
	next *overlay
	job  *job
}

type choice struct {
	key, label string
	then       func() step
}

// listAction is a key a list overlay offers on the item under the cursor.
type listAction struct {
	key, label string
	run        func(it pickItem) step
}

// overlay is a modal drawn over the body. It owns its keys until closed.
type overlay struct {
	kind  ovKind
	title string
	desc  string
	lines []string
	off   int // help: scroll offset

	input   textinput.Model
	verb    string // input: what enter does ("fetch", "next")
	onInput func(v string) (step, error)
	err     string

	items    []pickItem
	cur      int
	allowAll bool
	all      bool
	onPick   func(values []string, all bool) step
	actions  []listAction
	empty    string // list: shown when there are no items

	choices []choice
	// noEnter keeps enter from picking the first choice (irreversible ones).
	noEnter bool
}

func inputOverlay(title, desc, placeholder, value string, t Theme, s surfaces, onInput func(string) (step, error)) *overlay {
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
	in.SetValue(value)
	in.CursorEnd()
	in.Focus()
	return &overlay{kind: ovInput, title: title, desc: desc, input: in, verb: "next", onInput: onInput}
}

func pickOverlay(title, desc string, items []pickItem, allowAll bool, onPick func([]string, bool) step) *overlay {
	o := &overlay{kind: ovPick, title: title, desc: desc, items: items, allowAll: allowAll, onPick: onPick}
	o.cur = o.firstSelectable()
	return o
}

func listOverlay(title, desc, empty string, items []pickItem, actions ...listAction) *overlay {
	o := &overlay{kind: ovList, title: title, desc: desc, items: items, actions: actions, empty: empty}
	o.cur = o.firstSelectable()
	return o
}

func confirmOverlay(title string, lines []string, choices ...choice) *overlay {
	return &overlay{kind: ovConfirm, title: title, lines: lines, choices: choices}
}

// jobChoice is a confirm choice that starts j.
func jobChoice(key, label string, j job) choice {
	return choice{key: key, label: label, then: func() step { return step{job: &j} }}
}

// helpKeys lists every key of the dashboard. TestHelpListsEveryKey keeps it
// in step with updateKey.
var helpKeys = [][2]string{
	{"↑↓ j k", "move"},
	{"pgup pgdn ^u ^d", "page up / down"},
	{"g G home end", "top / end"},
	{"tab ← → h l", "switch column"},
	{"enter", "open the pack under the cursor"},
	{"/", "filter this column (esc clears)"},
	{"space", "enable / disable the skill (installs an upstream one)"},
	{"a", "add a repository: pick skills or categories, alias, agents"},
	{"u / U", "update this pack / every pack"},
	{"e", "choose agents for the skill or pack"},
	{"f", "fork a vendored or app skill into your library (rename it there)"},
	{"d", "remove skill (to trash) · unlink yours · purge pack"},
	{"n", "new skill of your own in skills/ or private/"},
	{"S", "scan agent directories and adopt what skd does not manage"},
	{"t", "trash: restore what skd set aside, or empty it"},
	{"s", "back up: commit and push the library"},
	{"A", "apply pending link changes"},
	{"r", "reload"},
	{"?", "this help"},
	{"q esc", "quit (esc clears a filter first)"},
	{"ctrl+c", "quit · while busy: quit when done, twice: stop now"},
}

func helpOverlay() *overlay {
	var lines []string
	for _, k := range helpKeys {
		lines = append(lines, fit(k[0], 16)+"  "+k[1])
	}
	lines = append(lines, "",
		"Nothing is deleted outright: removed files go to <library>/.trash;",
		"t lists them and restores them. Emptying the trash asks twice.")
	return &overlay{kind: ovHelp, title: "Keys", lines: lines}
}

func (o *overlay) values() []string {
	var out []string
	for _, it := range o.items {
		if it.on && !it.header {
			out = append(out, it.value)
		}
	}
	return out
}

func (o *overlay) selectable(i int) bool {
	it := o.items[i]
	if o.kind == ovList {
		return !it.header
	}
	return !it.locked || it.header
}

func (o *overlay) firstSelectable() int {
	for i := range o.items {
		if o.selectable(i) {
			return i
		}
	}
	return 0
}

// moveCur moves the cursor by d, skipping rows that cannot be chosen.
func (o *overlay) moveCur(d int) {
	n := len(o.items)
	if n == 0 {
		return
	}
	i := min(max(o.cur+d, 0), n-1)
	step := 1
	if d < 0 {
		step = -1
	}
	for j := i; j >= 0 && j < n; j += step {
		if o.selectable(j) {
			o.cur = j
			return
		}
	}
	for j := i; j >= 0 && j < n; j -= step {
		if o.selectable(j) {
			o.cur = j
			return
		}
	}
}

// toggle flips the item under the cursor; on a group heading it ticks the
// whole group, or clears it when every item is ticked already.
func (o *overlay) toggle() {
	if o.cur >= len(o.items) {
		return
	}
	it := &o.items[o.cur]
	if !it.header {
		if !it.locked {
			it.on = !it.on
		}
		return
	}
	all := true
	for _, x := range o.items {
		if !x.header && x.group == it.group && !x.locked && !x.on {
			all = false
		}
	}
	for i := range o.items {
		x := &o.items[i]
		if !x.header && x.group == it.group && !x.locked {
			x.on = !all
		}
	}
}

// update handles a key. It returns the overlay to keep (nil closes it) and
// a job to start, if the user confirmed one.
func (o *overlay) update(k tea.KeyMsg) (*overlay, *job, tea.Cmd) {
	key := k.String()
	if key == "esc" {
		return nil, nil, nil
	}
	done := func(s step) (*overlay, *job, tea.Cmd) {
		if s.next != nil && s.next.kind == ovInput {
			return s.next, s.job, textinput.Blink
		}
		return s.next, s.job, nil
	}
	switch o.kind {
	case ovHelp:
		switch key {
		case "up", "k":
			o.off = max(0, o.off-1)
			return o, nil, nil
		case "down", "j":
			o.off = min(o.off+1, max(0, len(o.lines)-1))
			return o, nil, nil
		}
		return nil, nil, nil
	case ovConfirm:
		if key == "n" || key == "q" {
			return nil, nil, nil
		}
		for i, c := range o.choices {
			if key == c.key || (i == 0 && key == "enter" && !o.noEnter) {
				return done(c.then())
			}
		}
		return o, nil, nil
	case ovInput:
		if key == "enter" {
			s, err := o.onInput(strings.TrimSpace(o.input.Value()))
			if err != nil {
				o.err = err.Error()
				return o, nil, nil
			}
			return done(s)
		}
		o.err = ""
		var cmd tea.Cmd
		o.input, cmd = o.input.Update(k)
		return o, nil, cmd
	case ovPick, ovList:
		switch key {
		case "up", "k":
			o.moveCur(-1)
		case "down", "j":
			o.moveCur(1)
		case "pgup":
			o.moveCur(-10)
		case "pgdown":
			o.moveCur(10)
		case "home", "g":
			o.moveCur(-len(o.items))
		case "end", "G":
			o.moveCur(len(o.items))
		case "q":
			return nil, nil, nil
		}
		if o.kind == ovList {
			for i, a := range o.actions {
				if (key == a.key || (i == 0 && key == "enter")) && o.cur < len(o.items) {
					return done(a.run(o.items[o.cur]))
				}
			}
			return o, nil, nil
		}
		switch key {
		case " ", "x":
			o.toggle()
		case "*":
			if o.allowAll {
				o.all = !o.all
			}
		case "enter":
			return done(o.onPick(o.values(), o.all))
		}
		return o, nil, nil
	}
	return o, nil, nil
}

// render draws the modal at width w with at most h rows.
func (o *overlay) render(w, h int, t Theme, s surfaces) []string {
	if w < 8 || h < 1 {
		return nil
	}
	bg := s.modal
	row := func(segs ...seg) string {
		return paint(w, bg, append([]seg{sg("  ", "")}, segs...)...)
	}
	inner := w - 4
	lines := []string{row(), row(sgb(o.title, t.Text))}
	for _, d := range wrap(o.desc, inner, 3) {
		lines = append(lines, row(sg(d, t.Muted)))
	}
	lines = append(lines, row())
	var foot []seg
	key := func(k, label string) []seg { return []seg{sgb(k, t.Accent), sg(" "+label+"   ", t.Text)} }
	switch o.kind {
	case ovHelp:
		avail := max(1, h-len(lines)-4)
		off := min(o.off, max(0, len(o.lines)-avail))
		for i := off; i < len(o.lines) && i < off+avail; i++ {
			lines = append(lines, row(sg(o.lines[i], t.Text)))
		}
		if len(o.lines) > avail {
			foot = append(foot, key("↑↓", "scroll")...)
			foot = append(foot, sg(ansiRange(off, avail, len(o.lines))+"   ", t.Muted))
		}
		foot = append(foot, sg("any other key closes", t.Muted))
	case ovConfirm:
		for _, l := range o.lines {
			lines = append(lines, row(sg(l, t.Text)))
		}
		lines = append(lines, row())
		for i, c := range o.choices {
			k := c.key
			if i == 0 && !o.noEnter {
				k += "/enter"
			}
			foot = append(foot, key(k, c.label)...)
		}
		foot = append(foot, key("esc", "cancel")...)
	case ovInput:
		v := o.input.View()
		vw := ansi.StringWidth(v)
		if vw > inner {
			v = ansi.Truncate(v, inner, "")
			vw = inner
		}
		lines = append(lines, paint(2, bg)+v+paint(w-2-vw, bg))
		if o.err != "" {
			lines = append(lines, row(sg("✗ "+o.err, t.Err)))
		}
		foot = append(key("enter", o.verb), key("esc", "cancel")...)
	case ovPick, ovList:
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
			if !it.header {
				labelW = max(labelW, ansi.StringWidth(it.label))
			}
		}
		labelW = min(labelW, max(12, inner/2))
		if len(o.items) == 0 {
			lines = append(lines, row(sg(o.empty, t.Muted)))
		}
		for i := off; i < len(o.items) && i < off+avail; i++ {
			it := o.items[i]
			rbg := bg
			mark := sg("  ", "")
			if i == o.cur {
				rbg = s.sel
				mark = sgb("❯ ", t.Accent)
			}
			if it.header {
				segs := []seg{mark}
				if o.kind == ovPick {
					segs = append(segs, o.box(i, t))
				}
				segs = append(segs, sgb(it.label, t.Muted), sg("  "+it.detail, t.Faint))
				lines = append(lines, paint(2, bg)+paint(w-2, rbg, segs...))
				continue
			}
			detail, dfg := it.detail, t.Muted
			if it.locked && it.note != "" {
				detail, dfg = it.note, t.Faint
			}
			segs := []seg{mark}
			if o.kind == ovPick {
				segs = append(segs, o.box(i, t))
			}
			lfg := t.Text
			if it.locked && o.kind == ovPick {
				lfg = t.Muted
			}
			segs = append(segs, sg(fit(it.label, labelW)+"  ", lfg), sg(detail, dfg))
			lines = append(lines, paint(2, bg)+paint(w-2, rbg, segs...))
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
		if o.kind == ovPick {
			foot = key("space", "toggle")
			if o.allowAll {
				foot = append(foot, key("*", "all")...)
			}
			foot = append(foot, key("enter", "confirm")...)
		} else {
			for i, a := range o.actions {
				k := a.key
				if i == 0 {
					k += "/enter"
				}
				foot = append(foot, key(k, a.label)...)
			}
		}
		foot = append(foot, key("esc", "cancel")...)
	}
	lines = append(lines, row(), row(foot...), row())
	if len(lines) > h {
		lines = append(lines[:max(0, h-1)], lines[len(lines)-1])
	}
	return lines
}

// box is the tick mark of pick row i; a heading shows its group's state.
func (o *overlay) box(i int, t Theme) seg {
	it := o.items[i]
	if it.header {
		n, on := 0, 0
		for _, x := range o.items {
			if !x.header && x.group == it.group && !x.locked {
				n++
				if x.on || o.all {
					on++
				}
			}
		}
		switch {
		case n > 0 && on == n:
			return sg("◉ ", t.OK)
		case on > 0:
			return sg("◐ ", t.OK)
		}
		return sg("○ ", t.Faint)
	}
	switch {
	case it.locked:
		return sg("◉ ", t.Muted)
	case it.on || o.all:
		return sg("◉ ", t.OK)
	}
	return sg("○ ", t.Faint)
}

// ansiRange describes a scroll window, e.g. "4–15 of 38".
func ansiRange(off, h, n int) string {
	end := min(off+h, n)
	return itoa(off+1) + "–" + itoa(end) + " of " + itoa(n)
}
