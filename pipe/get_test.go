package stage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetAs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		seed   map[string]any
		path   string
		call   func(s *Scope) (any, error)
		assert func(*testing.T, any, error)
	}{
		{
			name: "GetInt happy path",
			seed: map[string]any{"count": int(42)},
			path: "count",
			call: func(s *Scope) (any, error) { return s.GetInt("count") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.(int)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, 42, val)
			},
		},
		{
			name: "GetString missing path returns ErrPathNotFound",
			seed: map[string]any{},
			path: "absent",
			call: func(s *Scope) (any, error) { return s.GetString("absent") },
			assert: func(t *testing.T, v any, err error) {
				require.ErrorIs(t, err, ErrPathNotFound)
			},
		},
		{
			name: "GetInt64 happy path",
			seed: map[string]any{"bigcount": int64(9223372036854775807)},
			path: "bigcount",
			call: func(s *Scope) (any, error) { return s.GetInt64("bigcount") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.(int64)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, int64(9223372036854775807), val)
			},
		},
		{
			name: "GetFloat64 happy path",
			seed: map[string]any{"pi": float64(3.14159)},
			path: "pi",
			call: func(s *Scope) (any, error) { return s.GetFloat64("pi") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.(float64)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, 3.14159, val)
			},
		},
		{
			name: "GetString happy path",
			seed: map[string]any{"name": "Alice"},
			path: "name",
			call: func(s *Scope) (any, error) { return s.GetString("name") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.(string)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, "Alice", val)
			},
		},
		{
			name: "GetBool happy path",
			seed: map[string]any{"active": true},
			path: "active",
			call: func(s *Scope) (any, error) { return s.GetBool("active") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.(bool)
				require.NoError(t, err)
				require.True(t, ok)
				require.True(t, val)
			},
		},
		{
			name: "GetSlice happy path",
			seed: map[string]any{"items": []any{1, "two", 3.0}},
			path: "items",
			call: func(s *Scope) (any, error) { return s.GetSlice("items") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.([]any)
				require.NoError(t, err)
				require.True(t, ok)
				require.Len(t, val, 3)
				require.Equal(t, 1, val[0])
				require.Equal(t, "two", val[1])
				require.Equal(t, 3.0, val[2])
			},
		},
		{
			name: "GetMap happy path",
			seed: map[string]any{"config": map[string]any{"key": "value"}},
			path: "config",
			call: func(s *Scope) (any, error) { return s.GetMap("config") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.(map[string]any)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, "value", val["key"])
			},
		},
		{
			name: "missing path returns ErrPathNotFound",
			seed: map[string]any{"a": 1},
			path: "missing",
			call: func(s *Scope) (any, error) { return s.GetInt("missing") },
			assert: func(t *testing.T, v any, err error) {
				require.ErrorIs(t, err, ErrPathNotFound)
			},
		},
		{
			name: "GetInt64 missing path returns ErrPathNotFound",
			seed: map[string]any{"a": int64(1)},
			path: "missing",
			call: func(s *Scope) (any, error) { return s.GetInt64("missing") },
			assert: func(t *testing.T, v any, err error) {
				require.ErrorIs(t, err, ErrPathNotFound)
			},
		},
		{
			name: "GetInt64 wrong type returns ScopeTypeError",
			seed: map[string]any{"value": "string_value"},
			path: "value",
			call: func(s *Scope) (any, error) { return s.GetInt64("value") },
			assert: func(t *testing.T, v any, err error) {
				var typeErr *ScopeTypeError
				require.ErrorAs(t, err, &typeErr)
				require.Equal(t, "value", typeErr.Path)
				require.Equal(t, "int64", typeErr.Expected)
				require.Equal(t, "string", typeErr.Actual)
			},
		},
		{
			name: "GetFloat64 missing path returns ErrPathNotFound",
			seed: map[string]any{"a": float64(1)},
			path: "missing",
			call: func(s *Scope) (any, error) { return s.GetFloat64("missing") },
			assert: func(t *testing.T, v any, err error) {
				require.ErrorIs(t, err, ErrPathNotFound)
			},
		},
		{
			name: "GetFloat64 wrong type returns ScopeTypeError",
			seed: map[string]any{"value": "string_value"},
			path: "value",
			call: func(s *Scope) (any, error) { return s.GetFloat64("value") },
			assert: func(t *testing.T, v any, err error) {
				var typeErr *ScopeTypeError
				require.ErrorAs(t, err, &typeErr)
				require.Equal(t, "value", typeErr.Path)
				require.Equal(t, "float64", typeErr.Expected)
				require.Equal(t, "string", typeErr.Actual)
			},
		},
		{
			name: "wrong type returns ScopeTypeError",
			seed: map[string]any{"value": "string_value"},
			path: "value",
			call: func(s *Scope) (any, error) { return s.GetInt("value") },
			assert: func(t *testing.T, v any, err error) {
				var typeErr *ScopeTypeError
				require.ErrorAs(t, err, &typeErr)
				require.Equal(t, "value", typeErr.Path)
				require.Equal(t, "int", typeErr.Expected)
				require.Equal(t, "string", typeErr.Actual)
				require.Equal(t, `scope: path "value": expected int, got string`, typeErr.Error())
			},
		},
		{
			name: "stored nil value",
			seed: map[string]any{"empty": nil},
			path: "empty",
			call: func(s *Scope) (any, error) { return s.GetString("empty") },
			assert: func(t *testing.T, v any, err error) {
				var typeErr *ScopeTypeError
				require.ErrorAs(t, err, &typeErr)
				require.Equal(t, "empty", typeErr.Path)
				require.Equal(t, "string", typeErr.Expected)
				require.Equal(t, "<nil>", typeErr.Actual)
			},
		},
		{
			name: "float64 cannot be coerced to int",
			seed: map[string]any{"value": 3.14},
			path: "value",
			call: func(s *Scope) (any, error) { return s.GetInt("value") },
			assert: func(t *testing.T, v any, err error) {
				var typeErr *ScopeTypeError
				require.ErrorAs(t, err, &typeErr)
				require.Equal(t, "value", typeErr.Path)
				require.Equal(t, "int", typeErr.Expected)
				require.Equal(t, "float64", typeErr.Actual)
			},
		},
		{
			name: "GetAs with []any for slice",
			seed: map[string]any{"data": []any{"a", "b", "c"}},
			path: "data",
			call: func(s *Scope) (any, error) { return s.GetSlice("data") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.([]any)
				require.NoError(t, err)
				require.True(t, ok)
				require.Len(t, val, 3)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := NewScope(tc.seed)

			got, err := tc.call(s)
			tc.assert(t, got, err)
		})
	}
}

func BenchmarkGetInt(b *testing.B) {
	s := NewScope(map[string]any{"count": int(42)})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.GetInt("count")
	}
}

func BenchmarkGetString(b *testing.B) {
	s := NewScope(map[string]any{"name": "Alice"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.GetString("name")
	}
}
