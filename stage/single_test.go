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

func TestNewSingleExprCompileError(t *testing.T) {
	t.Parallel()

	_, err := NewSingleExpr("bad", "x +")
	var se *StageError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, TypeSingleExpr, se.Type)
}

func TestNewSingleExprEmptyName(t *testing.T) {
	t.Parallel()

	_, err := NewSingleExpr("", "1")
	var se *StageError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, TypeSingleExpr, se.Type)
	assert.ErrorIs(t, se, errEmptyStageName)
}
