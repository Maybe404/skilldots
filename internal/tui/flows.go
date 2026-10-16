package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/scan"
)

// The multi-step dialogs: add, new, fork, scan/adopt and the trash. Each
// step is an overlay; the last one starts a job that runs the same
// internal/app flow as the CLI command named in its comment.

func (m Model) open(o *overlay) (tea.Model, tea.Cmd) {
	m.ov = o
	if o.kind == ovInput {
		return m, o.input.Focus()
	}
	return m, nil
}

// addFlow is `skd add`: repository → skills (by name or category, or
// everything) → alias → agents.
func (m Model) addFlow() (tea.Model, tea.Cmd) {
	return m.open(inputOverlay("Add a repository", "owner/repo, a GitHub URL or any git URL · @ref pins a branch or tag",
		"mattpocock/skills", "", m.theme, m.surf, func(src string) (step, error) {
			if src == "" {
				return step{}, nil
			}
			return step{job: &job{label: "fetching " + src, run: func(o Ops) result {
				b, err := o.Browse(src)
				return result{browse: b, err: err, src: src}
			}}}, nil
		}))
}

// browseOverlay offers the skills of a fetched repository, grouped by
// category; a category heading ticks all of its skills (skd add --category).
func (m Model) browseOverlay(src string, b *Browse) *overlay {
	cats := pack.Categories(b.Skills)
	grouped := len(cats) > 1 || (len(cats) == 1 && cats[0] != "")
	var items []pickItem
	for _, cat := range cats {
		var group []pickItem
		for _, s := range b.Skills {
			if s.Category != cat {
				continue
			}
			detail := s.Description
			if !grouped && s.Category != "" {
				detail = s.Category + " · " + detail
			}
			group = append(group, pickItem{label: s.Name, detail: detail, value: s.Name, on: b.Have[s.Name],
				locked: b.Have[s.Name], note: "installed", group: cat})
		}
		if grouped {
			name := cat
			if name == "" {
				name = "(no category)"
			}
			items = append(items, pickItem{label: name, detail: plural(len(group), "skill"), header: true, group: cat})
		}
		items = append(items, group...)
	}
	title := fmt.Sprintf("%s @ %s — %d skills", b.Repo, short(b.Commit), len(b.Skills))
	desc := "space toggles a skill, or a whole category on its heading · * follows everything"
	return pickOverlay(title, desc, items, true, func(values []string, all bool) step {
		var names []string
		for _, v := range values {
			if !b.Have[v] {
				names = append(names, v)
			}
		}
		if !all && len(names) == 0 && !b.Subscribed {
			return step{}
		}
		return step{next: m.aliasStep(src, b, AddRequest{Names: names, All: all})}
	})
}

func (m Model) aliasStep(src string, b *Browse, req AddRequest) *overlay {
	desc := "a short name for " + b.Repo + " in commands (alias:skill) · empty: none"
	if b.Alias != "" {
		desc = "a short name for " + b.Repo + " · empty keeps " + b.Alias
	}
	return inputOverlay("Alias", desc, "optional", b.Alias, m.theme, m.surf, func(v string) (step, error) {
		if v != "" && (strings.ContainsAny(v, `/\:`) || engine.ReservedQualifier(v)) {
			return step{}, fmt.Errorf("alias %q: no / \\ or : and not skills, private, app or vendor", v)
		}
		req.Alias = v
		return step{next: m.agentsStep(src, b, req)}, nil
	})
}

func (m Model) agentsStep(src string, b *Browse, req AddRequest) *overlay {
	on := map[string]bool{}
	for _, a := range b.Targets {
		on[a] = true
	}
	var items []pickItem
	for _, t := range m.snap.Agents {
		items = append(items, pickItem{label: t.Name, detail: tildify(t.Path), value: t.Name, on: on[t.Name]})
	}
	keep := "the default agents (" + joinOr(m.snap.Defaults, "none") + ")"
	if b.Targets != nil {
		keep = "the pack's agents (" + joinOr(b.Targets, "none") + ")"
	}
	return pickOverlay("Agents for "+b.Repo, "none ticked: "+keep, items, false, func(values []string, _ bool) step {
		r := req
		r.Agents = append([]string(nil), values...)
		return step{job: &job{label: "adding " + b.Repo, run: func(o Ops) result {
			msg, err := o.Add(src, r)
			return result{msg: msg, err: err}
		}}}
	})
}

// newFlow is `skd new <name> [--private]`.
func (m Model) newFlow() (tea.Model, tea.Cmd) {
	return m.open(inputOverlay("New skill", "letters, digits, . _ - · created with a SKILL.md template and linked to "+joinOr(m.snap.Defaults, "no agents"),
		"my-skill", "", m.theme, m.surf, func(name string) (step, error) {
			if name == "" {
				return step{}, errors.New("type a name")
			}
			if err := manifest.CheckName(name); err != nil {
				return step{}, err
			}
			mk := func(private bool) func() step {
				return func() step {
					return step{job: &job{label: "creating " + name, run: func(o Ops) result {
						msg, err := o.New(name, private)
						return result{msg: msg, err: err, focus: string(app.NewKind(private)) + "::" + name}
					}}}
				}
			}
			return step{next: confirmOverlay("Create "+name, []string{
				"skills/ is published with the library; private/ stays on this machine.",
			}, choice{"y", "in skills/", mk(false)}, choice{"p", "in private/", mk(true)})}, nil
		}))
}

// fork is `skd fork <skill> [--as name] [--private]`.
func (m Model) fork() (tea.Model, tea.Cmd) {
	s, _ := m.target()
	if s == nil {
		return m.hint("select a skill to fork")
	}
	sk := *s
	switch {
	case !sk.Installed:
		return m.hint(sk.Name + " is not installed · space installs it, then fork")
	case sk.Kind != engine.KindVendor && sk.Kind != engine.KindApp:
		return m.hint(sk.LinkName + " is already yours · edit it in " + tildify(sk.Dir))
	}
	return m.open(inputOverlay("Fork "+sk.LinkName, "name of your copy · enter keeps "+sk.LinkName,
		sk.LinkName, sk.LinkName, m.theme, m.surf, func(name string) (step, error) {
			if name == "" {
				name = sk.LinkName
			}
			if err := manifest.CheckName(name); err != nil {
				return step{}, err
			}
			as := name
			if as == sk.LinkName {
				as = ""
			}
			mk := func(private bool) func() step {
				return func() step {
					return step{job: &job{label: "forking " + sk.LinkName, run: func(o Ops) result {
						msg, err := o.Fork(sk, private, as)
						return result{msg: msg, err: err, focus: string(app.NewKind(private)) + "::" + name}
					}}}
				}
			}
			return step{next: confirmOverlay("Fork "+sk.LinkName+" as "+name, []string{
				"Copy it into your library so you can edit it.",
				"The upstream copy is deselected; updates no longer touch your fork.",
			}, choice{"y", "into skills/ (published)", mk(false)}, choice{"p", "into private/ (this machine)", mk(true)})}, nil
		}))
}

// scanStates describes scan states the way `skd scan` does.
var scanStates = map[scan.State]string{
	scan.Unlinked: "→ library", scan.AppLink: "app link", scan.ExtLink: "link", scan.Copy: "copy", scan.Broken: "broken",
}

// scanView shows `skd scan` by agent with what `skd adopt` proposes for
// each entry; the ticked ones are adopted.
func (m Model) scanView(r *ScanResult) (tea.Model, tea.Cmd) {
	counts := map[scan.State]int{}
	for _, it := range r.Report.Items {
		counts[it.State]++
	}
	summary := fmt.Sprintf("%d entries · %d managed · %d unmanaged · %d broken",
		len(r.Report.Items), counts[scan.Managed]+counts[scan.Unlinked], counts[scan.Copy]+counts[scan.ExtLink]+counts[scan.AppLink], counts[scan.Broken])
	if len(r.Proposals) == 0 {
		m.status, m.statusLvl = "nothing to adopt · "+summary, lvOK
		return m, nil
	}
	byAgent := map[string][]int{}
	var agents []string
	for i, p := range r.Proposals {
		if byAgent[p.Item.Target] == nil {
			agents = append(agents, p.Item.Target)
		}
		byAgent[p.Item.Target] = append(byAgent[p.Item.Target], i)
	}
	var items []pickItem
	for _, a := range agents {
		path := ""
		for _, t := range m.snap.Agents {
			if t.Name == a {
				path = tildify(t.Path)
			}
		}
		items = append(items, pickItem{label: a, detail: path, header: true, group: a})
		for _, i := range byAgent[a] {
			p := r.Proposals[i]
			state := scanStates[p.Item.State]
			if state == "" {
				state = string(p.Item.State)
			}
			it := pickItem{label: p.Item.Name, detail: fit(state, 10) + " " + tildifyAll(p.Label), value: strconv.Itoa(i),
				on: p.Actionable(), group: a}
			if !p.Actionable() {
				it.locked, it.note = true, fit(state, 10)+" "+tildifyAll(p.Label)
			}
			items = append(items, it)
		}
	}
	props := r.Proposals
	m.ov = pickOverlay("Scan · adopt", summary+" · ticked entries are taken over; displaced directories go to the trash",
		items, false, func(values []string, _ bool) step {
			var chosen []app.Proposal
			for _, v := range values {
				if i, err := strconv.Atoi(v); err == nil && i < len(props) {
					chosen = append(chosen, props[i])
				}
			}
			if len(chosen) == 0 {
				return step{}
			}
			return step{job: &job{label: fmt.Sprintf("adopting %d", len(chosen)), run: func(o Ops) result {
				msg, err := o.Adopt(chosen)
				return result{msg: msg, err: err}
			}}}
		})
	m.status, m.statusLvl = summary, lvInfo
	return m, nil
}

// trashView is `skd trash`: restore an entry, or empty the trash after
// two confirmations (`skd trash empty`).
func (m Model) trashView(entries []engine.TrashEntry) (tea.Model, tea.Cmd) {
	if len(entries) == 0 {
		m.status, m.statusLvl = "trash is empty", lvInfo
		return m, nil
	}
	var items []pickItem
	for _, e := range entries {
		when := e.When.Format("2006-01-02 15:04")
		if e.When.IsZero() {
			when = "?"
		}
		items = append(items, pickItem{label: e.Name, detail: when + "  from " + tildify(e.Origin), value: e.ID})
	}
	n := len(entries)
	restore := listAction{key: "r", label: "restore", run: func(it pickItem) step {
		id := it.value
		return step{job: &job{label: "restoring " + it.label, run: func(o Ops) result {
			msg, err := o.Restore(id)
			return result{msg: msg, err: err}
		}}}
	}}
	empty := listAction{key: "x", label: "empty trash", run: func(pickItem) step {
		final := confirmOverlay(fmt.Sprintf("Delete %d item(s) forever?", n), []string{
			"This cannot be undone. Nothing else in skd deletes anything.",
		}, jobChoice("D", "delete forever", job{label: "emptying the trash", run: func(o Ops) result {
			msg, err := o.EmptyTrash()
			return result{msg: msg, err: err}
		}}))
		final.noEnter = true
		return step{next: confirmOverlay("Empty the trash", []string{
			fmt.Sprintf("%s in %s will be deleted permanently.", plural(n, "item"), tildify(m.snap.Root)+"/.trash"),
		}, choice{"y", "continue", func() step { return step{next: final} }})}
	}}
	m.ov = listOverlay("Trash · "+plural(n, "item"), "what skd set aside, newest first · restoring puts it back where it came from",
		"trash is empty", items, restore, empty)
	return m, nil
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// settingsView is `skd config`: every setting, enter changes one.
func (m Model) settingsView(settings []Setting) (tea.Model, tea.Cmd) {
	var items []pickItem
	byKey := map[string]Setting{}
	for _, st := range settings {
		byKey[st.Name] = st
		v := st.Value
		if v == "" {
			v = "unset · " + st.Default
		}
		items = append(items, pickItem{label: st.Name, detail: tildifyAll(v), value: st.Name})
	}
	change := listAction{key: "c", label: "change", run: func(it pickItem) step {
		st := byKey[it.value]
		if st.ReadOnly != "" {
			return step{next: confirmOverlay(st.Name, []string{st.Help, "Change it with: " + st.ReadOnly})}
		}
		desc := st.Help + " · empty unsets it"
		if len(st.Values) > 0 {
			desc = st.Help + " · one of " + strings.Join(st.Values, ", ") + " · empty unsets it"
		}
		return step{next: inputOverlay("Set "+st.Name, desc, st.Default, st.Value, m.theme, m.surf, func(v string) (step, error) {
			if err := st.Check(v); err != nil {
				return step{}, err
			}
			key := st.Name
			return step{job: &job{label: "saving " + key, run: func(o Ops) result {
				msg, err := o.SetSetting(key, v)
				return result{msg: msg, err: err}
			}}}, nil
		})}
	}}
	m.ov = listOverlay("Settings", "per user and per machine (skd config) · the theme applies the next time skd starts", "no settings", items, change)
	return m, nil
}
