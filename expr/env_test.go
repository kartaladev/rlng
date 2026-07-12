package expr

import (
	"testing"
	"time"

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

type envTestNoExportedFields struct {
	hidden string
}

type envTestWithNoExportedFields struct {
	V envTestNoExportedFields
}

type envTestDoublePointer struct {
	N **int
}

type envTestCyclic struct {
	Name string
	Next *envTestCyclic
}

func TestToEnv(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		in     any
		assert func(t *testing.T, got map[string]any, err error)
	}

	cases := []testCase{
		{
			name: "nil is empty env",
			in:   nil,
			assert: func(t *testing.T, got map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, map[string]any{}, got)
			},
		},
		{
			name: "map passthrough",
			in:   map[string]any{"a": 1},
			assert: func(t *testing.T, got map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, map[string]any{"a": 1}, got)
			},
		},
		{
			name: "struct with nested, slice, nil pointer",
			in: envTestOuter{
				Name:  "x",
				Inner: envTestInner{Ratio: 0.5},
				Tags:  []string{"a", "b"},
				Ptr:   nil,
			},
			assert: func(t *testing.T, got map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, map[string]any{
					"Name":  "x",
					"Inner": map[string]any{"Ratio": 0.5},
					"Tags":  []any{"a", "b"},
					"Ptr":   nil,
				}, got)
			},
		},
		{
			name: "pointer to struct converts identically to value struct",
			in: &envTestOuter{
				Name:  "x",
				Inner: envTestInner{Ratio: 0.5},
				Tags:  []string{"a", "b"},
				Ptr:   nil,
			},
			assert: func(t *testing.T, got map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, map[string]any{
					"Name":  "x",
					"Inner": map[string]any{"Ratio": 0.5},
					"Tags":  []any{"a", "b"},
					"Ptr":   nil,
				}, got)
			},
		},
		{
			name: "non-nil pointer field converts to nested map",
			in: envTestOuter{
				Name: "x",
				Ptr:  &envTestInner{Ratio: 0.9},
			},
			assert: func(t *testing.T, got map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, map[string]any{"Ratio": 0.9}, got["Ptr"])
			},
		},
		{
			name: "map field converts element-wise with stringified keys",
			in: envTestWithMap{
				Scores: map[int]string{1: "a", 2: "b"},
			},
			assert: func(t *testing.T, got map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, map[string]any{
					"Scores": map[string]any{"1": "a", "2": "b"},
				}, got)
			},
		},
		{
			name: "unexported field is skipped",
			in: envTestUnexported{
				Public:  "x",
				private: "y",
			},
			assert: func(t *testing.T, got map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, map[string]any{"Public": "x"}, got)
				_, ok := got["private"]
				assert.False(t, ok)
			},
		},
		{
			name: "unsupported kind errors",
			in:   42,
			assert: func(t *testing.T, got map[string]any, err error) {
				require.Error(t, err)
				assert.Nil(t, got)
			},
		},
		{
			name: "value struct with no exported fields (time.Time) survives instead of flattening to an empty map",
			in:   envTestWithTime{T: time.Date(2024, time.March, 15, 10, 30, 0, 0, time.UTC)},
			assert: func(t *testing.T, got map[string]any, err error) {
				require.NoError(t, err)
				gotTime, ok := got["T"].(time.Time)
				require.True(t, ok, "expected T to be a time.Time, got %T", got["T"])
				assert.True(t, time.Date(2024, time.March, 15, 10, 30, 0, 0, time.UTC).Equal(gotTime))
			},
		},
		{
			name: "custom struct with no exported fields passes through as its value",
			in:   envTestWithNoExportedFields{V: envTestNoExportedFields{hidden: "secret"}},
			assert: func(t *testing.T, got map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, envTestNoExportedFields{hidden: "secret"}, got["V"])
			},
		},
		{
			name: "cyclic struct env is a bounded error, not a stack-overflow crash",
			in: func() any {
				n := &envTestCyclic{Name: "loop"}
				n.Next = n
				return n
			}(),
			assert: func(t *testing.T, got map[string]any, err error) {
				require.ErrorIs(t, err, ErrEnvTooDeep)
				assert.Nil(t, got)
			},
		},
		{
			name: "nested pointer field is fully unwrapped to the underlying value",
			in: func() any {
				i := 7
				p := &i
				return envTestDoublePointer{N: &p}
			}(),
			assert: func(t *testing.T, got map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 7, got["N"])
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := toEnv(tc.in)
			tc.assert(t, got, err)
		})
	}
}
