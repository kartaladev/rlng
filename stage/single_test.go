package stage

import (
	"context"
	"testing"

	"github.com/kartaladev/rlng/expr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSingleExprExecute(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) (*SingleExpr, *Scope)
		ctx    func(ctx context.Context) context.Context // nil = identity
		assert func(t *testing.T, sc *Scope, err error)
	}

	cases := []testCase{
		{
			name: "computes and writes to stage name by default",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("total", "price * qty")
				require.NoError(t, err)
				return s, NewScope(map[string]any{"price": 10, "qty": 3})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("total")
				require.True(t, ok)
				assert.Equal(t, 30, got)
			},
		},
		{
			name: "custom output path",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("total", "price * qty", WithOutput("order.total"))
				require.NoError(t, err)
				return s, NewScope(map[string]any{"price": 10, "qty": 3})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("order.total")
				require.True(t, ok)
				assert.Equal(t, 30, got)
			},
		},
		{
			name: "condition false skips write",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("bonus", "100", WithCondition("vip"))
				require.NoError(t, err)
				return s, NewScope(map[string]any{"vip": false})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				_, ok := sc.Get("bonus")
				assert.False(t, ok, "skipped stage must write nothing")
			},
		},
		{
			name: "condition true writes",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("bonus", "100", WithCondition("vip"))
				require.NoError(t, err)
				return s, NewScope(map[string]any{"vip": true})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("bonus")
				require.True(t, ok)
				assert.Equal(t, 100, got)
			},
		},
		{
			name: "fallback on eval error",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("ratio", "a % b",
					WithExprOptions(expr.WithFallback("-1")))
				require.NoError(t, err)
				return s, NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("ratio")
				require.True(t, ok)
				assert.Equal(t, -1, got)
			},
		},
		{
			name: "eval error surfaces as StageError",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("ratio", "a % b")
				require.NoError(t, err)
				return s, NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "ratio", se.Stage)
			},
		},
		{
			name: "canceled context short-circuits",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("total", "price * qty")
				require.NoError(t, err)
				return s, NewScope(map[string]any{"price": 10, "qty": 3})
			},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.ErrorIs(t, err, context.Canceled)
				_, ok := sc.Get("total")
				assert.False(t, ok)
			},
		},
		{
			name: "provenance on records a derivation",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				s, err := NewSingleExpr("total", "price * qty")
				require.NoError(t, err)
				return s, NewScope(map[string]any{"price": 10, "qty": 3}, WithProvenance())
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				require.NoError(t, err)
				got, ok := sc.Get("total")
				require.True(t, ok)
				assert.Equal(t, 30, got)

				d, ok := sc.Derivation("total")
				require.True(t, ok)
				assert.Equal(t, "total", d.Stage)
				assert.Equal(t, TypeSingleExpr, d.StageType)
				assert.Equal(t, "eval", d.Operation)
				assert.Equal(t, "price * qty", d.Expression)
				assert.Equal(t, map[string]any{"price": 10, "qty": 3}, d.Inputs)
				assert.Equal(t, 30, d.Value)
			},
		},
		{
			name: "condition eval error surfaces as StageError",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				// "a % b" with b == 0 makes the condition predicate fail at eval.
				s, err := NewSingleExpr("bonus", "100", WithCondition("a % b == 0"))
				require.NoError(t, err)
				return s, NewScope(map[string]any{"a": 1, "b": 0})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "bonus", se.Stage)
				assert.Equal(t, TypeSingleExpr, se.Type)
			},
		},
		{
			name: "write conflict surfaces as StageError",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				// Output "price.total" makes the write traverse the scalar seed
				// "price", so Set fails with ErrPathNotMap (provenance off).
				s, err := NewSingleExpr("total", "qty", WithOutput("price.total"))
				require.NoError(t, err)
				return s, NewScope(map[string]any{"price": 10, "qty": 3})
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "total", se.Stage)
				assert.Equal(t, TypeSingleExpr, se.Type)
				assert.ErrorIs(t, se, ErrPathNotMap)
			},
		},
		{
			name: "provenance on: write conflict surfaces as StageError",
			build: func(t *testing.T) (*SingleExpr, *Scope) {
				// Same collision as above, on the provenance write path (Derive).
				s, err := NewSingleExpr("total", "qty", WithOutput("price.total"))
				require.NoError(t, err)
				return s, NewScope(map[string]any{"price": 10, "qty": 3}, WithProvenance())
			},
			assert: func(t *testing.T, sc *Scope, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, "total", se.Stage)
				assert.Equal(t, TypeSingleExpr, se.Type)
				assert.ErrorIs(t, se, ErrPathNotMap)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s, sc := tc.build(t)
			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			err := s.Execute(ctx, sc)
			tc.assert(t, sc, err)
		})
	}
}

func TestNewSingleExprErrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() (*SingleExpr, error)
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name:  "empty name",
			build: func() (*SingleExpr, error) { return NewSingleExpr("", "1") },
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, TypeSingleExpr, se.Type)
				assert.ErrorIs(t, se, errEmptyStageName)
			},
		},
		{
			name:  "expression fails to compile",
			build: func() (*SingleExpr, error) { return NewSingleExpr("bad", "x +") },
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, TypeSingleExpr, se.Type)
			},
		},
		{
			name:  "condition fails to compile",
			build: func() (*SingleExpr, error) { return NewSingleExpr("s", "1", WithCondition("a &&")) },
			assert: func(t *testing.T, err error) {
				var se *StageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, TypeSingleExpr, se.Type)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.build()
			tc.assert(t, err)
		})
	}
}

func TestSingleExprAccessors(t *testing.T) {
	t.Parallel()

	s, err := NewSingleExpr("total", "price * qty", WithDependsOn("base"))
	require.NoError(t, err)
	assert.Equal(t, "total", s.Name())
	assert.Equal(t, TypeSingleExpr, s.Type())
	assert.Equal(t, []string{"base"}, s.DependsOn())
}
