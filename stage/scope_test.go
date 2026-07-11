package stage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeSetGet(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() *Scope
		assert func(t *testing.T, s *Scope)
	}

	cases := []testCase{
		{
			name:  "single-segment set and get",
			build: func() *Scope { return NewScope(nil) },
			assert: func(t *testing.T, s *Scope) {
				require.NoError(t, s.Set("amount", 100))
				got, ok := s.Get("amount")
				require.True(t, ok)
				assert.Equal(t, 100, got)
			},
		},
		{
			name:  "dot-path creates nested maps",
			build: func() *Scope { return NewScope(nil) },
			assert: func(t *testing.T, s *Scope) {
				require.NoError(t, s.Set("discount.rate", 0.1))
				got, ok := s.Get("discount.rate")
				require.True(t, ok)
				assert.Equal(t, 0.1, got)
			},
		},
		{
			name:  "seed is read via get",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *Scope) {
				got, ok := s.Get("a")
				require.True(t, ok)
				assert.Equal(t, 1, got)
			},
		},
		{
			name:  "missing path returns false",
			build: func() *Scope { return NewScope(nil) },
			assert: func(t *testing.T, s *Scope) {
				_, ok := s.Get("nope")
				assert.False(t, ok)
			},
		},
		{
			name:  "get with empty path returns false",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *Scope) {
				_, ok := s.Get("")
				assert.False(t, ok)
			},
		},
		{
			name:  "get descending through a scalar returns false",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *Scope) {
				_, ok := s.Get("a.b")
				assert.False(t, ok)
			},
		},
		{
			name:  "empty path is an error",
			build: func() *Scope { return NewScope(nil) },
			assert: func(t *testing.T, s *Scope) {
				require.ErrorIs(t, s.Set("", 1), errEmptyPath)
			},
		},
		{
			name:  "descend through scalar errors",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *Scope) {
				require.ErrorIs(t, s.Set("a.b", 2), ErrPathNotMap)
			},
		},
		{
			name:  "lenient overwrite (default) wins last",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *Scope) {
				require.NoError(t, s.Set("a", 2))
				got, _ := s.Get("a")
				assert.Equal(t, 2, got)
			},
		},
		{
			name:  "strict overwrite conflicts",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}, WithStrict()) },
			assert: func(t *testing.T, s *Scope) {
				require.ErrorIs(t, s.Set("a", 2), ErrPathConflict)
			},
		},
		{
			name:  "snapshot is decoupled from later writes",
			build: func() *Scope { return NewScope(map[string]any{"a": 1}) },
			assert: func(t *testing.T, s *Scope) {
				snap := s.Snapshot()
				require.NoError(t, s.Set("b", 2))
				_, ok := snap["b"]
				assert.False(t, ok, "snapshot must not see writes made after it was taken")
			},
		},
		{
			name:  "seed copy protects against caller mutation",
			build: func() *Scope { return nil }, // built inline below
			assert: func(t *testing.T, _ *Scope) {
				seed := map[string]any{"a": 1}
				s := NewScope(seed)
				seed["a"] = 999
				got, _ := s.Get("a")
				assert.Equal(t, 1, got, "scope must not observe caller's post-construction seed mutation")
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
