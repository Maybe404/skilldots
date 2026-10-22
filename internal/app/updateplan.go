package app

import (
	"context"
	"slices"
	"strings"

	"github.com/Maybe404/skilldots/internal/i18n"
)

// UpdatePlan is what `update` would do, from a fresh upstream check: the
// packs it would change, skill by skill, for a person to confirm before
// anything in the library changes.
type UpdatePlan struct {
	// Outdated is the check itself (recorded like `skd outdated`).
	*Outdated
	// Updates are the packs an update would change, in manifest order.
	// Without named packs those are the packs with updates
	// (PackStatus.HasUpdates); a named pack is also offered when upstream
	// only moved skills or has a new commit with no skill changes.
	Updates []PackStatus
}

// PlanUpdate checks the named packs (every pack when none) and lists what
// updating them would change. Nothing in the library changes; callers
// show the plan, let the person choose, and pass the chosen packs to
// Update, which fetches again (upstream may have moved on meanwhile).
func (s *Session) PlanUpdate(ctx context.Context, repos []string, each func(repo string, st *PackStatus, err error)) (*UpdatePlan, error) {
	o, err := s.OutdatedContext(ctx, repos, each)
	if err != nil {
		return nil, err
	}
	plan := &UpdatePlan{Outdated: o, Updates: []PackStatus{}}
	named := len(repos) > 0
	for _, st := range o.Packs {
		if st.HasUpdates() || (named && (len(st.Moved) > 0 || st.Commit != st.Latest)) {
			plan.Updates = append(plan.Updates, st)
		}
	}
	return plan, nil
}

// Repos are the repositories of the packs in the plan.
func (p *UpdatePlan) Repos() []string {
	var out []string
	for _, st := range p.Updates {
		out = append(out, st.Repo)
	}
	return out
}

// Summary counts what updating the pack would change: "2 changed · 1 new".
func (p PackStatus) Summary() string {
	var parts []string
	offered := p.Offered()
	for _, c := range []struct {
		n      int
		format string
	}{{len(p.Changed), i18n.M("%d changed")}, {len(p.Pending), i18n.M("%d to add")}, {len(offered), i18n.M("%d new upstream")}, {len(p.Gone), i18n.M("%d gone upstream")}, {len(p.Moved), i18n.M("%d moved")},
		{len(p.Landed), i18n.M("%d landed")}} {
		if c.n > 0 {
			parts = append(parts, i18n.Sprintf(c.format, c.n))
		}
	}
	switch {
	case p.Commit == "":
		parts = append([]string{i18n.T("not vendored yet")}, parts...)
	case len(parts) == 0 && p.Commit != p.Latest:
		parts = []string{i18n.T("new commit, no skill changes")}
	case len(parts) == 0:
		parts = []string{i18n.T("up to date")}
	}
	return strings.Join(parts, " · ")
}

// Offered are the new upstream skills the pack does not select: an
// update reports them and `add` installs them. New skills a pack that
// follows everything selects are in Pending.
func (p PackStatus) Offered() []string {
	var out []string
	for _, n := range p.New {
		if !slices.Contains(p.Pending, n) {
			out = append(out, n)
		}
	}
	return out
}

// Rows are the skill-level changes of a pack, labelled, in the order
// front ends show them: {"changed", "tdd, review"}.
func (p PackStatus) Rows() [][2]string {
	var out [][2]string
	for _, row := range []struct {
		label string
		names []string
		note  string
	}{{i18n.M("changed"), p.Changed, ""}, {i18n.M("will add"), p.Pending, ""}, {i18n.M("upstream new"), p.Offered(), i18n.M(" (not selected)")},
		{i18n.M("upstream gone"), p.Gone, i18n.M(" (kept)")}, {i18n.M("moved"), p.Moved, i18n.M(" (path only)")},
		{i18n.M("landed"), p.Landed, i18n.M(" (already what you have)")}} {
		if len(row.names) > 0 {
			out = append(out, [2]string{i18n.T(row.label), strings.Join(row.names, ", ") + i18n.T(row.note)})
		}
	}
	return out
}
