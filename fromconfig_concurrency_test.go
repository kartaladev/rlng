package rlng_test

import (
	"testing"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromYAMLConcurrency(t *testing.T) {
	yaml := `
stages:
  - name: a
    type: single-expr
    expr: "1 + 1"
`
	tests := []struct {
		name   string
		opts   []rlng.Option
		assert func(t *testing.T, out map[string]any, err error)
	}{
		{
			name: "NewFromYAML with WithConcurrency builds and evaluates",
			opts: []rlng.Option{rlng.WithConcurrency()},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.EqualValues(t, 2, out["a"])
			},
		},
		{
			name: "NewFromYAML with WithMaxParallel(1) builds and evaluates",
			opts: []rlng.Option{rlng.WithMaxParallel(1)},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.EqualValues(t, 2, out["a"])
			},
		},
		{
			name: "invalid bound surfaces from NewFromYAML",
			opts: []rlng.Option{rlng.WithMaxParallel(0)},
			assert: func(t *testing.T, _ map[string]any, err error) {
				var e *pipe.InvalidMaxParallelError
				require.ErrorAs(t, err, &e)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eng, err := rlng.NewFromYAML(t.Context(), yaml, tc.opts...)
			if err != nil {
				tc.assert(t, nil, err)
				return
			}
			out, err := eng.Evaluate(t.Context(), map[string]any{})
			tc.assert(t, out, err)
		})
	}
}

func TestTypedFromYAMLConcurrency(t *testing.T) {
	yaml := `
stages:
  - name: a
    type: single-expr
    expr: "1 + 1"
`
	mapper, err := rlng.NewMapper[map[string]any](rlng.MappingTemplate{"a": "a"})
	require.NoError(t, err)

	eng, err := rlng.NewTypedFromYAML[map[string]any, map[string]any](
		t.Context(), yaml, mapper, rlng.WithConcurrency())
	require.NoError(t, err)

	out, err := eng.Evaluate(t.Context(), map[string]any{})
	require.NoError(t, err)
	assert.EqualValues(t, 2, out["a"])
}

func TestNewRejectsConcurrencyOnPrebuiltPipeline(t *testing.T) {
	p, err := pipe.NewPipeline([]pipe.Stage{})
	require.NoError(t, err)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "New with WithConcurrency",
			call: func() error { _, e := rlng.New(p, rlng.WithConcurrency()); return e },
		},
		{
			name: "New with WithMaxParallel",
			call: func() error { _, e := rlng.New(p, rlng.WithMaxParallel(2)); return e },
		},
		{
			name: "NewTypedEngine with WithConcurrency",
			call: func() error {
				m, _ := rlng.NewMapper[map[string]any](rlng.MappingTemplate{"a": "a"})
				_, e := rlng.NewTypedEngine[map[string]any, map[string]any](p, m, rlng.WithConcurrency())
				return e
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, tc.call(), rlng.ErrConcurrencyRequiresConfig)
		})
	}
}
