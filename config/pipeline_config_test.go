package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildWithConstants(t *testing.T) {
	t.Parallel()

	// threshold is a pipeline-level constant referenced by both a rule condition
	// and the default decision; runtime input may override it.
	def := config.PipelineDef{
		Constants: map[string]any{"threshold": 650, "passLabel": "pass"},
		Stages: []config.StageDef{
			{
				Name: "g", Type: "decision-table",
				Rules: []config.RuleDef{
					{Condition: config.ExprDef{Expr: "score >= threshold"}, Decisions: map[string]config.ExprDef{"tier": {Expr: "passLabel"}}},
				},
				Default: map[string]config.ExprDef{"tier": {Expr: `"fail"`}},
			},
		},
	}

	type testCase struct {
		name  string
		score int
		want  string
	}

	cases := []testCase{
		{name: "above threshold passes", score: 700, want: "pass"},
		{name: "below threshold hits default", score: 600, want: "fail"},
	}

	p, err := def.Build()
	require.NoError(t, err)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sc := pipe.NewScope(map[string]any{"score": tc.score})
			require.NoError(t, p.Run(t.Context(), sc))
			v, _ := sc.Get("g.tier")
			assert.Equal(t, tc.want, v)
		})
	}
}

func TestConstantsPreserveStageGlobals(t *testing.T) {
	t.Parallel()

	// A pipeline constant and a per-expression global must coexist: injecting
	// constants must not discard the stage's own declared globals.
	def := config.PipelineDef{
		Constants: map[string]any{"suffix": "-const"},
		Stages: []config.StageDef{
			{Name: "s", Type: "single-expr", Expr: &config.ExprDef{
				Expr:    "label + suffix",
				Globals: map[string]any{"label": "stage"},
			}},
		},
	}
	p, err := def.Build()
	require.NoError(t, err)

	sc := pipe.NewScope(map[string]any{})
	require.NoError(t, p.Run(t.Context(), sc))
	v, _ := sc.Get("s")
	// Both the stage global (label) and the pipeline constant (suffix) resolve.
	assert.Equal(t, "stage-const", v)
}

func TestParseMapping(t *testing.T) {
	t.Parallel()

	yaml := `
constants:
  taxRate: 0.1
stages:
  - name: base
    type: single-expr
    expr: price * qty
mapping:
  total: base * (1 + taxRate)
  label: "'quote'"
`
	def, err := config.Parse(t.Context(), config.FromYAMLString(yaml))
	require.NoError(t, err)
	assert.Equal(t, "base * (1 + taxRate)", def.Mapping["total"])
	assert.Equal(t, "'quote'", def.Mapping["label"])
	assert.Equal(t, 0.1, def.Constants["taxRate"])
}
