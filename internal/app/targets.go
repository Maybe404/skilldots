package app

import (
	"slices"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// TargetsOutcome is the result of a targets edit.
type TargetsOutcome struct {
	// Label is the skill's link name or the pack's repo.
	Label string
	// Pack is set when a whole pack was edited.
	Pack bool
	// Targets is the new target list, Before the one it replaced.
	Targets []string
	Before  []string
	Apply   *ApplyOutcome
	// NotThere are the agents Disable was asked to take it off that it
	// was not on (nothing changed for them); AlreadyThere, those Enable
	// was asked to put it on that it was already on.
	NotThere     []string
	AlreadyThere []string
	// SharedBefore and SharedAfter are the agents that read the skill (or
	// the pack's skills) through the shared directory ~/.agents/skills
	// before and after the edit (engine.Entry.ViaShared): no link of
	// their own is made or kept for them.
	SharedBefore []string
	SharedAfter  []string
}

// EditTargets replaces the targets of a skill (or, when r names no skill
// but a pack, of the pack) with edit(current), saves and re-links.
// Targets inherit: manifest default → pack → skill; a skill's current
// list is what it resolved to, a pack's is its own or the default, "*"
// written out as the agents it stands for here (engine.ExpandTargets, the
// same expansion Resolve uses).
func (s *Session) EditTargets(r Ref, edit func(cur []string) []string) (*TargetsOutcome, error) {
	return s.editTargets(r, func(_, expanded []string) []string { return edit(expanded) })
}

// editTargets is EditTargets whose edit also gets the list as written
// (raw, with "*" when it has it), so an edit that only adds an agent can
// keep "*" rather than write out the agents it stands for.
func (s *Session) editTargets(r Ref, edit func(raw, expanded []string) []string) (*TargetsOutcome, error) {
	m := s.Lib.Manifest
	var next []string
	out := &TargetsOutcome{}
	var affected func(e engine.Entry) bool
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	if r.Pack == "" {
		e, err := s.Entry(entries, r)
		if err != nil {
			return nil, err
		}
		if e != nil {
			raw := rawTargets(m, e)
			out.Before = append([]string(nil), e.TargetNames...)
			next = nonNil(edit(raw, append([]string(nil), e.TargetNames...)))
			setEntryTargets(m, e, next)
			out.Label = e.LinkName
			kind, origin, name := e.Kind, e.Origin, e.Name
			affected = func(x engine.Entry) bool { return x.Kind == kind && x.Origin == origin && x.Name == name }
		} else if r.Kind != "" {
			return nil, i18n.Errorf("no skill %q", r.String())
		}
	}
	if out.Label == "" {
		key := r.Pack
		if key == "" {
			key = r.Key
		}
		p := m.FindPack(key)
		if p == nil {
			if r.Pack != "" {
				return nil, i18n.Errorf("no pack %q", key)
			}
			return nil, i18n.Errorf("no skill or pack %q", key)
		}
		raw := append([]string(nil), m.Targets.Default...)
		if p.Targets != nil {
			raw = append([]string(nil), *p.Targets...)
		}
		cur := engine.ExpandTargets(raw, m.Targets.Default, s.Reg)
		out.Before = append([]string(nil), cur...)
		next = nonNil(edit(raw, cur))
		p.Targets = &next
		out.Label, out.Pack = p.Repo, true
		repo := p.Repo
		affected = func(x engine.Entry) bool { return x.Kind == engine.KindVendor && x.Origin == repo }
	}
	out.SharedBefore = viaShared(entries, affected)
	out.Targets = next
	ap, err := s.saveAndApply()
	if err != nil {
		return nil, err
	}
	out.Apply = ap
	if after, err := s.Resolve(); err == nil {
		out.SharedAfter = viaShared(after, affected)
	}
	return out, nil
}

// viaShared are the agents that read the entries for which in is true
// through the shared directory (engine.Entry.ViaShared).
func viaShared(entries []engine.Entry, in func(engine.Entry) bool) []string {
	var out []string
	for _, e := range entries {
		if in(e) {
			for _, a := range e.ViaShared {
				if !slices.Contains(out, a) {
					out = append(out, a)
				}
			}
		}
	}
	return out
}

// rawTargets is e's target list as skills.toml has it, inherited and with
// "*" not expanded.
func rawTargets(m *manifest.Manifest, e *engine.Entry) []string {
	list := m.Targets.Default
	switch e.Kind {
	case engine.KindVendor:
		if p := m.FindPack(e.Origin); p != nil {
			if p.Targets != nil {
				list = *p.Targets
			}
			if o := p.Override(e.Name, false); o != nil && o.Targets != nil {
				list = *o.Targets
			}
		}
	case engine.KindApp:
		for _, a := range m.Apps {
			if a.Name == e.Name && a.Targets != nil {
				list = *a.Targets
			}
		}
	default:
		for _, l := range m.Locals {
			if l.Name == e.Name && l.Targets != nil {
				list = *l.Targets
			}
		}
	}
	return append([]string(nil), list...)
}

// Enable adds agents (the manifest default when none) to r's targets. Only
// those agents are added: a list with "*" keeps it, and an agent "*"
// already stands for changes nothing.
func (s *Session) Enable(r Ref, agents []string) (*TargetsOutcome, error) {
	want := agents
	if len(want) == 0 {
		want = s.Lib.Manifest.Targets.Default
	}
	out, err := s.editTargets(r, func(raw, expanded []string) []string {
		next := raw
		for _, a := range want {
			if !slices.Contains(expanded, a) {
				next = append(next, a)
			}
		}
		return dedupe(next)
	})
	if out != nil {
		for _, a := range agents {
			if slices.Contains(out.Before, a) {
				out.AlreadyThere = append(out.AlreadyThere, a)
			}
		}
	}
	return out, err
}

// Disable removes agents (all when none) from r's targets. The library
// keeps the files. A list with "*" is written out as the agents it stands
// for, all of them but those removed.
func (s *Session) Disable(r Ref, agents []string) (*TargetsOutcome, error) {
	out, err := s.EditTargets(r, s.dropAgents(agents))
	if out != nil {
		for _, a := range agents {
			if !slices.Contains(out.Before, a) {
				out.NotThere = append(out.NotThere, a)
			}
		}
	}
	return out, err
}

func (s *Session) dropAgents(agents []string) func(cur []string) []string {
	return func(cur []string) []string {
		if len(agents) == 0 {
			return []string{}
		}
		drop := map[string]bool{}
		for _, a := range agents {
			drop[a] = true
		}
		var out []string
		for _, t := range cur {
			if !drop[t] {
				out = append(out, t)
			}
		}
		return out
	}
}

// SetTargets sets r's targets to exactly agents; none keeps it in the
// library without linking it anywhere.
func (s *Session) SetTargets(r Ref, agents []string) (*TargetsOutcome, error) {
	return s.EditTargets(r, func([]string) []string { return append([]string{}, agents...) })
}

// setEntryTargets records next as the target list of e in the manifest.
func setEntryTargets(m *manifest.Manifest, e *engine.Entry, next []string) {
	switch e.Kind {
	case engine.KindVendor:
		m.FindPack(e.Origin).Override(e.Name, true).Targets = &next
	case engine.KindApp:
		for _, a := range m.Apps {
			if a.Name == e.Name {
				a.Targets = &next
			}
		}
	default:
		for _, l := range m.Locals {
			if l.Name == e.Name {
				l.Targets = &next
				return
			}
		}
		m.Locals = append(m.Locals, &manifest.Local{Name: e.Name, Targets: &next})
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
