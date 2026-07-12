package pipe_test

import (
	"context"
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiExprExecute(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope)
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, sc *pipe.Scope, err error)
	}

	cases := []testCase{
		{
			name: "writes each result under stage.name",
			build: func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope) {
				m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{
					{Name: "base", Expression: "price * qty", Priority: 0},
					{Name: "taxed", Expression: "base * 1.1", Priority: 1},
				})
				require.NoError(t, err)
				return m, pipe.NewScope(map[string]any{"price": 10.0, "qty": 2.0})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				base, ok := sc.Get("calc.base")
				require.True(t, ok)
				assert.Equal(t, 20.0, base)
				taxed, ok := sc.Get("calc.taxed")
				require.True(t, ok)
				assert.InDelta(t, 22.0, taxed, 1e-9)
			},
		},
		{
			name: "priority controls visibility order",
			build: func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope) {
				// 'taxed' references 'base'; declaring it first but with a higher
				// priority number must still evaluate 'base' first.
				m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{
					{Name: "taxed", Expression: "base * 2", Priority: 10},
					{Name: "base", Expression: "5", Priority: 1},
				})
				require.NoError(t, err)
				return m, pipe.NewScope(nil)
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				taxed, ok := sc.Get("calc.taxed")
				require.True(t, ok)
				assert.Equal(t, 10, taxed)
			},
		},
		{
			name: "equal priority preserves declaration order",
			build: func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope) {
				// Both priority 0; 'later' references 'alpha', so a stable sort
				// must keep declaration order for 'later' to see 'alpha'.
				m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{
					{Name: "alpha", Expression: "5", Priority: 0},
					{Name: "later", Expression: "alpha + 1", Priority: 0},
				})
				require.NoError(t, err)
				return m, pipe.NewScope(nil)
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)
				later, ok := sc.Get("calc.later")
				require.True(t, ok)
				assert.Equal(t, 6, later)
			},
		},
		{
			name: "canceled context short-circuits",
			build: func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope) {
				m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{{Name: "x", Expression: "1"}})
				require.NoError(t, err)
				return m, pipe.NewScope(nil)
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.ErrorIs(t, err, context.Canceled)
			},
		},
		{
			name: "provenance on records each expr's derivation",
			build: func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope) {
				m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{
					{Name: "base", Expression: "price * qty", Priority: 0},
					{Name: "taxed", Expression: "base * 1.1", Priority: 1},
				})
				require.NoError(t, err)
				return m, pipe.NewScope(map[string]any{"price": 10.0, "qty": 2.0}, pipe.WithProvenance())
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				require.NoError(t, err)

				base, ok := sc.Derivation("calc.base")
				require.True(t, ok)
				assert.Equal(t, "calc", base.Stage)
				assert.Equal(t, pipe.TypeMultiExpr, base.StageType)
				assert.Equal(t, "expr:base", base.Operation)
				assert.Equal(t, "price * qty", base.Expression)
				assert.Equal(t, map[string]any{"price": 10.0, "qty": 2.0}, base.Inputs)
				assert.Equal(t, 20.0, base.Value)

				taxed, ok := sc.Derivation("calc.taxed")
				require.True(t, ok)
				assert.Equal(t, "expr:taxed", taxed.Operation)
				assert.Equal(t, "base * 1.1", taxed.Expression)
				assert.Equal(t, map[string]any{"base": 20.0}, taxed.Inputs)
			},
		},
		{
			name: "eval error surfaces as StageError",
			build: func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope) {
				m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{{Name: "x", Expression: "a % b"}})
				require.NoError(t, err)
				return m, pipe.NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "calc", se.Stage)
				assert.Equal(t, pipe.TypeMultiExpr, se.Type)
			},
		},
		{
			name: "write conflict surfaces as StageError",
			build: func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope) {
				// The scalar seed "calc" collides with the stage namespace, so
				// Set("calc.base", …) fails with ErrPathNotMap (provenance off).
				m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{
					{Name: "base", Expression: "price * qty", Priority: 0},
				})
				require.NoError(t, err)
				return m, pipe.NewScope(map[string]any{"calc": 1, "price": 10.0, "qty": 2.0})
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "calc", se.Stage)
				assert.Equal(t, pipe.TypeMultiExpr, se.Type)
				assert.ErrorIs(t, se, pipe.ErrPathNotMap)
			},
		},
		{
			name: "provenance on: write conflict surfaces as StageError",
			build: func(t *testing.T) (*pipe.MultiExpr, *pipe.Scope) {
				// Same collision as above, on the provenance write path (Derive).
				m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{
					{Name: "base", Expression: "price * qty", Priority: 0},
				})
				require.NoError(t, err)
				return m, pipe.NewScope(map[string]any{"calc": 1, "price": 10.0, "qty": 2.0}, pipe.WithProvenance())
			},
			assert: func(t *testing.T, sc *pipe.Scope, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "calc", se.Stage)
				assert.Equal(t, pipe.TypeMultiExpr, se.Type)
				assert.ErrorIs(t, se, pipe.ErrPathNotMap)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, sc := tc.build(t)
			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			err := m.Execute(ctx, sc)
			tc.assert(t, sc, err)
		})
	}
}

func TestNewMultiExprValidation(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name      string
		stageName string
		exprs     []pipe.NamedExpr
		assert    func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name:      "empty set is rejected",
			stageName: "calc",
			exprs:     nil,
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:      "empty stage name is rejected",
			stageName: "",
			exprs:     []pipe.NamedExpr{{Name: "a", Expression: "1"}},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, pipe.TypeMultiExpr, se.Type)
				assert.ErrorIs(t, se, pipe.ErrEmptyStageName)
			},
		},
		{
			name:      "empty name is rejected",
			stageName: "calc",
			exprs:     []pipe.NamedExpr{{Name: "", Expression: "1"}},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:      "duplicate name is rejected",
			stageName: "calc",
			exprs: []pipe.NamedExpr{
				{Name: "a", Expression: "1"},
				{Name: "a", Expression: "2"},
			},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:      "expression fails to compile",
			stageName: "calc",
			exprs:     []pipe.NamedExpr{{Name: "a", Expression: "x +"}},
			assert: func(t *testing.T, err error) {
				var se *pipe.StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, pipe.TypeMultiExpr, se.Type)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := pipe.NewMultiExpr(tc.stageName, tc.exprs)
			tc.assert(t, err)
		})
	}
}

func TestMultiExprAccessors(t *testing.T) {
	t.Parallel()

	m, err := pipe.NewMultiExpr("calc", []pipe.NamedExpr{{Name: "a", Expression: "1"}}, pipe.WithDependsOn("seed"))
	require.NoError(t, err)
	assert.Equal(t, "calc", m.Name())
	assert.Equal(t, pipe.TypeMultiExpr, m.Type())
	assert.Equal(t, []string{"seed"}, m.DependsOn())
}
