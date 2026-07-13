package pipe_test

import (
	"testing"

	"github.com/kartaladev/rlng/expr"
	"github.com/kartaladev/rlng/pipe"
)

// TestDecisionTable_PerDecisionFallback covers Spec 021 criterion 1: a
// per-decision fallback recovers one output while a sibling without one still
// errors under the same rule.
func TestDecisionTable_PerDecisionFallback(t *testing.T) {
	tests := []struct {
		name   string
		rule   pipe.Rule
		assert func(t *testing.T, sc *pipe.Scope, err error)
	}{
		{
			name: "fallback output recovers on a value error",
			rule: pipe.Rule{
				Condition: "true",
				Decisions: map[string]pipe.Decision{
					"safe": {Expr: "missing.field + 1", Options: []expr.Option{expr.WithFallback("0")}},
				},
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				if err != nil {
					t.Fatalf("expected fallback to recover, got %v", err)
				}
				if got, _ := sc.Get("t.safe"); got != 0 {
					t.Fatalf("safe = %v, want 0", got)
				}
			},
		},
		{
			name: "sibling without a fallback still errors",
			rule: pipe.Rule{
				Condition: "true",
				Decisions: map[string]pipe.Decision{
					"boom": {Expr: "missing.field + 1"}, // value error, no fallback
				},
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				if err == nil {
					t.Fatal("expected error from fallback-less decision, got nil")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dt, err := pipe.NewDecisionTable("t", []pipe.Rule{tt.rule})
			if err != nil {
				t.Fatalf("NewDecisionTable: %v", err)
			}
			sc := pipe.NewScope(map[string]any{"n": 0})
			err = dt.Execute(t.Context(), sc)
			tt.assert(t, sc, err)
		})
	}
}

// TestDecisionTable_PerDecisionGlobalsAndCoerce covers Spec 021 criterion 2:
// per-decision globals are honored (previously such a decision was rejected by
// the config layer and unrepresentable in pipe).
func TestDecisionTable_PerDecisionGlobalsAndCoerce(t *testing.T) {
	dt, err := pipe.NewDecisionTable("t", []pipe.Rule{{
		Condition: "true",
		Decisions: map[string]pipe.Decision{
			"g": {Expr: "threshold", Options: []expr.Option{expr.WithGlobals(map[string]any{"threshold": 42})}},
		},
	}})
	if err != nil {
		t.Fatalf("NewDecisionTable: %v", err)
	}
	sc := pipe.NewScope(map[string]any{})
	if err := dt.Execute(t.Context(), sc); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, _ := sc.Get("t.g"); got != 42 {
		t.Fatalf("g = %v, want 42", got)
	}
}

// TestDecisionTable_DefaultPerDecisionFallback covers Spec 021 criterion 3
// (D3 symmetry): a default decision carries its own fallback.
func TestDecisionTable_DefaultPerDecisionFallback(t *testing.T) {
	dt, err := pipe.NewDecisionTable("t",
		[]pipe.Rule{{Condition: "false", Decisions: map[string]pipe.Decision{"x": {Expr: "1"}}}},
		pipe.WithDefault(map[string]pipe.Decision{
			"x": {Expr: "missing.field", Options: []expr.Option{expr.WithFallback("7")}},
		}),
	)
	if err != nil {
		t.Fatalf("NewDecisionTable: %v", err)
	}
	sc := pipe.NewScope(map[string]any{})
	if err := dt.Execute(t.Context(), sc); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, _ := sc.Get("t.x"); got != 7 {
		t.Fatalf("default x = %v, want 7 (fallback)", got)
	}
}
