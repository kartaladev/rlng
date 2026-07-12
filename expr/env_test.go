package expr_test

import (
	"testing"
	"time"

	"github.com/kartaladev/rlng/expr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type envTestInner struct{ Ratio float64 }

type envTestOuter struct {
	Name  string
	Inner envTestInner
	Tags  []string
	Ptr   *envTestInner
}

type envTestWithMap struct {
	Scores map[int]string
}

type envTestUnexported struct {
	Public  string
	private string
}

type envTestWithTime struct {
	T time.Time
}

type envTestDoublePointer struct {
	N **int
}

type envTestCyclic struct {
	Name string
	Next *envTestCyclic
}

// TestStructEnvConversion exercises struct / pointer / slice / map env conversion
// through the public Function.Apply, which normalizes a struct env into the
// evaluation environment. It replaces the former white-box test of the internal
// converter, covering the same branches via the exported API.
func TestStructEnvConversion(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		src    string
		env    any
		assert func(t *testing.T, got any, err error)
	}

	cases := []testCase{
		{
			name: "nil env evaluates over an empty environment",
			src:  "1 + 1",
			env:  nil,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 2, got)
			},
		},
		{
			name: "map env passes through",
			src:  "a",
			env:  map[string]any{"a": 1},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 1, got)
			},
		},
		{
			name: "struct with nested struct, slice, and nil pointer",
			src:  `Name == "x" && Inner.Ratio == 0.5 && Tags[0] == "a" && Tags[1] == "b" && Ptr == nil`,
			env:  envTestOuter{Name: "x", Inner: envTestInner{Ratio: 0.5}, Tags: []string{"a", "b"}, Ptr: nil},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, true, got)
			},
		},
		{
			name: "pointer-to-struct env converts like the value struct",
			src:  `Inner.Ratio == 0.5 && Tags[1] == "b"`,
			env:  &envTestOuter{Name: "x", Inner: envTestInner{Ratio: 0.5}, Tags: []string{"a", "b"}},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, true, got)
			},
		},
		{
			name: "non-nil pointer field converts to a nested value",
			src:  "Ptr.Ratio",
			env:  envTestOuter{Ptr: &envTestInner{Ratio: 0.9}},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 0.9, got)
			},
		},
		{
			name: "map field converts with stringified keys",
			src:  `Scores["1"]`,
			env:  envTestWithMap{Scores: map[int]string{1: "a", 2: "b"}},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, "a", got)
			},
		},
		{
			name: "unexported field is skipped (not in the environment)",
			src:  "Public",
			env:  envTestUnexported{Public: "x", private: "y"},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, "x", got)
			},
		},
		{
			name: "unsupported env kind is an error",
			src:  "1",
			env:  42,
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)
				var evalErr *expr.EvalError
				require.ErrorAs(t, err, &evalErr)
			},
		},
		{
			name: "struct with no exported fields (time.Time) survives, so its methods are callable",
			src:  "T.Year()",
			env:  envTestWithTime{T: time.Date(2024, time.March, 15, 10, 30, 0, 0, time.UTC)},
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 2024, got)
			},
		},
		{
			name: "cyclic struct env is a bounded error, not a stack overflow",
			src:  "Name",
			env: func() any {
				n := &envTestCyclic{Name: "loop"}
				n.Next = n
				return n
			}(),
			assert: func(t *testing.T, got any, err error) {
				require.ErrorIs(t, err, expr.ErrEnvTooDeep)
			},
		},
		{
			name: "nested double pointer field is fully unwrapped",
			src:  "N",
			env: func() any {
				i := 7
				p := &i
				return envTestDoublePointer{N: &p}
			}(),
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 7, got)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, err := expr.NewFunction("f", tc.src)
			require.NoError(t, err)
			got, err := f.Apply(tc.env)
			tc.assert(t, got, err)
		})
	}
}
