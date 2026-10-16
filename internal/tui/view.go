package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Maybe404/skilldots/internal/engine"
)

// Minimum size the full layout needs; below it a notice is shown instead.
const (
	minWidth  = 60
	minHeight = 12
)

func itoa(n int) string { return strconv.Itoa(n) }

func tildify(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// tildifyAll shortens every home-directory path inside a message.
func tildifyAll(s string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" {
		return strings.ReplaceAll(s, home, "~")
	}
	return s
}

func (m Model) bodyHeight() int { return max(1, m.h-3) }

func (m Model) leftWidth() int {
	return min(44, max(28, m.w*34/100))
}

func (m Model) rightWidth() int { return max(1, m.w-m.leftWidth()-1) }

func (m Model) filterShown(f focus) bool {
	return m.filter[f] != "" || (m.filtering && m.focus == f)
}

// leftHeight is the number of list rows in the left column: the body minus
// an optional filter line and the scroll line.
func (m Model) leftHeight() int {
	h := m.bodyHeight() - 1
	if m.filterShown(focusLeft) {
		h--
	}
	return max(1, h)
}

func (m Model) rightListHeight() int {
	p := m.currentPack()
	if p == nil {
		return 1
	}
	h := m.bodyHeight() - len(m.packHeader(*p, m.rightWidth())) - 1 /*column titles*/ - 1 /*scroll*/ - m.descLines()
	if m.filterShown(focusRight) {
		h--
	}
	return max(1, h)
}

// descLines is how many lines under a pack's list describe the skill
// under the cursor; short terminals give them to the list instead.
func (m Model) descLines() int {
	if m.bodyHeight() < 16 {
		return 0
	}
	return 2
}

// View renders the whole screen.
func (m Model) View() string {
	if m.quitting || m.w == 0 || m.h == 0 {
		return ""
	}
	t := m.theme
	if m.w < minWidth || m.h < minHeight {
		var out []string
		for i := 0; i < m.h; i++ {
			out = append(out, paint(m.w, ""))
		}
		msg := fmt.Sprintf("window too small (%d×%d) · need %d×%d · q quits", m.w, m.h, minWidth, minHeight)
		out[m.h/2] = paint(m.w, "", sg(" "+msg, t.Warn))
		return strings.Join(out, "\n")
	}
	body := m.bodyHeight()
	lw, rw := m.leftWidth(), m.rightWidth()
	left := m.viewLeft(lw, body)
	right := m.viewRight(rw, body)
	div := lipgloss.NewStyle().Foreground(t.Line).Render("│")
	lines := make([]string, 0, m.h)
	lines = append(lines, m.viewHeader())
	bodyLines := make([]string, body)
	for i := 0; i < body; i++ {
		bodyLines[i] = left[i] + div + right[i]
	}
	if m.ov != nil {
		bodyLines = m.spliceOverlay(bodyLines)
	}
	lines = append(lines, bodyLines...)
	lines = append(lines, m.viewStatus(), m.viewKeys())
	return strings.Join(lines, "\n")
}

func (m Model) spliceOverlay(body []string) []string {
	w := min(m.w-4, 84)
	h := len(body) - 2
	ov := m.ov.render(w, h, m.theme, m.surf)
	top := max(0, (len(body)-len(ov))/2)
	x := (m.w - w) / 2
	for i, l := range ov {
		y := top + i
		if y >= len(body) {
			break
		}
		b := body[y]
		body[y] = ansi.Truncate(b, x, "") + l + ansi.Cut(b, x+w, m.w)
	}
	return body
}

func (m Model) viewHeader() string {
	t, s := m.theme, m.snap
	left := []seg{sg(" ", "")}
	left = append(left, gradient("◆ skilldots", t.Accent, t.Cyan, true)...)
	if m.version != "" {
		left = append(left, sg("  v"+strings.TrimPrefix(m.version, "v"), t.Muted))
	}
	right := []seg{sgb(s.Name, t.Text)}
	if s.Branch != "" {
		right = append(right, sg(" · ", t.Muted), sg(s.Branch, t.Cyan))
	}
	right = append(right, sg(" · ", t.Muted))
	right = append(right, m.syncState()...)
	right = append(right, sg(" ", ""))
	return spread(m.w, m.surf.bar, left, right)
}

// syncState summarises whether the machine and the remote match the
// library: links to apply, uncommitted work, unpushed commits.
func (m Model) syncState() []seg {
	t, s := m.theme, m.snap
	var parts [][]seg
	if s.Pending > 0 {
		parts = append(parts, []seg{sg("○ ", t.Warn), sg(fmt.Sprintf("%d to apply", s.Pending), t.Warn)})
	}
	if s.Dirty {
		parts = append(parts, []seg{sg("✎ uncommitted", t.Warn)})
	}
	switch {
	case !s.Remote:
		parts = append(parts, []seg{sg("no remote", t.Muted)})
	case !s.Tracked:
		parts = append(parts, []seg{sg("↑ not pushed", t.Warn)})
	case s.Ahead > 0:
		parts = append(parts, []seg{sg(fmt.Sprintf("↑%d unpushed", s.Ahead), t.Warn)})
	}
	if len(parts) == 0 || (len(parts) == 1 && !s.Remote && s.Pending == 0 && !s.Dirty) {
		out := []seg{sg("● ", t.OK), sg("synced", t.OK)}
		if !s.Remote {
			out = []seg{sg("● ", t.OK), sg("linked", t.OK), sg(" · no remote", t.Muted)}
		}
		return out
	}
	var out []seg
	for i, p := range parts {
		if i > 0 {
			out = append(out, sg(" · ", t.Muted))
		}
		out = append(out, p...)
	}
	return out
}

func (m Model) filterLine(w int, f focus) string {
	t := m.theme
	bg := lipgloss.Color("")
	if f == focusRight {
		bg = m.surf.panel
	}
	if m.filtering && m.focus == f {
		v := m.finput.View()
		vw := ansi.StringWidth(v)
		return paint(3, bg, sg(" / ", t.Accent)) + v + paint(w-3-vw, bg)
	}
	return paint(w, bg, sg(" / ", t.Accent), sg(m.filter[f], t.Text), sg("  esc clears", t.Faint))
}

func (m Model) viewLeft(w, h int) []string {
	t, s := m.theme, m.surf
	rows := m.leftRows()
	var out []string
	if m.filterShown(focusLeft) {
		out = append(out, m.filterLine(w, focusLeft))
	}
	lh := m.leftHeight()
	l := m.left
	l.clamp(len(rows), lh)
	for i := l.off; i < len(rows) && i < l.off+lh; i++ {
		r := rows[i]
		sel := i == m.left.cur && r.selectable()
		bg := lipgloss.Color("")
		mark := sg("   ", "")
		if sel {
			if m.focus == focusLeft {
				bg, mark = s.sel, sgb(" ❯ ", t.Accent)
			} else {
				bg, mark = s.selDim, sg(" › ", t.Muted)
			}
		}
		switch r.kind {
		case rowHeader:
			out = append(out, paint(w, "", sg("  ", ""), sgb(r.text, t.Muted), sg("  "+r.count, t.Faint)))
		case rowBlank:
			out = append(out, paint(w, ""))
		case rowNote:
			out = append(out, paint(w, "", sg("   "+r.text, t.Faint)))
		case rowPack:
			p := m.snap.Packs[r.pack]
			count := fmt.Sprintf("%d of %d", p.Selected, p.Total)
			if p.UsesAll {
				count = fmt.Sprintf("all %d", p.Selected)
			}
			nameW := w - 3 - ansi.StringWidth(count) - 2
			name := sg(p.Repo, t.Text)
			if ansi.StringWidth(p.Repo) > nameW {
				name.s = ansi.Truncate(p.Repo, nameW, "…")
			}
			if sel {
				name.bold = true
			}
			out = append(out, spread(w, bg, lift(t, []seg{mark, name}), []seg{sg(count, t.Cyan), sg(" ", "")}))
		case rowSkill:
			dot := sg("○ ", t.Faint)
			if r.skill.Enabled() {
				dot = sg("● ", t.OK)
			}
			name := sg(r.skill.LinkName, t.Text)
			if sel {
				name.bold = true
			}
			var right []seg
			switch {
			case r.skill.Conflict != "":
				dot, right = sg("! ", t.Warn), []seg{sg("conflict ", t.Warn)}
			case r.skill.Missing:
				right = []seg{sg("missing ", t.Warn)}
			}
			left := []seg{mark, dot, name}
			if sel {
				left, right = lift(t, left), lift(t, right)
			}
			out = append(out, spread(w, bg, left, right))
		}
	}
	for len(out) < h-1 {
		out = append(out, paint(w, ""))
	}
	scroll := ""
	if len(rows) > lh {
		scroll = ansiRange(l.off, lh, len(rows))
	}
	out = append(out, spread(w, "", nil, []seg{sg(scroll, t.Faint), sg(" ", "")}))
	return out[:h]
}

// packHeader is the block above a pack's skill list.
func (m Model) packHeader(p Pack, w int) []string {
	t, bg := m.theme, m.surf.panel
	row := func(segs ...seg) string { return paint(w, bg, append([]seg{sg(" ", "")}, segs...)...) }
	title := []seg{sgb(p.Repo, t.Text)}
	if p.Alias != "" {
		title = append(title, sg("  ("+p.Alias+")", t.Muted))
	}
	meta := []seg{}
	if p.Commit != "" {
		meta = append(meta, sg(short(p.Commit), t.Cyan))
	} else {
		meta = append(meta, sg("not fetched", t.Warn))
	}
	if p.When != "" {
		meta = append(meta, sg(" · "+p.When, t.Muted))
	}
	if p.NoLicense {
		meta = append(meta, sg(" · ", t.Muted), sg("no license", t.Warn))
	} else if p.License != "" {
		meta = append(meta, sg(" · "+p.License, t.Muted))
	}
	agents := "default (" + joinOr(m.snap.Defaults, "none") + ")"
	if p.Targets != nil {
		agents = joinOr(p.Targets, "none (kept in library)")
	}
	sel := fmt.Sprintf("%d of %d selected", p.Selected, p.Total)
	if p.UsesAll {
		sel = "follows everything upstream"
	}
	if !p.Upstream {
		sel += " · upstream list unknown until u"
	}
	lines := append(m.problem(w), row(), row(title...), row(meta...), row(sg("agents ", t.Faint), sg(agents, t.Muted), sg(" · "+sel, t.Faint)))
	if rep := m.reports[p.Repo]; rep != nil {
		lines = append(lines, row())
		head := []seg{sgb("update ", t.Text)}
		switch {
		case rep.OldCommit == "" || rep.OldCommit == rep.NewCommit:
			head = append(head, sg(short(rep.NewCommit)+" · up to date", t.Muted))
		default:
			head = append(head, sg(short(rep.OldCommit)+" → ", t.Muted), sg(short(rep.NewCommit), t.Accent))
		}
		lines = append(lines, row(head...))
		for _, r := range []struct {
			label string
			fg    lipgloss.Color
			names []string
		}{{"changed", t.Accent, rep.Changed}, {"added", t.OK, rep.Added}, {"upstream new", t.Cyan, rep.UpstreamNew},
			{"upstream gone", t.Warn, rep.UpstreamGone}, {"dropped", t.Err, rep.Dropped}} {
			if len(r.names) == 0 {
				if r.label == "changed" || r.label == "added" || r.label == "upstream new" {
					lines = append(lines, row(sg(fit(r.label, 14), r.fg), sg("—", t.Faint)))
				}
				continue
			}
			lines = append(lines, row(sg(fit(r.label, 14), r.fg), sg(strings.Join(r.names, ", "), t.Text)))
		}
	}
	lines = append(lines, row())
	return lines
}

// problem is a banner for what the engine skipped or could not link
// (name clashes, unknown targets, packs never fetched). It stays on screen
// until the library is fixed.
func (m Model) problem(w int) []string {
	ws := m.snap.Warnings
	if len(ws) == 0 {
		return nil
	}
	t, bg := m.theme, m.surf.panel
	text := tildifyAll(ws[0])
	if len(ws) > 1 {
		text = fmt.Sprintf("%d problems (skd apply -n lists them) · %s", len(ws), text)
	}
	var out []string
	for i, l := range wrap(text+" · r reloads after a fix", w-4, 3) {
		mark := "   "
		if i == 0 {
			mark = " ! "
		}
		out = append(out, paint(w, bg, sg(mark, t.Warn), sg(l, t.Warn)))
	}
	return out
}

func (m Model) viewRight(w, h int) []string {
	t, bg := m.theme, m.surf.panel
	row := func(segs ...seg) string { return paint(w, bg, append([]seg{sg(" ", "")}, segs...)...) }
	var out []string
	r := m.currentLeft()
	if r == nil || r.kind != rowPack {
		out = m.problem(w)
	}
	switch {
	case r == nil && len(m.snap.Packs) == 0 && len(m.snap.Mine)+len(m.snap.Private)+len(m.snap.Apps) == 0:
		out = append(out, row(), row(sgb("Your library is empty", t.Text)), row(),
			row(sg("Press ", t.Muted), sgb("a", t.Accent), sg(" to subscribe to a repository of skills,", t.Muted)),
			row(sg("for example mattpocock/skills or anthropics/skills.", t.Muted)), row(),
			row(sg("Skills you write go in "+tildify(m.snap.Root)+"/skills/.", t.Faint)))
	case r == nil:
		out = append(out, row(), row(sg("nothing selected", t.Muted)))
	case r.kind == rowSkill:
		out = append(out, m.skillDetail(r.skill, w)...)
	default:
		out = m.packView(m.snap.Packs[r.pack], w, h)
	}
	for len(out) < h {
		out = append(out, row())
	}
	return out[:h]
}

func (m Model) packView(p Pack, w, h int) []string {
	t, s := m.theme, m.surf
	bg := s.panel
	row := func(segs ...seg) string { return paint(w, bg, append([]seg{sg(" ", "")}, segs...)...) }
	out := m.packHeader(p, w)
	if m.filterShown(focusRight) {
		out = append(out, m.filterLine(w, focusRight))
	}
	rows := m.rightRows()
	nameW, catW := 12, 8
	for _, sk := range rows {
		nameW = max(nameW, ansi.StringWidth(sk.LinkName))
		catW = max(catW, ansi.StringWidth(sk.Category))
	}
	nameW = min(nameW, max(14, (w-8)*2/5))
	catW = min(catW, max(8, (w-8)/4))
	out = append(out, row(sg("    "+fit("SKILL", nameW)+"  "+fit("CATEGORY", catW)+"  AGENTS", t.Faint)))
	lh := m.rightListHeight()
	l := m.right
	l.clamp(len(rows), lh)
	if len(rows) == 0 {
		msg := "no skills vendored yet · u fetches upstream"
		if m.filter[focusRight] != "" {
			msg = "no match"
		}
		out = append(out, row(sg("   "+msg, t.Faint)))
	}
	for i := l.off; i < len(rows) && i < l.off+lh; i++ {
		sk := rows[i]
		rbg := bg
		mark := sg("  ", "")
		if i == m.right.cur {
			if m.focus == focusRight {
				rbg, mark = s.sel, sgb("❯ ", t.Accent)
			} else {
				rbg = s.selDim
			}
		}
		icon := sg("○ ", t.Faint)
		nameFg := t.Muted
		var agents seg
		switch {
		case !sk.Installed:
			agents = sg("not installed", t.Faint)
		case sk.Conflict != "":
			icon, nameFg, agents = sg("! ", t.Warn), t.Text, sg("name conflict · not linked", t.Warn)
		case sk.Missing:
			icon, nameFg, agents = sg("◉ ", t.Warn), t.Text, sg("missing from vendor/ · u re-fetches", t.Warn)
		case sk.Enabled():
			icon, nameFg, agents = sg("◉ ", t.OK), t.Text, sg(strings.Join(sk.Agents, " "), t.Cyan)
		default:
			icon, nameFg, agents = sg("◉ ", t.Muted), t.Text, sg("off · in library", t.Muted)
		}
		cat := sk.Category
		if cat == "" {
			cat = "·"
		}
		name := sg(fit(sk.LinkName, nameW)+"  ", nameFg)
		name.bold = i == m.right.cur && m.focus == focusRight
		segs := []seg{sg(" ", ""), mark, icon, name, sg(fit(cat, catW)+"  ", t.Muted), agents}
		if rbg != bg {
			segs = lift(t, segs)
		}
		out = append(out, paint(w, rbg, segs...))
	}
	// Pad the list area, then scroll position and the description of the
	// skill under the cursor.
	want := h - 1 - m.descLines()
	for len(out) < want {
		out = append(out, row())
	}
	out = out[:min(len(out), want)]
	scroll := ""
	if len(rows) > lh {
		scroll = ansiRange(l.off, lh, len(rows)) + " · / filter"
	}
	out = append(out, spread(w, bg, nil, []seg{sg(scroll, t.Faint), sg(" ", "")}))
	desc := ""
	if m.right.cur < len(rows) && len(rows) > 0 {
		sk := rows[m.right.cur]
		desc = sk.Description
		if sk.LinkName != sk.Name {
			desc = "linked as " + sk.LinkName + " (upstream " + sk.Name + ") · " + desc
		}
	}
	d := wrap(desc, w-2, m.descLines())
	for len(d) < m.descLines() {
		d = append(d, "")
	}
	for _, l := range d {
		out = append(out, row(sg(l, t.Muted)))
	}
	return out
}

func (m Model) skillDetail(sk Skill, w int) []string {
	t, bg := m.theme, m.surf.panel
	row := func(segs ...seg) string { return paint(w, bg, append([]seg{sg(" ", "")}, segs...)...) }
	kind := map[engine.Kind]string{
		engine.KindSkill:   "your skill · skills/, published with the library",
		engine.KindPrivate: "private · this machine only, never pushed",
		engine.KindApp:     "app · shipped with " + sk.Origin,
	}[sk.Kind]
	agents := []seg{sg("agents ", t.Faint)}
	if sk.Enabled() {
		agents = append(agents, sg(strings.Join(sk.Agents, " "), t.Cyan))
	} else {
		agents = append(agents, sg("off · kept in library", t.Muted))
	}
	out := []string{row(), row(sgb(sk.LinkName, t.Text)), row(sg(kind, t.Muted)), row(sg(tildify(sk.Dir), t.Faint)), row(agents...)}
	if sk.Missing {
		out = append(out, row(sg("source directory is missing", t.Warn)))
	}
	if sk.Conflict != "" {
		for _, l := range wrap("not linked: "+tildifyAll(sk.Conflict), w-2, 3) {
			out = append(out, row(sg(l, t.Warn)))
		}
	}
	out = append(out, row())
	for _, l := range wrap(sk.Description, w-2, 8) {
		out = append(out, row(sg(l, t.Text)))
	}
	out = append(out, row())
	hints := "space toggle · e agents · d unlink"
	if sk.Kind == engine.KindApp {
		hints += " · f fork"
	}
	if sk.Kind != engine.KindApp {
		hints += " · edit in " + tildify(sk.Dir)
	}
	out = append(out, row(sg(hints, t.Faint)))
	return out
}

// wrap word-wraps plain text to w cells, at most n lines.
func wrap(s string, w, n int) []string {
	s = strings.Join(strings.Fields(clean(s)), " ")
	if s == "" || w < 4 || n < 1 {
		return nil
	}
	var out []string
	for _, l := range strings.Split(lipgloss.NewStyle().Width(w).Render(s), "\n") {
		out = append(out, strings.TrimRight(l, " "))
	}
	if len(out) > n {
		out = out[:n]
		out[n-1] = ansi.Truncate(out[n-1]+" …", w, "…")
	}
	return out
}

func (m Model) viewStatus() string {
	t := m.theme
	if m.busy != "" {
		note := m.busyNote
		if m.quitAfter {
			note = "quitting when it finishes · ctrl+c again to stop it now"
		}
		segs := []seg{sg(" "+m.busy+"…", t.Accent)}
		if note != "" {
			segs = append(segs, sg(" · "+note, t.Warn))
		}
		return paint(1, "") + m.spin.View() + paint(m.w-1-ansi.StringWidth(m.spin.View()), "", segs...)
	}
	icon := map[level]seg{lvInfo: sg("· ", t.Muted), lvOK: sg("✓ ", t.OK), lvWarn: sg("! ", t.Warn), lvErr: sg("✗ ", t.Err)}[m.statusLvl]
	fg := t.Muted
	if m.statusLvl == lvErr {
		fg = t.Err
	}
	return paint(m.w, "", sg(" ", ""), icon, sg(m.status, fg))
}

func (m Model) viewKeys() string {
	t := m.theme
	keys := [][2]string{{"a", "add"}, {"u", "update"}, {"space", "toggle"}, {"e", "agents"}, {"f", "fork"}, {"d", "remove"},
		{"n", "new"}, {"S", "scan"}, {"t", "trash"}, {"s", "sync"}, {"/", "filter"}, {"?", "help"}, {"q", "quit"}}
	if m.filtering {
		keys = [][2]string{{"enter", "keep filter"}, {"esc", "clear"}, {"↑↓", "move"}}
	}
	// Drop the least important keys until the bar fits; help and quit
	// always stay.
	drop := []string{"/", "S", "t", "n", "s", "f", "e", "u", "d"}
	for {
		segs := []seg{sg(" ", "")}
		for _, k := range keys {
			segs = append(segs, sgb(k[0], t.Accent), sg(" "+k[1]+"  ", t.Muted))
		}
		if segWidth(segs) <= m.w || len(drop) == 0 || len(keys) <= 3 {
			return paint(m.w, m.surf.bar, segs...)
		}
		var kept [][2]string
		for _, k := range keys {
			if k[0] != drop[0] {
				kept = append(kept, k)
			}
		}
		keys, drop = kept, drop[1:]
	}
}
