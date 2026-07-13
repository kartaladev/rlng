package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
)

// TestBuild_PerDecisionOptions covers Spec 021 criteria 5–6: a decision that
// declares its own options now BUILDS (the old "per-decision options are not
// supported" rejection is gone), the option is honored at runtime, and the
// pipeline constants still reach the decision (per-decision options compose with
// the shared env rather than replacing it).
func TestBuild_PerDecisionOptions(t *testing.T) {
	const yaml = `
version: v1
constants:
  base: 100
stages:
  - name: t
    type: decision-table
    rules:
      - condition: "true"
        decisions:
          limit:
            expr: "missing.field + base"
            fallback: "base"
`
	def, err := config.Parse(t.Context(), config.FromYAMLString(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p, err := def.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	sc := pipe.NewScope(map[string]any{})
	if err := p.Run(t.Context(), sc); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The value expr errors (missing.field), so the fallback `base` applies; base
	// is a pipeline constant, so it must have reached the decision: 100.
	if got, _ := sc.Get("t.limit"); got != 100 {
		t.Fatalf("t.limit = %v, want 100 (fallback to constant base)", got)
	}
}
