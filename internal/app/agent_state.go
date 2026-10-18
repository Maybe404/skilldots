package app

import (
	"slices"
)

// SetDefaultAgents changes inheritance for skills without an explicit target list.
func (s *Session) SetDefaultAgents(names []string) (*ApplyOutcome, error) {
	for _, name := range names {
		if _, err := s.Reg.Get(name); err != nil {
			return nil, err
		}
	}
	s.Lib.Manifest.Targets.Default = dedupe(append([]string{}, names...))
	return s.saveAndApply()
}

// SetAgentEnabled suspends distribution without erasing per-skill selections.
// Apply removes only links it owns. Enabling restores the existing selections.
func (s *Session) SetAgentEnabled(name string, enabled bool) (*ApplyOutcome, error) {
	return s.PauseAgents([]string{name}, !enabled)
}

// PauseAgents pauses (or resumes) every agent named, all or none: a paused
// agent is listed in targets.disabled of skills.toml, gets no links, and
// keeps every skill's selection of it for when it is resumed.
func (s *Session) PauseAgents(names []string, paused bool) (*ApplyOutcome, error) {
	for _, name := range names {
		// A paused agent skd no longer knows (a custom target taken out of
		// skills.toml) can still be resumed, so the name can leave the list.
		if !paused && s.Paused(name) {
			continue
		}
		if _, err := s.Reg.Get(name); err != nil {
			return nil, err
		}
	}
	disabled := slices.DeleteFunc(slices.Clone(s.Lib.Manifest.Targets.Disabled), func(n string) bool { return slices.Contains(names, n) })
	if paused {
		disabled = append(disabled, dedupe(names)...)
	}
	s.Lib.Manifest.Targets.Disabled = disabled
	return s.saveAndApply()
}

// Paused reports whether the agent is paused (targets.disabled).
func (s *Session) Paused(name string) bool {
	return slices.Contains(s.Lib.Manifest.Targets.Disabled, name)
}
