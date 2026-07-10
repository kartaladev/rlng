package config

import (
	"testing"

	"github.com/kartaladev/rlng/stage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		def    PipelineDef
		assert func(t *testing.T, p *stage.Pipeline, err error)
	}

	sd := func(s StageDef) PipelineDef { return PipelineDef{Stages: []StageDef{s}} }

	cases := []testCase{
		{
			name: "single-expr builds and runs",
			def: PipelineDef{Stages: []StageDef{
				{Name: "base", Type: "single-expr", Expr: &ExprDef{Expr: "price * qty"}},
				{Name: "taxed", Type: "single-expr", Expr: &ExprDef{Expr: "base * 1.1"}, DependsOn: []string{"base"}},
			}},
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				require.NoError(t, err)
				sc := stage.NewScope(map[string]any{"price": 10.0, "qty": 2.0})
				require.NoError(t, p.Run(t.Context(), sc))
				v, ok := sc.Get("taxed")
				require.True(t, ok)
				assert.InDelta(t, 22.0, v, 1e-9)
			},
		},
		{
			name: "decision-table collect builds and runs",
			def: sd(StageDef{
				Name: "tiers", Type: "decision-table", HitPolicy: "collect",
				Rules: []RuleDef{
					{Condition: ExprDef{Expr: "amount > 100"}, Decisions: map[string]ExprDef{"tag": {Expr: `"big"`}}},
					{Condition: ExprDef{Expr: "amount > 0"}, Decisions: map[string]ExprDef{"tag": {Expr: `"pos"`}}},
				},
			}),
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				require.NoError(t, err)
				sc := stage.NewScope(map[string]any{"amount": 150})
				require.NoError(t, p.Run(t.Context(), sc))
				v, ok := sc.Get("tiers.tag")
				require.True(t, ok)
				assert.Equal(t, []any{"big", "pos"}, v)
			},
		},
		{
			name: "multi-expr builds and runs",
			def: sd(StageDef{
				Name: "calc", Type: "multi-expr",
				Exprs: []NamedExprDef{
					{Name: "a", Priority: 0, Expr: ExprDef{Expr: "2"}},
					{Name: "b", Priority: 1, Expr: ExprDef{Expr: "a * 3"}},
				},
			}),
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				require.NoError(t, err)
				sc := stage.NewScope(nil)
				require.NoError(t, p.Run(t.Context(), sc))
				v, ok := sc.Get("calc.b")
				require.True(t, ok)
				assert.Equal(t, 6, v)
			},
		},
		{
			name: "condition and output applied",
			def: sd(StageDef{
				Name: "gated", Type: "single-expr", Expr: &ExprDef{Expr: "99"},
				Condition: &ExprDef{Expr: "false"}, Output: "result",
			}),
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				require.NoError(t, err)
				sc := stage.NewScope(nil)
				require.NoError(t, p.Run(t.Context(), sc))
				_, ok := sc.Get("result")
				assert.False(t, ok) // condition false => no write
			},
		},
		{
			name:   "unknown type",
			def:    sd(StageDef{Name: "x", Type: "bogus"}),
			assert: assertConfigErr,
		},
		{
			name:   "single-expr missing expr",
			def:    sd(StageDef{Name: "x", Type: "single-expr"}),
			assert: assertConfigErr,
		},
		{
			name:   "multi-expr empty exprs",
			def:    sd(StageDef{Name: "x", Type: "multi-expr"}),
			assert: assertConfigErr,
		},
		{
			name:   "decision-table empty rules",
			def:    sd(StageDef{Name: "x", Type: "decision-table"}),
			assert: assertConfigErr,
		},
		{
			name: "invalid hit policy",
			def: sd(StageDef{Name: "x", Type: "decision-table", HitPolicy: "weird",
				Rules: []RuleDef{{Condition: ExprDef{Expr: "true"}, Decisions: map[string]ExprDef{"k": {Expr: "1"}}}}}),
			assert: assertConfigErr,
		},
		{
			name: "per-decision options rejected",
			def: sd(StageDef{Name: "x", Type: "decision-table",
				Rules: []RuleDef{{Condition: ExprDef{Expr: "true"}, Decisions: map[string]ExprDef{"k": {Expr: "1", Fallback: "0"}}}}}),
			assert: assertConfigErr,
		},
		{
			name: "multi-expr constructor error surfaces ConfigError",
			def: sd(StageDef{Name: "x", Type: "multi-expr",
				Exprs: []NamedExprDef{{Name: "", Priority: 0, Expr: ExprDef{Expr: "1"}}}}),
			assert: assertConfigErr,
		},
		{
			name: "decision-table bad decision expr surfaces StageError",
			def: sd(StageDef{Name: "x", Type: "decision-table",
				Rules: []RuleDef{{Condition: ExprDef{Expr: "true"}, Decisions: map[string]ExprDef{"k": {Expr: "1 +"}}}}}),
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				assert.Nil(t, p)
				var ce *ConfigError
				require.ErrorAs(t, err, &ce)
				var se *stage.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "bad expression surfaces StageError",
			def:  sd(StageDef{Name: "x", Type: "single-expr", Expr: &ExprDef{Expr: "1 +"}}),
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				assert.Nil(t, p)
				var ce *ConfigError
				require.ErrorAs(t, err, &ce)
				var se *stage.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "cycle surfaces pipeline error",
			def: PipelineDef{Stages: []StageDef{
				{Name: "a", Type: "single-expr", Expr: &ExprDef{Expr: "1"}, DependsOn: []string{"b"}},
				{Name: "b", Type: "single-expr", Expr: &ExprDef{Expr: "1"}, DependsOn: []string{"a"}},
			}},
			assert: func(t *testing.T, p *stage.Pipeline, err error) {
				assert.Nil(t, p)
				var ce *stage.CycleError
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

func assertConfigErr(t *testing.T, p *stage.Pipeline, err error) {
	t.Helper()
	assert.Nil(t, p)
	var ce *ConfigError
	require.ErrorAs(t, err, &ce)
}
