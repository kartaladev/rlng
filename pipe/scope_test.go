package pipe_test

import (
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeSetGet(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() *pipe.Scope
		assert func(t *testing.T, s *pipe.Scope)
	}

	cases := []testCase{
		{
			name:  "single-segment set and get",
			build: func() *pipe.Scope { return pipe.NewScope(nil) },
			assert: func(t *testing.T, s *pipe.Scope) {
				require.NoError(t, s.Set("amount", 100))
				got, ok := s.Get("amount")
				require.True(t, ok)
				assert.Equal(t, 100, got)
			},
		},
		{
			name:  "dot-path creates nested maps",
			build: func() *pipe.Scope { return pipe.NewScope(nil) },
			assert: func(t *testing.T, s *pipe.Scope) {
				require.NoError(t, s.Set("discount.rate", 0.1))
				got, ok := s.Get("discount.rate")
				require.True(t, ok)
				assert.Equal(t, 0.1, got)
			},
		},
		{
			name:  "seed is read via get",
			build: func() *pipe.Scope { return pipe.NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *pipe.Scope) {
				got, ok := s.Get("a")
				require.True(t, ok)
				assert.Equal(t, 1, got)
			},
		},
		{
			name:  "missing path returns false",
			build: func() *pipe.Scope { return pipe.NewScope(nil) },
			assert: func(t *testing.T, s *pipe.Scope) {
				_, ok := s.Get("nope")
				assert.False(t, ok)
			},
		},
		{
			name:  "get with empty path returns false",
			build: func() *pipe.Scope { return pipe.NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *pipe.Scope) {
				_, ok := s.Get("")
				assert.False(t, ok)
			},
		},
		{
			name:  "get descending through a scalar returns false",
			build: func() *pipe.Scope { return pipe.NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *pipe.Scope) {
				_, ok := s.Get("a.b")
				assert.False(t, ok)
			},
		},
		{
			name:  "empty path is an error",
			build: func() *pipe.Scope { return pipe.NewScope(nil) },
			assert: func(t *testing.T, s *pipe.Scope) {
				require.ErrorIs(t, s.Set("", 1), pipe.ErrEmptyPath)
			},
		},
		{
			name:  "descend through scalar errors",
			build: func() *pipe.Scope { return pipe.NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *pipe.Scope) {
				require.ErrorIs(t, s.Set("a.b", 2), pipe.ErrPathNotMap)
			},
		},
		{
			name:  "lenient overwrite (default) wins last",
			build: func() *pipe.Scope { return pipe.NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *pipe.Scope) {
				require.NoError(t, s.Set("a", 2))
				got, _ := s.Get("a")
				assert.Equal(t, 2, got)
			},
		},
		{
			name:  "strict overwrite conflicts",
			build: func() *pipe.Scope { return pipe.NewScope(map[string]any{"a": 1}, pipe.WithStrict()) },
			assert: func(t *testing.T, s *pipe.Scope) {
				require.ErrorIs(t, s.Set("a", 2), pipe.ErrPathConflict)
			},
		},
		{
			name:  "snapshot is decoupled from later writes",
			build: func() *pipe.Scope { return pipe.NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *pipe.Scope) {
				snap := s.Snapshot()
				require.NoError(t, s.Set("b", 2))
				_, ok := snap["b"]
				assert.False(t, ok, "snapshot must not see writes made after it was taken")
			},
		},
		{
			name:  "seed copy protects against caller mutation",
			build: func() *pipe.Scope { return nil }, // built inline below
			assert: func(t *testing.T, _ *pipe.Scope) {
				seed := map[string]any{"a": 1}
				s := pipe.NewScope(seed)
				seed["a"] = 999
				got, _ := s.Get("a")
				assert.Equal(t, 1, got, "scope must not observe caller's post-construction seed mutation")
			},
		},
		{
			name:  "nested seed map is deep-copied (caller mutation not observed)",
			build: func() *pipe.Scope { return nil },
			assert: func(t *testing.T, _ *pipe.Scope) {
				seed := map[string]any{"cfg": map[string]any{"rate": 1}}
				s := pipe.NewScope(seed)
				seed["cfg"].(map[string]any)["rate"] = 999
				got, _ := s.Get("cfg.rate")
				assert.Equal(t, 1, got, "scope must own a deep copy of nested seed maps")
			},
		},
		{
			name:  "pipeline write does not mutate caller's nested seed map",
			build: func() *pipe.Scope { return nil },
			assert: func(t *testing.T, _ *pipe.Scope) {
				seed := map[string]any{"cfg": map[string]any{"existing": 1}}
				s := pipe.NewScope(seed)
				require.NoError(t, s.Set("cfg.added", 2))
				_, ok := seed["cfg"].(map[string]any)["added"]
				assert.False(t, ok, "Set must not write into the caller's nested map")
			},
		},
		{
			name:  "empty leading path segment is an error",
			build: func() *pipe.Scope { return pipe.NewScope(nil) },
			assert: func(t *testing.T, s *pipe.Scope) {
				require.ErrorIs(t, s.Set(".a", 1), pipe.ErrEmptyPathSegment)
			},
		},
		{
			name:  "empty middle path segment is an error",
			build: func() *pipe.Scope { return pipe.NewScope(nil) },
			assert: func(t *testing.T, s *pipe.Scope) {
				require.ErrorIs(t, s.Set("a..b", 1), pipe.ErrEmptyPathSegment)
			},
		},
		{
			name:  "empty trailing path segment is an error",
			build: func() *pipe.Scope { return pipe.NewScope(nil) },
			assert: func(t *testing.T, s *pipe.Scope) {
				require.ErrorIs(t, s.Set("a.", 1), pipe.ErrEmptyPathSegment)
			},
		},
		{
			name:  "dotted seed key is stored verbatim (not split), never losing data",
			build: func() *pipe.Scope { return nil },
			assert: func(t *testing.T, _ *pipe.Scope) {
				// A scalar key and a dotted key sharing its prefix must both survive,
				// deterministically, regardless of map iteration order (no silent loss).
				s := pipe.NewScope(map[string]any{"a": 1, "a.b": 7})
				snap := s.Snapshot()
				assert.Equal(t, 1, snap["a"])
				assert.Equal(t, 7, snap["a.b"])
			},
		},
		{
			name:  "zero-value Scope Set does not panic",
			build: func() *pipe.Scope { return &pipe.Scope{} },
			assert: func(t *testing.T, s *pipe.Scope) {
				require.NoError(t, s.Set("a", 1))
				got, ok := s.Get("a")
				require.True(t, ok)
				assert.Equal(t, 1, got)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t, tc.build())
		})
	}
}

// TestScopeDeepSeedMapDoesNotOverflow guards the cloneValue depth bound: a seed
// map nested deeper than the internal clone-depth bound must be seeded without a
// stack overflow. 1100 is safely greater than that bound (~1000).
func TestScopeDeepSeedMapDoesNotOverflow(t *testing.T) {
	t.Parallel()

	deep := map[string]any{"leaf": 1}
	for i := 0; i < 1100; i++ {
		deep = map[string]any{"n": deep}
	}

	s := pipe.NewScope(map[string]any{"root": deep})
	_, ok := s.Get("root")
	assert.True(t, ok)
}
