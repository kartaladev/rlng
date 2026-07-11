package stage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiExprExecute(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) (*MultiExpr, *Scope)
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, sc *Scope, err error)
	}

	cases := []testCase{
		{
			name: "writes each result under stage.name",
			build: func(t *testing.T) (*MultiExpr, *Scope) {
				m, err := NewMultiExpr("calc", []NamedExpr{
					{Name: "base", Expression: "price * qty", Priority: 0},
					{Name: "taxed", Expression: "base * 1.1", Priority: 1},
				})
				require.NoError(t, err)
				return m, NewScope(map[string]any{"price": 10.0, "qty": 2.0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
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
			build: func(t *testing.T) (*MultiExpr, *Scope) {
				// 'taxed' references 'base'; declaring it first but with a higher
				// priority number must still evaluate 'base' first.
				m, err := NewMultiExpr("calc", []NamedExpr{
					{Name: "taxed", Expression: "base * 2", Priority: 10},
					{Name: "base", Expression: "5", Priority: 1},
				})
				require.NoError(t, err)
				return m, NewScope(nil)
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				taxed, ok := sc.Get("calc.taxed")
				require.True(t, ok)
				assert.Equal(t, 10, taxed)
			},
		},
		{
			name: "equal priority preserves declaration order",
			build: func(t *testing.T) (*MultiExpr, *Scope) {
				// Both priority 0; 'later' references 'alpha', so a stable sort
				// must keep declaration order for 'later' to see 'alpha'.
				m, err := NewMultiExpr("calc", []NamedExpr{
					{Name: "alpha", Expression: "5", Priority: 0},
					{Name: "later", Expression: "alpha + 1", Priority: 0},
				})
				require.NoError(t, err)
				return m, NewScope(nil)
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				later, ok := sc.Get("calc.later")
				require.True(t, ok)
				assert.Equal(t, 6, later)
			},
		},
		{
			name: "canceled context short-circuits",
			build: func(t *testing.T) (*MultiExpr, *Scope) {
				m, err := NewMultiExpr("calc", []NamedExpr{{Name: "x", Expression: "1"}})
				require.NoError(t, err)
				return m, NewScope(nil)
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.ErrorIs(t, err, context.Canceled)
			},
		},
		{
			name: "provenance on records each expr's derivation",
			build: func(t *testing.T) (*MultiExpr, *Scope) {
				m, err := NewMultiExpr("calc", []NamedExpr{
					{Name: "base", Expression: "price * qty", Priority: 0},
					{Name: "taxed", Expression: "base * 1.1", Priority: 1},
				})
				require.NoError(t, err)
				return m, NewScope(map[string]any{"price": 10.0, "qty": 2.0}, WithProvenance())
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)

				base, ok := sc.Derivation("calc.base")
				require.True(t, ok)
				assert.Equal(t, "calc", base.Stage)
				assert.Equal(t, TypeMultiExpr, base.StageType)
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
		exprs     []NamedExpr
		assert    func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name:      "empty set is rejected",
			stageName: "calc",
			exprs:     nil,
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:      "empty stage name is rejected",
			stageName: "",
			exprs:     []NamedExpr{{Name: "a", Expression: "1"}},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, TypeMultiExpr, se.Type)
				assert.ErrorIs(t, se, errEmptyStageName)
			},
		},
		{
			name:      "empty name is rejected",
			stageName: "calc",
			exprs:     []NamedExpr{{Name: "", Expression: "1"}},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
		{
			name:      "duplicate name is rejected",
			stageName: "calc",
			exprs: []NamedExpr{
				{Name: "a", Expression: "1"},
				{Name: "a", Expression: "2"},
			},
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewMultiExpr(tc.stageName, tc.exprs)
			tc.assert(t, err)
		})
	}
}
