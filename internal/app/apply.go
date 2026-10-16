package app

import (
	"github.com/Maybe404/skilldots/internal/engine"
)

// ApplyOptions tunes Apply.
type ApplyOptions struct {
	// DryRun plans without changing anything.
	DryRun bool
	// Force replaces unmanaged or modified entries (moved to the trash).
	Force bool
	// ForceKeys forces only these "<target>/<name>" entries.
	ForceKeys map[string]bool
}

// ApplyOutcome is the result of an apply; its JSON form is what
// `skd apply --json` prints.
type ApplyOutcome struct {
	Applied bool            `json:"applied"`
	DryRun  bool            `json:"dryRun,omitempty"`
	Changes []engine.Change `json:"changes"`
	Kept    int             `json:"kept"`
	Backups []string        `json:"backups,omitempty"`
	// Plan is what was (or, for a dry run, would be) done.
	Plan engine.Plan `json:"-"`
}

// Counts tallies the plan by action.
func (o *ApplyOutcome) Counts() map[string]int { return o.Plan.Counts() }

// Plan computes what apply would do for entries.
func (s *Session) Plan(entries []engine.Entry, force bool) engine.Plan {
	return engine.MakePlan(s.Lib, entries, force)
}

// Pending counts the link changes `skd apply` would make, skips excluded.
func (s *Session) Pending(entries []engine.Entry) int {
	n := 0
	for _, c := range s.Plan(entries, false).ChangeList() {
		if c.Action != "skip" {
			n++
		}
	}
	return n
}

// Apply makes the agent directories match the manifest and saves the lock.
// On failure the outcome still lists what was moved aside.
func (s *Session) Apply(o ApplyOptions) (*ApplyOutcome, error) {
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	plan := engine.MakePlanForce(s.Lib, entries, func(key string) bool { return o.Force || o.ForceKeys[key] })
	out := &ApplyOutcome{DryRun: o.DryRun, Changes: plan.ChangeList(), Kept: plan.Counts()["keep"], Plan: plan}
	if !plan.Changes() || o.DryRun {
		return out, nil
	}
	res, err := engine.Apply(s.Lib, plan)
	if saveErr := s.Lib.Save(); saveErr != nil && err == nil {
		err = saveErr
	}
	if res != nil {
		out.Backups = res.Backups
	}
	if err != nil {
		return out, err
	}
	out.Applied = true
	return out, nil
}

// saveAndApply saves the manifest and lock, then re-links.
func (s *Session) saveAndApply() (*ApplyOutcome, error) {
	if err := s.Lib.Save(); err != nil {
		return nil, err
	}
	return s.Apply(ApplyOptions{})
}
