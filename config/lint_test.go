package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLint(t *testing.T) {
	t.Parallel()

	rule := func(cond string, tier string) config.RuleDef {
		return config.RuleDef{Condition: config.ExprDef{Expr: cond}, Decisions: map[string]config.ExprDef{"tier": {Expr: tier}}}
	}

	type testCase struct {
		name   string
		def    config.PipelineDef
		assert func(t *testing.T, findings []config.Finding)
	}

	hasCode := func(findings []config.Finding, code string) bool {
		for _, f := range findings {
			if f.Code == code {
				return true
			}
		}
		return false
	}

	cases := []testCase{
		{
			name: "catch-all before later rules is unreachable",
			def: config.PipelineDef{Stages: []config.StageDef{{
				Name: "g", Type: "decision-table",
				Rules: []config.RuleDef{
					rule("true", `"a"`),
					rule("score >= 750", `"b"`),
				},
			}}},
			assert: func(t *testing.T, findings []config.Finding) {
				require.True(t, hasCode(findings, config.LintUnreachableRule))
			},
		},
		{
			name: "no catch-all and no default flags a coverage gap",
			def: config.PipelineDef{Stages: []config.StageDef{{
				Name: "g", Type: "decision-table",
				Rules: []config.RuleDef{
					rule("score >= 750", `"a"`),
				},
			}}},
			assert: func(t *testing.T, findings []config.Finding) {
				require.True(t, hasCode(findings, config.LintMissingDefault))
			},
		},
		{
			name: "catch-all last is exhaustive: no findings",
			def: config.PipelineDef{Stages: []config.StageDef{{
				Name: "g", Type: "decision-table",
				Rules: []config.RuleDef{
					rule("score >= 750", `"a"`),
					rule("true", `"b"`),
				},
			}}},
			assert: func(t *testing.T, findings []config.Finding) {
				assert.Empty(t, findings)
			},
		},
		{
			name: "default present suppresses the coverage-gap finding",
			def: config.PipelineDef{Stages: []config.StageDef{{
				Name: "g", Type: "decision-table",
				Rules: []config.RuleDef{
					rule("score >= 750", `"a"`),
				},
				Default: map[string]config.ExprDef{"tier": {Expr: `"x"`}},
			}}},
			assert: func(t *testing.T, findings []config.Finding) {
				assert.False(t, hasCode(findings, config.LintMissingDefault))
			},
		},
		{
			name: "non-decision-table stages are skipped",
			def: config.PipelineDef{Stages: []config.StageDef{
				{Name: "base", Type: "single-expr", Expr: &config.ExprDef{Expr: "price * qty"}},
			}},
			assert: func(t *testing.T, findings []config.Finding) {
				assert.Empty(t, findings)
			},
		},
		{
			name: "collect table is not flagged for coverage (empty result is valid)",
			def: config.PipelineDef{Stages: []config.StageDef{{
				Name: "c", Type: "decision-table", HitPolicy: "collect",
				Rules: []config.RuleDef{
					rule("score >= 750", `"a"`),
				},
			}}},
			assert: func(t *testing.T, findings []config.Finding) {
				assert.Empty(t, findings)
			},
		},
		{
			name: "semantic catch-all (1 == 1) is recognized: not flagged missing-default",
			def: config.PipelineDef{Stages: []config.StageDef{{
				Name: "t", Type: "decision-table",
				Rules: []config.RuleDef{
					rule("1 == 1", `"1"`),
				},
			}}},
			assert: func(t *testing.T, findings []config.Finding) {
				assert.False(t, hasCode(findings, config.LintMissingDefault), "1 == 1 is a catch-all; must not flag missing-default")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t, tc.def.Lint())
		})
	}
}
