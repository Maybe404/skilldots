package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/app"
	"github.com/Maybe404/skilldots/internal/engine"
)

// When some planned change was not made, apply does not end on a plain
// "applied": its last line counts what was not changed, and the command
// exits non-zero (with its JSON document printed first under --json).
func TestApplyWithRefusedChangesDoesNotLookDone(t *testing.T) {
	var out bytes.Buffer
	prev := stdout
	defer func() { stdout = prev }()
	stdout = &out
	plan := engine.Plan{Ops: []engine.Op{{Action: "create", Name: "a"}, {Action: "create", Name: "b"}}}
	o := &app.ApplyOutcome{Applied: true, Plan: plan, Refused: []string{"/x/b: not changed: target moved"}}
	printApply(&ctx{}, o)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	last := lines[len(lines)-1]
	if strings.Contains(last, "✓") || !strings.Contains(last, "1 not changed") {
		t.Errorf("last line %q", last)
	}
	if err := refusedError(o); err == nil || !strings.Contains(err.Error(), "1 planned change") {
		t.Errorf("error %v", err)
	}
	out.Reset()
	o.Refused = nil
	printApply(&ctx{}, o)
	if !strings.Contains(out.String(), "✓") || refusedError(o) != nil {
		t.Errorf("without refusals: %q", out.String())
	}
}
