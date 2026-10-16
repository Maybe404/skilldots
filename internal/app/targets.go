package app

import (
	"fmt"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// TargetsOutcome is the result of a targets edit.
type TargetsOutcome struct {
	// Label is the skill's link name or the pack's repo.
	Label string
	// Pack is set when a whole pack was edited.
	Pack bool
	// Targets is the new target list.
	Targets []string
	Apply   *ApplyOutcome
}

// EditTargets replaces the targets of a skill (or, when r names no skill
// but a pack, of the pack) with edit(current), saves and re-links.
// Targets inherit: manifest default → pack → skill; a skill's current
// list is what it resolved to, a pack's is its own or the default.
func (s *Session) EditTargets(r Ref, edit func(cur []string) []string) (*TargetsOutcome, error) {
	m := s.Lib.Manifest
	var next []string
	out := &TargetsOutcome{}
	if r.Pack == "" {
		entries, err := s.Resolve()
		if err != nil {
			return nil, err
		}
		e, err := s.Entry(entries, r)
		if err != nil {
			return nil, err
		}
		if e != nil {
			next = nonNil(edit(append([]string(nil), e.TargetNames...)))
			setEntryTargets(m, e, next)
			out.Label = e.LinkName
		} else if r.Kind != "" {
			return nil, fmt.Errorf("no skill %q", r.String())
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
				return nil, fmt.Errorf("no pack %q", key)
			}
			return nil, fmt.Errorf("no skill or pack %q", key)
		}
		cur := append([]string(nil), m.Targets.Default...)
		if p.Targets != nil {
			cur = append([]string(nil), *p.Targets...)
		}
		next = nonNil(edit(cur))
		p.Targets = &next
		out.Label, out.Pack = p.Repo, true
	}
	out.Targets = next
	ap, err := s.saveAndApply()
	if err != nil {
		return nil, err
	}
	out.Apply = ap
	return out, nil
}

// Enable adds agents (the manifest default when none) to r's targets.
func (s *Session) Enable(r Ref, agents []string) (*TargetsOutcome, error) {
	return s.EditTargets(r, func(cur []string) []string {
		if len(agents) == 0 {
			return dedupe(append(cur, s.Lib.Manifest.Targets.Default...))
		}
		return dedupe(append(cur, agents...))
	})
}

// Disable removes agents (all when none) from r's targets. The library
// keeps the files.
func (s *Session) Disable(r Ref, agents []string) (*TargetsOutcome, error) {
	return s.EditTargets(r, func(cur []string) []string {
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
	})
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
