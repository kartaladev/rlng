package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDecisionTableExtensions(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		def    config.PipelineDef
		assert func(t *testing.T, p *pipe.Pipeline, err error)
	}

	sd := func(s config.StageDef) config.PipelineDef {
		return config.PipelineDef{Stages: []config.StageDef{s}}
	}

	cases := []testCase{
		{
			name: "unique hit policy builds and errors on multiple matches",
			def: sd(config.StageDef{
				Name: "u", Type: "decision-table", HitPolicy: "unique",
				Rules: []config.RuleDef{
					{Condition: config.ExprDef{Expr: "amount > 100"}, Decisions: map[string]config.ExprDef{"t": {Expr: `"a"`}}},
					{Condition: config.ExprDef{Expr: "amount > 50"}, Decisions: map[string]config.ExprDef{"t": {Expr: `"b"`}}},
				},
			}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{"amount": 200})
				runErr := p.Run(t.Context(), sc)
				assert.ErrorIs(t, runErr, pipe.ErrMultipleMatches)
			},
		},
		{
			name: "collect sum aggregation builds and reduces",
			def: sd(config.StageDef{
				Name: "fees", Type: "decision-table", HitPolicy: "collect", Aggregation: "sum",
				Rules: []config.RuleDef{
					{Condition: config.ExprDef{Expr: "wire"}, Decisions: map[string]config.ExprDef{"fee": {Expr: "25"}}},
					{Condition: config.ExprDef{Expr: "rush"}, Decisions: map[string]config.ExprDef{"fee": {Expr: "15"}}},
				},
			}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{"wire": true, "rush": true})
				require.NoError(t, p.Run(t.Context(), sc))
				v, _ := sc.Get("fees.fee")
				assert.Equal(t, int64(40), v, "an all-int sum stays exact in int64")
			},
		},
		{
			name: "default decisions apply on no match",
			def: sd(config.StageDef{
				Name: "g", Type: "decision-table",
				Rules: []config.RuleDef{
					{Condition: config.ExprDef{Expr: "score >= 750"}, Decisions: map[string]config.ExprDef{"tier": {Expr: `"prime"`}}},
				},
				Default: map[string]config.ExprDef{"tier": {Expr: `"declined"`}},
			}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{"score": 500})
				require.NoError(t, p.Run(t.Context(), sc))
				v, _ := sc.Get("g.tier")
				assert.Equal(t, "declined", v)
			},
		},
		{
			name: "rule id and message surface as the firing rule",
			def: sd(config.StageDef{
				Name: "g", Type: "decision-table",
				Rules: []config.RuleDef{
					{ID: "PRIME", Message: "prime tier", Condition: config.ExprDef{Expr: "score >= 750"}, Decisions: map[string]config.ExprDef{"tier": {Expr: `"prime"`}}},
				},
			}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{"score": 800})
				require.NoError(t, p.Run(t.Context(), sc))
				fr, ok := sc.FiringRule("g")
				require.True(t, ok)
				assert.Equal(t, "PRIME", fr.RuleID)
				assert.Equal(t, "prime tier", fr.Message)
			},
		},
		{
			name: "unknown aggregation is a config error",
			def: sd(config.StageDef{
				Name: "x", Type: "decision-table", HitPolicy: "collect", Aggregation: "median",
				Rules: []config.RuleDef{
					{Condition: config.ExprDef{Expr: "true"}, Decisions: map[string]config.ExprDef{"v": {Expr: "1"}}},
				},
			}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := tc.def.Build()
			tc.assert(t, p, err)
		})
	}
}

func TestBuildTableConfigBranches(t *testing.T) {
	t.Parallel()

	sd := func(s config.StageDef) config.PipelineDef {
		return config.PipelineDef{Stages: []config.StageDef{s}}
	}
	collectAgg := func(agg string) config.PipelineDef {
		return sd(config.StageDef{
			Name: "c", Type: "decision-table", HitPolicy: "collect", Aggregation: agg,
			Rules: []config.RuleDef{
				{Condition: config.ExprDef{Expr: "true"}, Decisions: map[string]config.ExprDef{"v": {Expr: "10"}}},
				{Condition: config.ExprDef{Expr: "true"}, Decisions: map[string]config.ExprDef{"v": {Expr: "4"}}},
			},
		})
	}

	type testCase struct {
		name   string
		def    config.PipelineDef
		assert func(t *testing.T, p *pipe.Pipeline, err error)
	}

	cases := []testCase{
		{
			name: "any hit policy builds and agrees",
			def: sd(config.StageDef{
				Name: "an", Type: "decision-table", HitPolicy: "any",
				Rules: []config.RuleDef{
					{Condition: config.ExprDef{Expr: "true"}, Decisions: map[string]config.ExprDef{"ok": {Expr: "true"}}},
					{Condition: config.ExprDef{Expr: "true"}, Decisions: map[string]config.ExprDef{"ok": {Expr: "true"}}},
				},
			}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{})
				require.NoError(t, p.Run(t.Context(), sc))
				v, _ := sc.Get("an.ok")
				assert.Equal(t, true, v)
			},
		},
		{
			name: "aggregation min",
			def:  collectAgg("min"),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{})
				require.NoError(t, p.Run(t.Context(), sc))
				v, _ := sc.Get("c.v")
				assert.Equal(t, 4, v)
			},
		},
		{
			name: "aggregation max",
			def:  collectAgg("max"),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{})
				require.NoError(t, p.Run(t.Context(), sc))
				v, _ := sc.Get("c.v")
				assert.Equal(t, 10, v)
			},
		},
		{
			name: "aggregation count",
			def:  collectAgg("count"),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{})
				require.NoError(t, p.Run(t.Context(), sc))
				v, _ := sc.Get("c.v")
				assert.Equal(t, 2, v)
			},
		},
		{
			name: "default with per-decision options is a config error",
			def: sd(config.StageDef{
				Name: "g", Type: "decision-table",
				Rules: []config.RuleDef{
					{Condition: config.ExprDef{Expr: "true"}, Decisions: map[string]config.ExprDef{"x": {Expr: "1"}}},
				},
				Default: map[string]config.ExprDef{"x": {Expr: "1", Fallback: "0"}},
			}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := tc.def.Build()
			tc.assert(t, p, err)
		})
	}
}
