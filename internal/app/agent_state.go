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
	if _, err := s.Reg.Get(name); err != nil {
		return nil, err
	}
	disabled := slices.DeleteFunc(s.Lib.Manifest.Targets.Disabled, func(n string) bool { return n == name })
	if !enabled {
		disabled = append(disabled, name)
	}
	s.Lib.Manifest.Targets.Disabled = disabled
	return s.saveAndApply()
}
