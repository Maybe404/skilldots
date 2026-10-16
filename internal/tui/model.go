package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
)

type focus int

const (
	focusLeft focus = iota
	focusRight
)

type rowKind int

const (
	rowHeader rowKind = iota
	rowBlank
	rowNote
	rowPack
	rowSkill
)

// leftRow is one line of the left column.
type leftRow struct {
	kind  rowKind
	text  string // header title or note
	count string // header suffix
	pack  int    // index into Snapshot.Packs for rowPack
	skill Skill  // for rowSkill
}

func (r leftRow) selectable() bool { return r.kind == rowPack || r.kind == rowSkill }

func (r leftRow) key(s *Snapshot) string {
	switch r.kind {
	case rowPack:
		return "pack:" + s.Packs[r.pack].Repo
	case rowSkill:
		return r.skill.Key()
	}
	return ""
}

// list is a cursor plus scroll offset.
type list struct{ cur, off int }

// clamp keeps the cursor on screen for a window of h rows.
func (l *list) clamp(n, h int) {
	if l.cur >= n {
		l.cur = n - 1
	}
	if l.cur < 0 {
		l.cur = 0
	}
	if h < 1 {
		h = 1
	}
	if l.cur < l.off {
		l.off = l.cur
	}
	if l.cur >= l.off+h {
		l.off = l.cur - h + 1
	}
	if l.off > n-h {
		l.off = n - h
	}
	if l.off < 0 {
		l.off = 0
	}
}

type level int

const (
	lvInfo level = iota
	lvOK
	lvWarn
	lvErr
)

// job is one operation run off the UI goroutine.
type job struct {
	label string
	run   func(Ops) result
}

type result struct {
	msg     string
	err     error
	reports []*engine.SyncReport
	browse  *Browse
	src     string // the add argument, for browse results
	snap    *Snapshot
	// scan and trash open their views when the job ends.
	scan     *ScanResult
	trash    []engine.TrashEntry
	settings []Setting
	// history is the update history (h); historyRead is set even when it
	// is empty.
	history     []app.UpdateRecord
	historyRead bool
	// plan is what updating would change (u, U), for planFor ("" all).
	plan    *app.UpdatePlan
	planFor string
	// foreground is a terminal editor to run with the screen handed over.
	foreground *exec.Cmd
	// focus is the Skill.Key of a row to put the cursor on afterwards.
	focus string
}

type doneMsg struct{ res result }

// checkState is where the background upstream check (skd outdated) is.
type checkState int

const (
	checkNone checkState = iota
	checkRunning
	checkDone
	checkFailed
)

// startCheckMsg starts the upstream check; Init sends it once.
type startCheckMsg struct{}

// checkMsg is a finished upstream check, with a snapshot read after it
// (the cache clones it fetched list upstream skills).
type checkMsg struct {
	out  *app.Outdated
	err  error
	snap *Snapshot
}

// Model is the dashboard.
type Model struct {
	// replay are key messages to handle before any other input.
	replay []tea.Msg

	ops     Ops
	theme   Theme
	surf    surfaces
	version string
	snap    *Snapshot

	w, h  int
	focus focus
	left  list
	right list

	filter    [2]string
	filtering bool
	finput    textinput.Model

	ov *overlay

	busy      string // label of the running job, "" when idle
	busyNote  string // why a key was refused while busy
	spin      spinner.Model
	status    string
	statusLvl level
	reports   map[string]*engine.SyncReport

	// The upstream check runs beside operations, never as one: it does
	// not make the dashboard busy and no key waits for it.
	check     checkState
	checkErr  string                    // why it failed, for checkFailed
	outdated  map[string]app.PackStatus // by repo, from the last check
	checkFail []app.Failure             // packs the last check could not fetch
	// checks lets Run stop a check still running when the dashboard quits.
	checks *checkControl
	// jobs counts operations still running, shared by every copy of the
	// model so Run can wait for them after the program stops.
	jobs *tracker

	// quitAfter is set by ctrl+c while a job runs: the dashboard quits
	// when it finishes instead of leaving git half-way.
	quitAfter bool
	// forceQuit is set by a second ctrl+c while a job runs: Run stops
	// the job's git and `skd sync` processes instead of waiting.
	forceQuit bool
	quitting  bool
}

// tracker counts running jobs. Once closed it refuses new ones, so a job
// whose command is dispatched after the program stopped never starts.
type tracker struct {
	mu     sync.Mutex
	n      int
	closed bool
	idle   chan struct{} // closed when n drops to 0 after close
}

func newTracker() *tracker { return &tracker{} }

func (t *tracker) begin() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return false
	}
	t.n++
	return true
}

func (t *tracker) end() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.n--
	if t.n == 0 && t.idle != nil {
		close(t.idle)
		t.idle = nil
	}
}

// close stops new jobs and returns a channel that is closed once the
// running ones end (already closed when none run).
func (t *tracker) close() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	ch := make(chan struct{})
	if t.n == 0 {
		close(ch)
	} else {
		t.idle = ch
	}
	return ch
}

// checkControl is shared by every copy of the model: the cancel function
// of the running upstream check, and a count of checks still running.
type checkControl struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// begin returns the context for a new check.
func (c *checkControl) begin() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()
	c.wg.Add(1)
	return ctx
}

// stop cancels a running check and waits up to timeout for its git to
// end, so quitting leaves no git behind and is never held up for long.
// It reports whether the check ended in time.
func (c *checkControl) stop(timeout time.Duration) bool {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// New builds the dashboard around an initial snapshot.
func New(ops Ops, snap *Snapshot, theme Theme, version string) Model {
	fi := textinput.New()
	fi.Prompt = ""
	fi.CharLimit = 64
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	m := Model{ops: ops, theme: theme, surf: theme.surfaces(), version: version, snap: snap,
		finput: fi, spin: sp, reports: map[string]*engine.SyncReport{}, jobs: newTracker(), checks: &checkControl{}}
	m.spin.Style = m.spin.Style.Foreground(theme.Accent)
	m.left.cur = m.firstSelectable(m.leftRows())
	if len(snap.Warnings) > 0 {
		m.status, m.statusLvl = firstLine(snap.Warnings[0]), lvWarn
	} else {
		m.status = "↑↓ move · tab switch column · ? all keys"
	}
	return m
}

// firstLine flattens a message onto one status line, home paths as ~.
func firstLine(s string) string {
	s = tildifyAll(strings.TrimSpace(s))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " " + strings.Join(strings.Fields(s[i:]), " ")
	}
	return s
}

// Init replays the keys typed before the dashboard could read them (see
// Options.Input), in order, and starts checking upstream for updates.
func (m Model) Init() tea.Cmd {
	check := func() tea.Msg { return startCheckMsg{} }
	if len(m.replay) == 0 {
		return check
	}
	var cmds []tea.Cmd
	for _, msg := range m.replay {
		cmds = append(cmds, func() tea.Msg { return msg })
	}
	return tea.Batch(tea.Sequence(cmds...), check)
}

// ---- derived lists ------------------------------------------------------

func match(filter string, fields ...string) bool {
	if filter == "" {
		return true
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), filter) {
			return true
		}
	}
	return false
}

func (m Model) leftRows() []leftRow {
	s := m.snap
	f := strings.ToLower(m.filter[focusLeft])
	var rows []leftRow
	var packs []leftRow
	for i, p := range s.Packs {
		if match(f, p.Repo, p.Alias) {
			packs = append(packs, leftRow{kind: rowPack, pack: i})
		}
	}
	rows = append(rows, leftRow{kind: rowHeader, text: "PACKS", count: countOf(len(packs), len(s.Packs), f)})
	switch {
	case len(packs) > 0:
		rows = append(rows, packs...)
	case len(s.Packs) == 0:
		rows = append(rows, leftRow{kind: rowNote, text: "none yet · a adds one"})
	default:
		rows = append(rows, leftRow{kind: rowNote, text: "no match"})
	}
	for _, g := range []struct {
		title  string
		skills []Skill
	}{{"MINE", s.Mine}, {"PRIVATE", s.Private}, {"APPS", s.Apps}} {
		var items []leftRow
		for _, sk := range g.skills {
			if match(f, sk.LinkName, sk.Description) {
				items = append(items, leftRow{kind: rowSkill, skill: sk})
			}
		}
		if len(items) == 0 {
			continue
		}
		rows = append(rows, leftRow{kind: rowBlank}, leftRow{kind: rowHeader, text: g.title, count: countOf(len(items), len(g.skills), f)})
		rows = append(rows, items...)
	}
	return rows
}

func countOf(shown, total int, filter string) string {
	if filter != "" && shown != total {
		return fmt.Sprintf("%d/%d", shown, total)
	}
	return fmt.Sprint(total)
}

func (m Model) firstSelectable(rows []leftRow) int {
	for i, r := range rows {
		if r.selectable() {
			return i
		}
	}
	return 0
}

// currentLeft returns the selected left row, or nil.
func (m Model) currentLeft() *leftRow {
	rows := m.leftRows()
	if m.left.cur < 0 || m.left.cur >= len(rows) || !rows[m.left.cur].selectable() {
		return nil
	}
	r := rows[m.left.cur]
	return &r
}

// currentPack is the pack selected on the left, or nil.
func (m Model) currentPack() *Pack {
	if r := m.currentLeft(); r != nil && r.kind == rowPack {
		p := m.snap.Packs[r.pack]
		return &p
	}
	return nil
}

func (m Model) rightRows() []Skill {
	p := m.currentPack()
	if p == nil {
		return nil
	}
	f := strings.ToLower(m.filter[focusRight])
	var out []Skill
	for _, s := range p.Skills {
		if match(f, s.LinkName, s.Name, s.Category, s.Description) {
			out = append(out, s)
		}
	}
	return out
}

// target is what an action applies to: the skill under the cursor (right
// column, or a skill row on the left) and the pack in view.
func (m Model) target() (*Skill, *Pack) {
	r := m.currentLeft()
	if r == nil {
		return nil, nil
	}
	if r.kind == rowSkill {
		s := r.skill
		return &s, nil
	}
	p := m.currentPack()
	if m.focus == focusRight {
		rows := m.rightRows()
		if m.right.cur >= 0 && m.right.cur < len(rows) {
			s := rows[m.right.cur]
			return &s, p
		}
	}
	return nil, p
}

// ---- update -------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.clampAll()
		return m, nil
	case spinner.TickMsg:
		if m.busy == "" && m.check != checkRunning {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case doneMsg:
		return m.finish(msg.res)
	case editedMsg:
		if msg.err != nil {
			m.status, m.statusLvl = "editor: "+firstLine(msg.err.Error()), lvErr
		}
		return m, nil
	case startCheckMsg:
		return m.startCheck()
	case checkMsg:
		return m.finishCheck(msg), nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if m.busy != "" && m.quitAfter {
				// Asked twice: Run stops the job's processes and exits.
				m.forceQuit, m.quitting = true, true
				return m, tea.Quit
			}
			if m.busy != "" {
				// Quitting now would leave git or `skd sync` running
				// on their own, possibly half-way through a write.
				m.quitAfter = true
				m.busyNote = "quitting when it finishes"
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit
		}
		if m.ov != nil {
			return m.updateOverlay(msg)
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateKey(msg)
	}
	return m, nil
}

func (m *Model) clampAll() {
	rows := m.leftRows()
	if m.left.cur >= len(rows) {
		m.left.cur = len(rows) - 1
	}
	if m.left.cur < 0 || (m.left.cur < len(rows) && !rows[m.left.cur].selectable()) {
		m.left.cur = m.nextSelectable(rows, m.left.cur, 1)
	}
	m.left.clamp(len(rows), m.leftHeight())
	m.right.clamp(len(m.rightRows()), m.rightListHeight())
	if m.focus == focusRight && m.currentPack() == nil {
		m.focus = focusLeft
	}
}

// nextSelectable finds the nearest selectable row from i in direction d,
// falling back to the other direction, then to i.
func (m Model) nextSelectable(rows []leftRow, i, d int) int {
	for j := i; j >= 0 && j < len(rows); j += d {
		if rows[j].selectable() {
			return j
		}
	}
	for j := i; j >= 0 && j < len(rows); j -= d {
		if rows[j].selectable() {
			return j
		}
	}
	if i < 0 {
		return 0
	}
	return i
}

func (m Model) move(d int) Model {
	if m.focus == focusRight {
		n := len(m.rightRows())
		m.right.cur += d
		m.right.clamp(n, m.rightListHeight())
		return m
	}
	rows := m.leftRows()
	i := m.left.cur + d
	if i < 0 {
		i = 0
	}
	if i >= len(rows) {
		i = len(rows) - 1
	}
	step := 1
	if d < 0 {
		step = -1
	}
	if j := m.nextSelectable(rows, i, step); j != m.left.cur {
		m.left.cur = j
		m.right = list{}
		m.filter[focusRight] = ""
	}
	m.left.clamp(len(rows), m.leftHeight())
	return m
}

func (m Model) switchFocus(to focus) Model {
	if to == focusRight && len(m.rightRows()) == 0 && m.filter[focusRight] == "" {
		return m
	}
	m.focus = to
	m.clampAll()
	return m
}

func (m Model) updateKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q":
		return m.quit()
	case "esc":
		if m.filter[m.focus] != "" {
			return m.clearFilter(m.focus), nil
		}
		return m.quit()
	case "up", "k":
		return m.move(-1), nil
	case "down", "j":
		return m.move(1), nil
	case "pgup", "ctrl+u":
		return m.move(-m.pageSize()), nil
	case "pgdown", "ctrl+d":
		return m.move(m.pageSize()), nil
	case "home", "g":
		return m.move(-1 << 20), nil
	case "end", "G":
		return m.move(1 << 20), nil
	case "tab", "shift+tab":
		if m.focus == focusLeft {
			return m.switchFocus(focusRight), nil
		}
		return m.switchFocus(focusLeft), nil
	case "right", "l", "enter":
		return m.switchFocus(focusRight), nil
	case "left":
		return m.switchFocus(focusLeft), nil
	case "h":
		return m.start(job{label: "reading the update history", run: func(o Ops) result {
			recs, err := o.History("")
			return result{history: recs, err: err, historyRead: true}
		}})
	case "/":
		m.filtering = true
		m.finput.SetValue(m.filter[m.focus])
		m.finput.CursorEnd()
		m.finput.Focus()
		return m, textinput.Blink
	case "?":
		m.ov = helpOverlay()
		return m, nil
	case "r":
		return m.start(job{label: "reloading", run: func(o Ops) result { return result{msg: "reloaded"} }})
	case ",":
		return m.start(job{label: "reading settings", run: func(o Ops) result {
			st, err := o.Settings()
			return result{settings: st, err: err}
		}})
	case "c":
		if m.check == checkRunning {
			return m.hint("already checking upstream")
		}
		return m.startCheck()
	}
	if m.busy != "" && len(k.String()) == 1 && strings.Contains(" aAudesfntSoh,", k.String()) {
		m.busyNote = "wait until it finishes"
		return m, nil
	}
	switch k.String() {
	case " ":
		return m.toggle()
	case "a":
		return m.addFlow()
	case "n":
		return m.newFlow()
	case "t":
		return m.start(job{label: "reading the trash", run: func(o Ops) result {
			items, err := o.TrashList()
			return result{trash: items, err: err}
		}})
	case "S":
		return m.start(job{label: "scanning agent directories", run: func(o Ops) result {
			r, err := o.Scan()
			return result{scan: r, err: err}
		}})
	case "u":
		if p := m.currentPack(); p != nil {
			return m.planUpdate(p.Repo)
		}
		return m.planUpdate("")
	case "U":
		return m.planUpdate("")
	case "e":
		return m.agents()
	case "o":
		return m.edit()
	case "f":
		return m.fork()
	case "d":
		return m.trash()
	case "s":
		m.ov = confirmOverlay("Back up the library", []string{
			"Commit everything in " + m.snap.Name + " and push it to its git remote (skd sync).",
			"private/ is a separate repository and is never pushed.",
		}, jobChoice("y", "commit and push", job{label: "backing up", run: func(o Ops) result {
			msg, err := o.Sync()
			return result{msg: msg, err: err}
		}}))
		return m, nil
	case "A":
		return m.start(job{label: "applying", run: func(o Ops) result {
			msg, err := o.Apply()
			return result{msg: msg, err: err}
		}})
	}
	return m, nil
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.busy != "" {
		m.busyNote = "q again when it finishes · ctrl+c quits once it is done"
		return m, nil
	}
	m.quitting = true
	return m, tea.Quit
}

func (m Model) pageSize() int {
	if m.focus == focusRight {
		return max(1, m.rightListHeight()-1)
	}
	return max(1, m.leftHeight()-1)
}

// planUpdate is u (repo set: the pack under the cursor) and U (every
// pack): check upstream first, then show what would change and ask.
// Nothing is updated until the person confirms (see planView).
func (m Model) planUpdate(repo string) (tea.Model, tea.Cmd) {
	if len(m.snap.Packs) == 0 {
		return m.hint("no packs to update · a to add one")
	}
	var repos []string
	label := "checking upstream"
	if repo != "" {
		repos, label = []string{repo}, "checking "+repo
	}
	return m.start(job{label: label, run: func(o Ops) result {
		plan, err := o.PlanUpdate(repos)
		return result{plan: plan, err: err, planFor: repo}
	}})
}

// packsWithUpdates are the packs the last check found changes in, in the
// dashboard's order.
func (m Model) packsWithUpdates() []string {
	var out []string
	for _, p := range m.snap.Packs {
		if m.hasUpdates(p.Repo) {
			out = append(out, p.Repo)
		}
	}
	return out
}

func (m Model) hasUpdates(repo string) bool {
	st, ok := m.outdated[repo]
	return ok && st.HasUpdates()
}

// startCheck runs `skd outdated` in the background.
func (m Model) startCheck() (tea.Model, tea.Cmd) {
	if m.check == checkRunning {
		return m, nil
	}
	if len(m.snap.Packs) == 0 {
		m.check, m.outdated, m.checkFail = checkDone, map[string]app.PackStatus{}, nil
		return m, nil
	}
	m.check = checkRunning
	ops, checks := m.ops, m.checks
	ctx := checks.begin()
	run := func() tea.Msg {
		defer checks.wg.Done()
		out, err := ops.Check(ctx)
		if ctx.Err() != nil {
			return nil // the dashboard quit
		}
		var snap *Snapshot
		if err == nil {
			snap, _ = ops.Load()
		}
		return checkMsg{out: out, err: err, snap: snap}
	}
	return m, tea.Batch(m.spin.Tick, run)
}

// finishCheck takes in a finished upstream check. Problems go on the
// status line; the dashboard works as before either way.
func (m Model) finishCheck(c checkMsg) Model {
	m.check, m.checkErr, m.checkFail = checkDone, "", nil
	if c.snap != nil && m.busy == "" {
		m = m.replaceSnapshot(c.snap)
	}
	switch {
	case c.err != nil:
		m.check, m.checkErr = checkFailed, firstLine(c.err.Error())
	case c.out == nil:
	case len(c.out.Packs) == 0 && len(c.out.Failed) > 0:
		f := c.out.Failed[0]
		m.check, m.checkErr, m.checkFail = checkFailed, firstLine(f.Repo+": "+f.Error), c.out.Failed
	default:
		m = m.takeCheck(c.out, true)
	}
	switch {
	case m.check == checkFailed:
		m.status, m.statusLvl = "upstream check failed: "+m.checkErr+" · c checks again", lvWarn
	case len(m.checkFail) > 0:
		f := m.checkFail[0]
		m.status, m.statusLvl = fmt.Sprintf("could not check %s: %s · c checks again", f.Repo, firstLine(f.Error)), lvWarn
	}
	return m
}

// takeCheck takes in what an upstream check found: the background check
// (all packs, replacing what was known) or the check before an update
// (merged in).
func (m Model) takeCheck(o *app.Outdated, all bool) Model {
	if all || m.outdated == nil {
		m.outdated = map[string]app.PackStatus{}
		m.checkFail = nil
	}
	if all {
		m.check = checkDone
	}
	failed := map[string]bool{}
	for _, f := range o.Failed {
		failed[f.Repo] = true
	}
	var keep []app.Failure
	for _, f := range m.checkFail {
		if !failed[f.Repo] {
			keep = append(keep, f)
		}
	}
	m.checkFail = append(keep, o.Failed...)
	commits := map[string]string{}
	for _, p := range m.snap.Packs {
		commits[p.Repo] = p.Commit
	}
	for _, st := range o.Packs {
		// An update that ended while the check ran already moved the
		// pack on: what the check saw is out of date.
		if commit, ok := commits[st.Repo]; ok && commit == st.Commit {
			m.outdated[st.Repo] = st
		}
		m.checkFail = slices.DeleteFunc(m.checkFail, func(f app.Failure) bool { return f.Repo == st.Repo })
	}
	return m
}

func (m Model) hint(s string) (tea.Model, tea.Cmd) {
	m.status, m.statusLvl = s, lvInfo
	return m, nil
}

func (m Model) toggle() (tea.Model, tea.Cmd) {
	s, p := m.target()
	if s == nil {
		if p != nil && len(p.Skills) > 0 {
			return m.hint("→ to pick a skill in " + p.Repo + ", then space")
		}
		return m.hint("nothing to toggle here")
	}
	sk := *s
	if !sk.Installed {
		return m.start(job{label: "installing " + sk.Name, run: func(o Ops) result {
			msg, err := o.Add(sk.Origin, AddRequest{Names: []string{sk.Name}})
			return result{msg: msg, err: err}
		}})
	}
	if sk.Enabled() {
		return m.start(job{label: "disabling " + sk.LinkName, run: func(o Ops) result {
			msg, err := o.Disable(sk)
			return result{msg: msg, err: err}
		}})
	}
	if len(m.snap.Defaults) == 0 {
		return m.hint("no default agents in skills.toml · e to pick agents")
	}
	return m.start(job{label: "enabling " + sk.LinkName, run: func(o Ops) result {
		msg, err := o.Enable(sk)
		return result{msg: msg, err: err}
	}})
}

func (m Model) agents() (tea.Model, tea.Cmd) {
	s, p := m.target()
	var cur []string
	var title string
	var run func(Ops, []string) (string, error)
	switch {
	case s != nil && !s.Installed:
		return m.hint(s.Name + " is not installed · space installs it")
	case s != nil:
		sk := *s
		cur, title = sk.Agents, "Agents for "+sk.LinkName
		run = func(o Ops, a []string) (string, error) { return o.SetTargets(&sk, "", a) }
	case p != nil:
		repo := p.Repo
		cur = p.Targets
		if cur == nil {
			cur = m.snap.Defaults
		}
		title = "Agents for every skill in " + repo
		run = func(o Ops, a []string) (string, error) { return o.SetTargets(nil, repo, a) }
	default:
		return m.hint("select a skill or a pack first")
	}
	on := map[string]bool{}
	for _, a := range cur {
		on[a] = true
	}
	var items []pickItem
	seen := map[string]bool{}
	for _, t := range m.snap.Agents {
		items = append(items, pickItem{label: t.Name, detail: tildify(t.Path), value: t.Name, on: on[t.Name]})
		seen[t.Name] = true
	}
	for _, a := range cur {
		if !seen[a] {
			items = append(items, pickItem{label: a, detail: "not found on this machine", value: a, on: true})
		}
	}
	if len(items) == 0 {
		return m.hint("no agent skill directories found on this machine")
	}
	m.ov = pickOverlay(title, "only agents installed here are listed · none = keep in library, link nowhere", items, false,
		func(values []string, _ bool) step {
			chosen := append([]string{}, values...)
			return step{job: &job{label: "linking", run: func(o Ops) result {
				msg, err := run(o, chosen)
				return result{msg: msg, err: err}
			}}}
		})
	return m, nil
}

func (m Model) trash() (tea.Model, tea.Cmd) {
	s, p := m.target()
	switch {
	case s != nil && !s.Installed:
		return m.hint(s.Name + " is not installed; nothing to remove")
	case s != nil && s.Kind == engine.KindVendor:
		sk := *s
		m.ov = confirmOverlay("Remove "+sk.LinkName, []string{
			"Unlink it from every agent and drop it from vendor/.",
			"The files move to the library trash: t restores them.",
		}, jobChoice("y", "remove", job{label: "removing " + sk.LinkName, run: func(o Ops) result {
			msg, err := o.Trash(sk)
			return result{msg: msg, err: err}
		}}))
	case s != nil:
		sk := *s
		m.ov = confirmOverlay("Unlink "+sk.LinkName, []string{
			"Unlink it from every agent. Your files stay where they are:",
			tildify(sk.Dir),
		}, jobChoice("y", "unlink", job{label: "unlinking " + sk.LinkName, run: func(o Ops) result {
			msg, err := o.Trash(sk)
			return result{msg: msg, err: err}
		}}))
	case p != nil:
		repo := p.Repo
		links := 0
		for _, sk := range p.Skills {
			links += len(sk.Agents)
		}
		m.ov = confirmOverlay("Purge "+repo, []string{
			fmt.Sprintf("Unsubscribe and remove %d link(s) from agent directories.", links),
			"vendor/" + repo + " moves to the library trash (t restores it).",
		}, jobChoice("y", "purge pack", job{label: "purging " + repo, run: func(o Ops) result {
			msg, err := o.Purge(repo)
			return result{msg: msg, err: err}
		}}))
	default:
		return m.hint("select a skill or a pack first")
	}
	return m, nil
}

func (m Model) updateFilter(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.filtering = false
		m.finput.Blur()
		return m.clearFilter(m.focus), nil
	case "enter", "tab":
		m.filtering = false
		m.finput.Blur()
		return m, nil
	case "up", "down":
		m.filtering = false
		m.finput.Blur()
		return m.updateKey(k)
	}
	var cmd tea.Cmd
	m.finput, cmd = m.finput.Update(k)
	m.filter[m.focus] = m.finput.Value()
	if m.focus == focusLeft {
		rows := m.leftRows()
		m.left = list{cur: m.firstSelectable(rows)}
		m.right = list{}
	} else {
		m.right = list{}
	}
	m.clampAll()
	return m, cmd
}

// clearFilter drops the filter of column f and keeps the cursor on the
// row it was on.
func (m Model) clearFilter(f focus) Model {
	var leftKey, rightKey string
	if r := m.currentLeft(); r != nil {
		leftKey = r.key(m.snap)
	}
	if rows := m.rightRows(); m.right.cur >= 0 && m.right.cur < len(rows) {
		rightKey = rows[m.right.cur].Key()
	}
	m.filter[f] = ""
	if f == focusLeft {
		for i, r := range m.leftRows() {
			if r.selectable() && r.key(m.snap) == leftKey {
				m.left.cur = i
			}
		}
	} else {
		for i, s := range m.rightRows() {
			if s.Key() == rightKey {
				m.right.cur = i
			}
		}
	}
	m.clampAll()
	return m
}

// ---- jobs ---------------------------------------------------------------

func (m Model) start(j job) (tea.Model, tea.Cmd) {
	if m.busy != "" {
		m.busyNote = "wait until it finishes"
		return m, nil
	}
	m.busy, m.busyNote = j.label, ""
	ops, jobs := m.ops, m.jobs
	run := func() tea.Msg {
		if !jobs.begin() {
			return nil // the dashboard already stopped
		}
		defer jobs.end()
		res := j.run(ops)
		if res.browse == nil && res.scan == nil && res.trash == nil && res.settings == nil && !res.historyRead && res.plan == nil {
			snap, err := ops.Load()
			if err != nil && res.err == nil {
				res.err = err
			}
			res.snap = snap
		}
		return doneMsg{res}
	}
	return m, tea.Batch(m.spin.Tick, run)
}

func (m Model) finish(r result) (tea.Model, tea.Cmd) {
	label := m.busy
	m.busy, m.busyNote = "", ""
	if m.quitAfter {
		// ctrl+c was pressed while the job ran; Run prints the outcome.
		m = m.setResult(label, r)
		m.quitting = true
		return m, tea.Quit
	}
	var w warning
	switch {
	case r.historyRead && (r.err == nil || errors.As(r.err, &w)):
		if r.err != nil {
			m.status, m.statusLvl = string(w), lvWarn
		}
		return m.historyView(r.history)
	case r.err != nil:
	case r.browse != nil:
		m.ov = m.browseOverlay(r.src, r.browse)
		m.status, m.statusLvl = fmt.Sprintf("%s · %d skills · pick and press enter", r.browse.Repo, len(r.browse.Skills)), lvInfo
		if n := len(r.browse.Preselect); n > 0 {
			m.status = fmt.Sprintf("%s · %d skills · %d ticked from what you pasted · enter to continue", r.browse.Repo, len(r.browse.Skills), n)
		}
		if len(r.browse.Missing) > 0 {
			m.status, m.statusLvl = fmt.Sprintf("%s has nothing named %s · pick and press enter", r.browse.Repo, strings.Join(r.browse.Missing, ", ")), lvWarn
		}
		return m, nil
	case r.scan != nil:
		return m.scanView(r.scan)
	case r.trash != nil || label == "reading the trash":
		return m.trashView(r.trash)
	case r.settings != nil:
		return m.settingsView(r.settings)
	case r.plan != nil:
		return m.planView(r.planFor, r.plan)
	}
	m = m.setResult(label, r)
	if r.foreground != nil && r.err == nil {
		return m, tea.ExecProcess(r.foreground, func(err error) tea.Msg { return editedMsg{err} })
	}
	return m, nil
}

// editedMsg is a terminal editor that exited.
type editedMsg struct{ err error }

// setResult takes in a finished job's reports, snapshot and status line.
func (m Model) setResult(label string, r result) Model {
	for _, rep := range r.reports {
		m.reports[rep.Repo] = rep
		if m.outdated != nil {
			// Updated: the pack matches upstream now.
			m.outdated[rep.Repo] = app.PackStatus{Repo: rep.Repo, Commit: rep.NewCommit, Latest: rep.NewCommit}
		}
	}
	if r.snap != nil {
		m = m.replaceSnapshot(r.snap)
	}
	if r.focus != "" && r.err == nil {
		m = m.focusKey(r.focus)
	}
	var w warning
	switch {
	case errors.As(r.err, &w):
		m.status, m.statusLvl = string(w), lvWarn
	case r.err != nil:
		m.status, m.statusLvl = firstLine(r.err.Error()), lvErr
		if r.msg != "" {
			m.status += " · " + r.msg
		}
	case r.msg != "":
		m.status, m.statusLvl = r.msg, lvOK
	default:
		m.status, m.statusLvl = label+" done", lvOK
	}
	return m
}

// replaceSnapshot swaps in fresh data and keeps the cursor on the same
// pack or skill when it still exists.
func (m Model) replaceSnapshot(s *Snapshot) Model {
	var leftKey, rightKey string
	if r := m.currentLeft(); r != nil {
		leftKey = r.key(m.snap)
	}
	if rows := m.rightRows(); m.right.cur < len(rows) && m.right.cur >= 0 && len(rows) > 0 {
		rightKey = rows[m.right.cur].Key()
	}
	m.snap = s
	rows := m.leftRows()
	found := false
	for i, r := range rows {
		if r.selectable() && r.key(s) == leftKey {
			m.left.cur, found = i, true
			break
		}
	}
	if !found {
		m.left.cur = m.nextSelectable(rows, min(m.left.cur, len(rows)-1), -1)
	}
	if rightKey != "" {
		for i, sk := range m.rightRows() {
			if sk.Key() == rightKey {
				m.right.cur = i
			}
		}
	}
	m.clampAll()
	return m
}

func (m Model) updateOverlay(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	ov, j, cmd := m.ov.update(k)
	m.ov = ov
	if ov != nil && ov.err != "" {
		// Input the overlay refused: say why on the status line too.
		m.status, m.statusLvl = ov.err, lvErr
	}
	if j != nil {
		return m.start(*j)
	}
	return m, cmd
}

// focusKey puts the cursor on the left row of the skill with key, if any.
func (m Model) focusKey(key string) Model {
	m.filter[focusLeft] = ""
	for i, r := range m.leftRows() {
		if r.kind == rowSkill && r.skill.Key() == key {
			m.left.cur, m.focus = i, focusLeft
			m.right = list{}
			m.clampAll()
			break
		}
	}
	return m
}
