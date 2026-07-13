package config_test

import (
	"strings"
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
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
			name: "single-expr builds and runs",
			def: config.PipelineDef{Stages: []config.StageDef{
				{Name: "base", Type: "single-expr", Expr: &config.ExprDef{Expr: "price * qty"}},
				{Name: "taxed", Type: "single-expr", Expr: &config.ExprDef{Expr: "base * 1.1"}, DependsOn: []string{"base"}},
			}},
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{"price": 10.0, "qty": 2.0})
				require.NoError(t, p.Run(t.Context(), sc))
				v, ok := sc.Get("taxed")
				require.True(t, ok)
				assert.InDelta(t, 22.0, v, 1e-9)
			},
		},
		{
			name: "decision-table collect builds and runs",
			def: sd(config.StageDef{
				Name: "tiers", Type: "decision-table", HitPolicy: "collect",
				Rules: []config.RuleDef{
					{Condition: config.ExprDef{Expr: "amount > 100"}, Decisions: map[string]config.ExprDef{"tag": {Expr: `"big"`}}},
					{Condition: config.ExprDef{Expr: "amount > 0"}, Decisions: map[string]config.ExprDef{"tag": {Expr: `"pos"`}}},
				},
			}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(map[string]any{"amount": 150})
				require.NoError(t, p.Run(t.Context(), sc))
				v, ok := sc.Get("tiers.tag")
				require.True(t, ok)
				assert.Equal(t, []any{"big", "pos"}, v)
			},
		},
		{
			name: "multi-expr builds and runs",
			def: sd(config.StageDef{
				Name: "calc", Type: "multi-expr",
				Exprs: []config.NamedExprDef{
					{Name: "a", Priority: 0, Expr: config.ExprDef{Expr: "2"}},
					{Name: "b", Priority: 1, Expr: config.ExprDef{Expr: "a * 3"}},
				},
			}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(nil)
				require.NoError(t, p.Run(t.Context(), sc))
				v, ok := sc.Get("calc.b")
				require.True(t, ok)
				assert.Equal(t, 6, v)
			},
		},
		{
			name: "condition and output applied",
			def: sd(config.StageDef{
				Name: "gated", Type: "single-expr", Expr: &config.ExprDef{Expr: "99"},
				Condition: &config.ExprDef{Expr: "false"}, Output: "result",
			}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				sc := pipe.NewScope(nil)
				require.NoError(t, p.Run(t.Context(), sc))
				_, ok := sc.Get("result")
				assert.False(t, ok) // condition false => no write
			},
		},
		{
			name:   "unknown type",
			def:    sd(config.StageDef{Name: "x", Type: "bogus"}),
			assert: assertConfigErr,
		},
		{
			name:   "single-expr missing expr",
			def:    sd(config.StageDef{Name: "x", Type: "single-expr"}),
			assert: assertConfigErr,
		},
		{
			name:   "multi-expr empty exprs",
			def:    sd(config.StageDef{Name: "x", Type: "multi-expr"}),
			assert: assertConfigErr,
		},
		{
			name:   "decision-table empty rules",
			def:    sd(config.StageDef{Name: "x", Type: "decision-table"}),
			assert: assertConfigErr,
		},
		{
			name: "invalid hit policy",
			def: sd(config.StageDef{Name: "x", Type: "decision-table", HitPolicy: "weird",
				Rules: []config.RuleDef{{Condition: config.ExprDef{Expr: "true"}, Decisions: map[string]config.ExprDef{"k": {Expr: "1"}}}}}),
			assert: assertConfigErr,
		},
		{
			name: "per-decision options now build (B5)",
			def: sd(config.StageDef{Name: "x", Type: "decision-table",
				Rules: []config.RuleDef{{Condition: config.ExprDef{Expr: "true"}, Decisions: map[string]config.ExprDef{"k": {Expr: "1", Fallback: "0"}}}}}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				require.NoError(t, err)
				require.NotNil(t, p)
			},
		},
		{
			name: "multi-expr constructor error surfaces ConfigError",
			def: sd(config.StageDef{Name: "x", Type: "multi-expr",
				Exprs: []config.NamedExprDef{{Name: "", Priority: 0, Expr: config.ExprDef{Expr: "1"}}}}),
			assert: assertConfigErr,
		},
		{
			name: "decision-table bad decision expr surfaces StageError",
			def: sd(config.StageDef{Name: "x", Type: "decision-table",
				Rules: []config.RuleDef{{Condition: config.ExprDef{Expr: "true"}, Decisions: map[string]config.ExprDef{"k": {Expr: "1 +"}}}}}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				assert.Nil(t, p)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "bad expression surfaces StageError",
			def:  sd(config.StageDef{Name: "x", Type: "single-expr", Expr: &config.ExprDef{Expr: "1 +"}}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				assert.Nil(t, p)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name: "empty pipeline is rejected",
			def:  config.PipelineDef{},
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				assert.Nil(t, p)
				require.ErrorIs(t, err, config.ErrNoStages)
			},
		},
		{
			name: "bad condition is attributed to the condition field",
			def:  sd(config.StageDef{Name: "x", Type: "single-expr", Expr: &config.ExprDef{Expr: "1"}, Condition: &config.ExprDef{Expr: "1 +"}}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				assert.Nil(t, p)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "condition", ce.Field)
			},
		},
		{
			name: "stage error is not double-prefixed with the stage name",
			def: sd(config.StageDef{Name: "t", Type: "decision-table",
				Rules: []config.RuleDef{{Condition: config.ExprDef{Expr: "true"}, Decisions: map[string]config.ExprDef{"k": {Expr: "1 +"}}}}}),
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				assert.Nil(t, p)
				assert.Equal(t, 1, strings.Count(err.Error(), `stage "t"`), "stage name must appear exactly once")
			},
		},
		{
			name: "cycle surfaces pipeline error",
			def: config.PipelineDef{Stages: []config.StageDef{
				{Name: "a", Type: "single-expr", Expr: &config.ExprDef{Expr: "1"}, DependsOn: []string{"b"}},
				{Name: "b", Type: "single-expr", Expr: &config.ExprDef{Expr: "1"}, DependsOn: []string{"a"}},
			}},
			assert: func(t *testing.T, p *pipe.Pipeline, err error) {
				assert.Nil(t, p)
				var ce *pipe.CycleError
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

func TestBuildSingleExprAttributeErrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		doc    []byte
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name: "value-expr failure attributed to expr field",
			doc:  []byte(`{"stages":[{"name":"s","type":"single-expr","expr":"@@@","condition":"###"}]}`),
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "s", ce.Stage)
				assert.Equal(t, "expr", ce.Field) // the value expr is the real first failure
			},
		},
		{
			name: "condition-only failure attributed to condition field",
			doc:  []byte(`{"stages":[{"name":"s","type":"single-expr","expr":"1","condition":"###"}]}`),
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "condition", ce.Field)
			},
		},
		{
			name: "empty stage name with valid expr and condition is not misattributed to condition",
			doc:  []byte(`{"stages":[{"name":"","type":"single-expr","expr":"1","condition":"true"}]}`),
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
				assert.NotEqual(t, "condition", ce.Field)
				assert.Empty(t, ce.Field) // stage-level error, not attributed to a sub-expression
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := config.Parse(t.Context(), config.FromJSONBytes(tc.doc))
			require.NoError(t, err)
			_, err = d.Build()
			tc.assert(t, err)
		})
	}
}

func assertConfigErr(t *testing.T, p *pipe.Pipeline, err error) {
	t.Helper()
	assert.Nil(t, p)
	var ce *config.ConfigError
	require.ErrorAs(t, err, &ce)
}

func TestBuildStampsRulesetIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		opts   []config.BuildOption
		assert func(t *testing.T, id pipe.RulesetIdentity, wantHash string)
	}{
		{
			name: "version from the document",
			assert: func(t *testing.T, id pipe.RulesetIdentity, wantHash string) {
				assert.Equal(t, wantHash, id.Hash)
				assert.Equal(t, "v1.0.0", id.Version)
			},
		},
		{
			name: "WithRulesetVersion overrides the document",
			opts: []config.BuildOption{config.WithRulesetVersion("v2.0.0")},
			assert: func(t *testing.T, id pipe.RulesetIdentity, wantHash string) {
				assert.Equal(t, "v2.0.0", id.Version)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Each subtest parses its own def rather than sharing one across
			// parallel subtests: Build mutates its receiver (hydrateConstants,
			// and now the memoized hashMemo), so two Build() calls on the same
			// *PipelineDef from concurrent subtests would race.
			d, err := config.Parse(t.Context(), config.FromYAMLString(hashYAML+"version: v1.0.0\n"))
			require.NoError(t, err)
			p, err := d.Build(tt.opts...)
			require.NoError(t, err)
			sc := pipe.NewScope(map[string]any{"price": 1, "qty": 1})
			require.NoError(t, p.Run(t.Context(), sc))
			id, ok := sc.Ruleset()
			require.True(t, ok)
			tt.assert(t, id, d.Hash())
		})
	}
}
