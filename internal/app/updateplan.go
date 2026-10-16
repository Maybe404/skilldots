package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
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
		n    int
		word string
	}{{len(p.Changed), "changed"}, {len(p.Pending), "to add"}, {len(offered), "new upstream"}, {len(p.Gone), "gone upstream"}, {len(p.Moved), "moved"}} {
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.word))
		}
	}
	switch {
	case p.Commit == "":
		parts = append([]string{"not vendored yet"}, parts...)
	case len(parts) == 0 && p.Commit != p.Latest:
		parts = []string{"new commit, no skill changes"}
	case len(parts) == 0:
		parts = []string{"up to date"}
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
	}{{"changed", p.Changed, ""}, {"will add", p.Pending, ""}, {"upstream new", p.Offered(), " (not selected)"},
		{"upstream gone", p.Gone, " (kept)"}, {"moved", p.Moved, " (path only)"}} {
		if len(row.names) > 0 {
			out = append(out, [2]string{row.label, strings.Join(row.names, ", ") + row.note})
		}
	}
	return out
}
