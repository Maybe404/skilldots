package tui

import (
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
	"github.com/Maybe404/skilldots/internal/scan"
	"github.com/Maybe404/skilldots/internal/targets"
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
	ov := inputOverlay(i18n.T("Add a repository"), i18n.T("owner/repo · a GitHub or skills.sh link to a skill · a pasted `npx skills add …` line · any git URL · @ref pins a branch or tag"),
		i18n.T("mattpocock/skills"), "", m.theme, m.surf, func(src string) (step, error) {
			if src == "" {
				return step{}, nil
			}
			if _, err := app.ParseAddText(src); err != nil {
				return step{}, err
			}
			return step{job: &job{label: i18n.Sprintf("fetching %s", src), run: func(o Ops) result {
				b, err := o.Browse(src)
				return result{browse: b, err: err, src: src}
			}}}, nil
		})
	// Room for a pasted install command.
	ov.input.CharLimit = 1000
	return m.open(ov)
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
			group = append(group, pickItem{label: s.Name, detail: detail, value: s.Name, on: b.Have[s.Name] || b.Preselect[s.Name],
				locked: b.Have[s.Name], note: i18n.T("installed"), group: cat})
		}
		if grouped {
			name := cat
			if name == "" {
				name = i18n.T("(no category)")
			}
			items = append(items, pickItem{label: name, detail: plural(len(group), "skill"), header: true, group: cat})
		}
		items = append(items, group...)
	}
	title := i18n.Sprintf("%s @ %s — %d skills", b.Repo, short(b.Commit), len(b.Skills))
	desc := i18n.T("space toggles a skill, or a whole category on its heading · * follows everything")
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
	desc := i18n.Sprintf("a short name for %s in commands (alias:skill) · empty: none", b.Repo)
	if b.Alias != "" {
		desc = i18n.Sprintf("a short name for %s · empty keeps %s", b.Repo, b.Alias)
	}
	return inputOverlay(i18n.T("Alias"), desc, i18n.T("optional"), b.Alias, m.theme, m.surf, func(v string) (step, error) {
		if v != "" && (strings.ContainsAny(v, `/\:`) || engine.ReservedQualifier(v)) {
			return step{}, i18n.Errorf("alias %q: no / \\ or : and not skills, private, app or vendor", v)
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
	keep := i18n.Sprintf("the default agents (%s)", joinOr(m.snap.Defaults, "none"))
	if b.Targets != nil {
		keep = i18n.Sprintf("the pack's agents (%s)", joinOr(b.Targets, "none"))
	}
	return pickOverlay(i18n.Sprintf("Agents for %s", b.Repo), i18n.Sprintf("none ticked: %s", keep), items, false, func(values []string, _ bool) step {
		r := req
		r.Agents = append([]string(nil), values...)
		return step{job: &job{label: i18n.Sprintf("adding %s", b.Repo), run: func(o Ops) result {
			msg, err := o.Add(src, r)
			return result{msg: msg, err: err}
		}}}
	})
}

// newFlow is `skd new <name> [--private]`.
func (m Model) newFlow() (tea.Model, tea.Cmd) {
	return m.open(inputOverlay(i18n.T("New skill"), i18n.Sprintf("letters, digits, . _ - · created with a SKILL.md template and linked to %s", joinOr(m.snap.Defaults, "no agents")),
		i18n.T("my-skill"), "", m.theme, m.surf, func(name string) (step, error) {
			if name == "" {
				return step{}, i18n.New("type a name")
			}
			if err := manifest.CheckName(name); err != nil {
				return step{}, err
			}
			mk := func(private bool) func() step {
				return func() step {
					return step{job: &job{label: i18n.Sprintf("creating %s", name), run: func(o Ops) result {
						msg, err := o.New(name, private)
						return result{msg: msg, err: err, focus: string(app.NewKind(private)) + "::" + name}
					}}}
				}
			}
			return step{next: confirmOverlay(i18n.Sprintf("Create %s", name), []string{
				i18n.T("skills/ is published with the library; private/ stays on this machine."),
			}, choice{"y", i18n.T("in skills/"), mk(false)}, choice{"p", i18n.T("in private/"), mk(true)})}, nil
		}))
}

// fork is `skd fork <skill> [--as name] [--private]`.
func (m Model) fork() (tea.Model, tea.Cmd) {
	s, _ := m.target()
	if s == nil {
		return m.hint(i18n.T("select a skill to fork"))
	}
	sk := *s
	switch {
	case !sk.Installed:
		return m.hint(i18n.Sprintf("%s is not installed · space installs it, then fork", sk.Name))
	case sk.Kind != engine.KindVendor && sk.Kind != engine.KindApp:
		return m.hint(i18n.Sprintf("%s is already yours · edit it in %s", sk.LinkName, tildify(sk.Dir)))
	}
	return m.open(inputOverlay(i18n.Sprintf("Fork %s", sk.LinkName), i18n.Sprintf("name of your copy · enter keeps %s", sk.LinkName),
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
					return step{job: &job{label: i18n.Sprintf("forking %s", sk.LinkName), run: func(o Ops) result {
						msg, err := o.Fork(sk, private, as)
						return result{msg: msg, err: err, focus: string(app.NewKind(private)) + "::" + name}
					}}}
				}
			}
			return step{next: confirmOverlay(i18n.Sprintf("Fork %s as %s", sk.LinkName, name), []string{
				i18n.T("Copy it into your library so you can edit it."),
				i18n.T("The upstream copy is deselected; updates no longer touch your fork."),
			}, choice{"y", i18n.T("into skills/ (published)"), mk(false)}, choice{"p", i18n.T("into private/ (this machine)"), mk(true)})}, nil
		}))
}

// scanStates describes scan states the way `skd scan` does.
var scanStates = map[scan.State]string{
	scan.Unlinked: i18n.M("→ library"), scan.AppLink: i18n.M("app link"), scan.ExtLink: i18n.M("link"), scan.Copy: i18n.M("copy"), scan.Broken: i18n.M("broken"),
	scan.Left: i18n.M("left by a target not in use"),
}

// scanView shows `skd scan` by agent with what `skd adopt` proposes for
// each entry; the ticked ones are adopted.
func (m Model) scanView(r *ScanResult) (tea.Model, tea.Cmd) {
	counts := map[scan.State]int{}
	for _, it := range r.Report.Items {
		counts[it.State]++
	}
	summary := i18n.Sprintf("%d entries · %d managed · %d unmanaged · %d broken",
		len(r.Report.Items), counts[scan.Managed]+counts[scan.Unlinked], counts[scan.Copy]+counts[scan.ExtLink]+counts[scan.AppLink], counts[scan.Broken])
	if len(r.Proposals) == 0 {
		m.status, m.statusLvl = i18n.Sprintf("nothing to adopt · %s", summary), lvOK
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
			state := i18n.T(scanStates[p.Item.State])
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
	m.ov = pickOverlay(i18n.T("Scan · adopt"), i18n.Sprintf("%s · ticked entries are taken over; displaced directories go to the trash", summary),
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
			return step{job: &job{label: i18n.Sprintf("adopting %d", len(chosen)), run: func(o Ops) result {
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
		m.status, m.statusLvl = i18n.T("trash is empty"), lvInfo
		return m, nil
	}
	var items []pickItem
	problems := map[string]string{}
	for _, e := range entries {
		when := e.When.Format("2006-01-02 15:04")
		if e.When.IsZero() {
			when = "?"
		}
		from := ""
		if e.Origin != "" {
			from = i18n.Sprintf("from %s", tildify(e.Origin))
		}
		detail := when + "  " + from
		if !e.Restorable {
			problems[e.ID] = e.Problem
			detail = when + i18n.T("  cannot be restored")
			if from != "" {
				detail += " · " + from
			}
		}
		items = append(items, pickItem{label: e.Name, detail: detail, value: e.ID})
	}
	n := len(entries)
	restore := listAction{key: "r", label: i18n.T("restore"), run: func(it pickItem) step {
		id := it.value
		// The same refusal as `skd trash restore`, said before any job.
		if p, bad := problems[id]; bad {
			lines := append(wrap(p, 64, 8), "", i18n.T("skd only puts back what it took from an agent's skill"),
				i18n.T("directory or the library's vendor/. Move it by hand if you trust it."))
			return step{next: textOverlay(i18n.Sprintf("Cannot restore %s", clean(it.label)), lines)}
		}
		return step{job: &job{label: i18n.Sprintf("restoring %s", clean(it.label)), run: func(o Ops) result {
			msg, err := o.Restore(id)
			return result{msg: msg, err: err}
		}}}
	}}
	empty := listAction{key: "x", label: i18n.T("empty trash"), run: func(pickItem) step {
		final := confirmOverlay(i18n.Sprintf("Delete %d item(s) forever?", n), []string{
			i18n.T("This cannot be undone. Nothing else in skd deletes anything."),
		}, jobChoice("D", i18n.T("delete forever"), job{label: i18n.T("emptying the trash"), run: func(o Ops) result {
			msg, err := o.EmptyTrash()
			return result{msg: msg, err: err}
		}}))
		final.noEnter = true
		return step{next: confirmOverlay(i18n.T("Empty the trash"), []string{
			i18n.Sprintf("%s in %s will be deleted permanently.", plural(n, "item"), tildify(m.snap.Root)+"/.trash"),
		}, choice{"y", i18n.T("continue"), func() step { return step{next: final} }})}
	}}
	m.ov = listOverlay(i18n.Sprintf("Trash · %s", plural(n, "item")), i18n.T("what skd set aside, newest first · restoring puts it back where it came from"),
		i18n.T("trash is empty"), items, restore, empty)
	return m, nil
}

// plurals are the nouns plural counts, as the formats i18n.Count takes.
var plurals = map[string][2]string{
	"skill":  {i18n.M("%d skill"), i18n.M("%d skills")},
	"item":   {i18n.M("%d item"), i18n.M("%d items")},
	"update": {i18n.M("%d update"), i18n.M("%d updates")},
	"pack":   {i18n.M("%d pack"), i18n.M("%d packs")},
}

func plural(n int, word string) string {
	if f, ok := plurals[word]; ok {
		return i18n.Count(n, f[0], f[1])
	}
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
			v = i18n.Sprintf("unset · %s", i18n.T(st.Default))
		}
		items = append(items, pickItem{label: st.Name, detail: tildifyAll(v), value: st.Name})
	}
	change := listAction{key: "c", label: i18n.T("change"), run: func(it pickItem) step {
		st := byKey[it.value]
		if st.ReadOnly != "" {
			return step{next: confirmOverlay(st.Name, []string{i18n.T(st.Help), i18n.Sprintf("Change it with: %s", st.ReadOnly)})}
		}
		desc := i18n.Sprintf("%s · empty unsets it", i18n.T(st.Help))
		if len(st.Values) > 0 {
			desc = i18n.Sprintf("%s · one of %s · empty unsets it", i18n.T(st.Help), strings.Join(st.Values, ", "))
		}
		return step{next: inputOverlay(i18n.Sprintf("Set %s", st.Name), desc, st.Default, st.Value, m.theme, m.surf, func(v string) (step, error) {
			if err := st.Check(v); err != nil {
				return step{}, err
			}
			key := st.Name
			return step{job: &job{label: i18n.Sprintf("saving %s", key), run: func(o Ops) result {
				msg, err := o.SetSetting(key, v)
				if err == nil && key == config.Lang {
					// Before the reload that follows every job: its texts
					// are made in the language just chosen.
					useLang(v)
					msg = settingMsg(key, v)
				}
				return result{msg: msg, err: err}
			}}}, nil
		})}
	}}
	m.ov = listOverlay(i18n.T("Settings"), i18n.T("per user and per machine (skd config) · the theme applies the next time skd starts"), i18n.T("no settings"), items, change)
	return m, nil
}

// targetsView is `skd targets`: the agent targets, and the custom ones
// of skills.toml skd does not use; a approves one (skd targets trust).
// The dashboard runs at a terminal, so whoever presses a is a person.
func (m Model) targetsView(rows []TargetRow) (tea.Model, tea.Cmd) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Active != rows[j].Active {
			return !rows[i].Active
		}
		return rows[i].Name < rows[j].Name
	})
	var items []pickItem
	byName := map[string]TargetRow{}
	for _, r := range rows {
		byName[r.Name] = r
		detail := tildify(r.Path)
		switch {
		case !r.Active && r.Approvable:
			detail = i18n.Sprintf("not approved · a approves · %s", tildify(r.Path))
		case !r.Active:
			detail = i18n.Sprintf("not used · %s", tildify(r.Raw))
		case !r.Builtin:
			detail = i18n.Sprintf("custom · %s", tildify(r.Path))
		}
		items = append(items, pickItem{label: r.Name, detail: detail, value: r.Name})
	}
	approve := listAction{key: "a", label: i18n.T("approve"), run: func(it pickItem) step {
		r := byName[it.value]
		switch {
		case r.Active && r.Builtin:
			return step{next: textOverlay(clean(r.Name), []string{i18n.T("A built-in target: always in use."), tildify(r.Path)})}
		case r.Active:
			return step{next: textOverlay(clean(r.Name), []string{i18n.T("Approved already: skd links skills into"), tildify(r.Path)})}
		case !r.Approvable:
			return step{next: textOverlay(i18n.Sprintf("Cannot approve %s", clean(r.Name)), wrap(r.Problem, 64, 8))}
		}
		verb := i18n.T("skd will link skills into this directory, put back trash entries")
		if r.Mode == "copy" {
			verb = i18n.T("skd will copy skills into this directory, put back trash entries")
		}
		row := r
		lines := []string{
			fit(i18n.T("skills.toml path"), 18) + r.Raw,
			fit(i18n.T("directory"), 18) + r.Path,
			fit(i18n.T("mode"), 18) + r.Mode,
			"",
			verb,
			i18n.T("that came from it, and with --force move what is in their way into"),
			i18n.T("the library trash. Approve only a directory you meant for skills."),
		}
		trust := func(typed string) *job {
			return &job{label: i18n.Sprintf("approving %s", clean(row.Name)), run: func(o Ops) result {
				msg, err := o.TrustTarget(row, typed)
				return result{msg: msg, err: err}
			}}
		}
		if !targets.Unusual(r.Path) {
			return step{next: confirmOverlay(i18n.Sprintf("Approve custom target %s", clean(r.Name)), lines, choice{key: "y", label: i18n.T("approve"), then: func() step {
				return step{job: trust("")}
			}})}
		}
		lines = append(lines, "", i18n.T("! This directory is not named skills. Whatever reads or runs files"),
			i18n.T("  from it (an editor, a shell, a service) would load what skd puts there."))
		typeName := func() step {
			return step{next: inputOverlay(i18n.Sprintf("Type %s to approve", clean(row.Name)), i18n.Sprintf("the directory %s is not named skills · esc cancels", tildify(row.Path)),
				"", "", m.theme, m.surf, func(v string) (step, error) {
					if v != row.Name {
						return step{}, i18n.Errorf("type %s exactly to approve it, or esc to cancel", clean(row.Name))
					}
					return step{job: trust(v)}, nil
				})}
		}
		ov := confirmOverlay(i18n.Sprintf("Approve custom target %s", clean(r.Name)), lines, choice{key: "y", label: i18n.T("type its name to approve"), then: typeName})
		ov.noEnter = true
		return step{next: ov}
	}}
	m.ov = listOverlay(i18n.T("Agent targets"), i18n.T("where skd links skills · a custom target of skills.toml is used once you approve it here (skd targets trust)"),
		i18n.T("no targets"), items, approve)
	return m, nil
}

// historyView is `skd history`: the recorded updates, newest first;
// enter shows one with upstream's commit messages.
func (m Model) historyView(recs []app.UpdateRecord) (tea.Model, tea.Cmd) {
	if len(recs) == 0 {
		if m.status == "" || m.statusLvl != lvWarn {
			m.status, m.statusLvl = i18n.T("no updates recorded yet · u and U record each one"), lvInfo
		}
		return m, nil
	}
	var items []pickItem
	for i, r := range recs {
		items = append(items, pickItem{label: r.Time.Local().Format("2006-01-02 15:04") + "  " + r.Repo,
			detail: short(r.From) + "→" + short(r.To) + " · " + app.RecordSummary(r), value: strconv.Itoa(i)})
	}
	show := listAction{key: "v", label: i18n.T("details"), run: func(it pickItem) step {
		i, _ := strconv.Atoi(it.value)
		return step{next: textOverlay(recs[i].Repo+" · "+recs[i].Time.Local().Format("2006-01-02 15:04"), recordLines(recs[i]))}
	}}
	m.ov = listOverlay(i18n.Sprintf("Update history · %s", plural(len(recs), "update")), i18n.Sprintf("newest first · recorded in %s in the library (skd history)", app.HistoryFile),
		i18n.T("no updates recorded yet"), items, show)
	return m, nil
}

// recordLines is one update record in full.
func recordLines(r app.UpdateRecord) []string {
	from := short(r.From)
	if from == "" {
		from = i18n.T("new")
	}
	lines := []string{from + " → " + short(r.To), ""}
	if r.ApplyError != "" {
		lines = append(lines, i18n.Sprintf("linking failed: %s · A applies again", r.ApplyError), "")
	}
	for _, row := range app.RecordRows(r) {
		lines = append(lines, fit(row[0], 14)+row[1])
	}
	lines = append(lines, "")
	switch {
	case r.CommitsUnknown:
		lines = append(lines, i18n.T("upstream commits unknown (history rewritten or not fetched)"))
	case len(r.Commits) == 0:
		lines = append(lines, i18n.T("no upstream commits touched these skills"))
	default:
		lines = append(lines, i18n.T("upstream commits:"))
		for _, c := range r.Commits {
			lines = append(lines, "  "+short(c.Hash)+" "+clean(c.Subject))
		}
		if r.MoreCommits {
			lines = append(lines, i18n.Sprintf("  … more: git log %s..%s in the cache clone", short(r.From), short(r.To)))
		}
	}
	return lines
}

// planView asks before updating (u, U): what upstream changed, per pack
// and skill. For one pack (repo set) it is a confirmation; for every pack
// a list with each pack ticked, and only the ticked ones are updated.
func (m Model) planView(repo string, plan *app.UpdatePlan) (tea.Model, tea.Cmd) {
	m = m.takeCheck(plan.Outdated, repo == "")
	failNote := ""
	if n := len(plan.Failed); n > 0 {
		f := plan.Failed[0]
		failNote = i18n.Sprintf("could not check %s: %s", f.Repo, firstLine(f.Error))
	}
	update := func(repos []string) *job {
		label := i18n.Sprintf("updating %s", plural(len(repos), "pack"))
		if len(repos) == 1 {
			label = i18n.Sprintf("updating %s", repos[0])
		}
		return &job{label: label, run: func(o Ops) result {
			reps, msg, err := o.Update(repos)
			return result{msg: msg, err: err, reports: reps}
		}}
	}
	if repo != "" {
		if failNote != "" {
			m.status, m.statusLvl = failNote, lvErr
			return m, nil
		}
		st := plan.Find(repo)
		if st == nil {
			return m.hint(i18n.Sprintf("%s was not checked", repo))
		}
		lines := []string{short(st.Commit) + " → " + short(st.Latest) + " · " + st.Summary()}
		if st.Commit == "" {
			lines[0] = i18n.Sprintf("not vendored yet · upstream %s", short(st.Latest))
		}
		for _, row := range st.Rows() {
			lines = append(lines, fit(row[0], 14)+row[1])
		}
		if len(plan.Updates) == 0 {
			lines = append(lines, "", i18n.T("Nothing changed upstream. Updating anyway re-vendors missing skills and re-links."))
		}
		lines = append(lines, "", i18n.T("Recorded in the update history (h)."))
		m.ov = confirmOverlay(i18n.Sprintf("Update %s", repo), lines, choice{key: "y", label: i18n.T("update"), then: func() step {
			return step{job: update([]string{repo})}
		}})
		return m, nil
	}
	if len(plan.Updates) == 0 {
		msg, lvl := i18n.T("nothing to update upstream · c checks again · u on a pack updates it anyway"), lvOK
		if failNote != "" {
			msg, lvl = failNote, lvWarn
		}
		m.status, m.statusLvl = msg, lvl
		return m, nil
	}
	var items []pickItem
	for _, st := range plan.Updates {
		items = append(items, pickItem{label: st.Repo, detail: short(st.Commit) + "→" + short(st.Latest) + " · " + st.Summary(),
			value: st.Repo, on: true, group: st.Repo})
		for _, row := range st.Rows() {
			items = append(items, pickItem{label: "  " + row[0], detail: row[1], info: true, group: st.Repo})
		}
	}
	desc := i18n.T("every pack is ticked · space unticks one · enter updates the ticked packs · recorded in the update history (h)")
	if failNote != "" {
		desc = i18n.Sprintf("%s · %s", failNote, desc)
	}
	m.ov = pickOverlay(i18n.Sprintf("Update %s", plural(len(plan.Updates), "pack")), desc, items, false, func(values []string, _ bool) step {
		if len(values) == 0 {
			return step{}
		}
		return step{job: update(values)}
	})
	return m, nil
}

// edit is `skd edit <skill>`: open the skill's directory in the editor. A
// vendored or app skill is replaced on update, so it asks first: fork it
// and open the copy, or open the files anyway.
func (m Model) edit() (tea.Model, tea.Cmd) {
	s, _ := m.target()
	if s == nil {
		return m.hint(i18n.T("select a skill to open"))
	}
	sk := *s
	if !sk.Installed {
		return m.hint(i18n.Sprintf("%s is not installed · space installs it", sk.Name))
	}
	open := func(opts app.EditOptions) func() step {
		return func() step {
			return step{job: &job{label: i18n.Sprintf("opening %s", sk.LinkName), run: func(o Ops) result {
				msg, fg, err := o.Edit(sk, opts)
				r := result{msg: msg, err: err, foreground: fg}
				if opts.Fork {
					r.focus = string(app.NewKind(opts.Private)) + "::" + sk.LinkName
				}
				return r
			}}}
		}
	}
	if sk.Kind != engine.KindVendor && sk.Kind != engine.KindApp {
		return m.start(*open(app.EditOptions{})().job)
	}
	note := app.OverwriteNote(engine.Entry{Kind: sk.Kind, Origin: sk.Origin})
	m.ov = confirmOverlay(i18n.Sprintf("Open %s", sk.LinkName), []string{
		i18n.Sprintf("It %s.", note),
		i18n.T("Fork it to edit your own copy: upstream no longer touches it."),
	}, choice{"f", i18n.T("fork into skills/ and open"), open(app.EditOptions{Fork: true})},
		choice{"p", i18n.T("fork into private/ and open"), open(app.EditOptions{Fork: true, Private: true})},
		choice{"o", i18n.T("open anyway"), open(app.EditOptions{Force: true})})
	return m, nil
}
