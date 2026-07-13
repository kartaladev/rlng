package pipe_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/require"
)

func TestGetAs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		seed   map[string]any
		path   string
		call   func(s *pipe.Scope) (any, error)
		assert func(*testing.T, any, error)
	}{
		{
			name: "GetInt happy path",
			seed: map[string]any{"count": int(42)},
			path: "count",
			call: func(s *pipe.Scope) (any, error) { return s.GetInt("count") },
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
			call: func(s *pipe.Scope) (any, error) { return s.GetString("absent") },
			assert: func(t *testing.T, v any, err error) {
				require.ErrorIs(t, err, pipe.ErrPathNotFound)
			},
		},
		{
			// A v2 Scope JSON round-trip (ADR-0038) reloads every sized
			// integer as int64 (the sole canonical integer kind); GetInt must
			// still widen it back to int losslessly.
			name: "GetInt on an int64 value (v2 reload kind) widens losslessly",
			seed: map[string]any{"count": int64(42)},
			path: "count",
			call: func(s *pipe.Scope) (any, error) { return s.GetInt("count") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.(int)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, 42, val)
			},
		},
		{
			name: "GetInt64 happy path",
			seed: map[string]any{"bigcount": int64(9223372036854775807)},
			path: "bigcount",
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64("bigcount") },
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
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64("pi") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.(float64)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, 3.14159, val)
			},
		},
		{
			// A json.Number is the shape GetInt64 sees reading a legacy
			// (pre-014, untagged) Scope JSON blob.
			name: "GetInt64 on a valid-integer json.Number",
			seed: map[string]any{"n": json.Number("123")},
			path: "n",
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64("n") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.(int64)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, int64(123), val)
			},
		},
		{
			name: "GetInt64 on a non-integer json.Number errors",
			seed: map[string]any{"n": json.Number("1.5")},
			path: "n",
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64("n") },
			assert: func(t *testing.T, v any, err error) {
				var typeErr *pipe.ScopeTypeError
				require.ErrorAs(t, err, &typeErr)
				require.Equal(t, "n", typeErr.Path)
				require.Equal(t, "int64", typeErr.Expected)
				require.Equal(t, "json.Number(1.5)", typeErr.Actual)
			},
		},
		{
			// Same rationale as the GetInt64 case above: json.Number is the
			// shape GetFloat64 sees reading a legacy Scope JSON blob.
			name: "GetFloat64 on a valid json.Number",
			seed: map[string]any{"n": json.Number("3.14")},
			path: "n",
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64("n") },
			assert: func(t *testing.T, v any, err error) {
				val, ok := v.(float64)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, 3.14, val)
			},
		},
		{
			name: "GetString happy path",
			seed: map[string]any{"name": "Alice"},
			path: "name",
			call: func(s *pipe.Scope) (any, error) { return s.GetString("name") },
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
			call: func(s *pipe.Scope) (any, error) { return s.GetBool("active") },
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
			call: func(s *pipe.Scope) (any, error) { return s.GetSlice("items") },
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
			call: func(s *pipe.Scope) (any, error) { return s.GetMap("config") },
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
			call: func(s *pipe.Scope) (any, error) { return s.GetInt("missing") },
			assert: func(t *testing.T, v any, err error) {
				require.ErrorIs(t, err, pipe.ErrPathNotFound)
			},
		},
		{
			name: "GetInt64 missing path returns ErrPathNotFound",
			seed: map[string]any{"a": int64(1)},
			path: "missing",
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64("missing") },
			assert: func(t *testing.T, v any, err error) {
				require.ErrorIs(t, err, pipe.ErrPathNotFound)
			},
		},
		{
			name: "GetInt64 wrong type returns ScopeTypeError",
			seed: map[string]any{"value": "string_value"},
			path: "value",
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64("value") },
			assert: func(t *testing.T, v any, err error) {
				var typeErr *pipe.ScopeTypeError
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
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64("missing") },
			assert: func(t *testing.T, v any, err error) {
				require.ErrorIs(t, err, pipe.ErrPathNotFound)
			},
		},
		{
			name: "GetFloat64 wrong type returns ScopeTypeError",
			seed: map[string]any{"value": "string_value"},
			path: "value",
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64("value") },
			assert: func(t *testing.T, v any, err error) {
				var typeErr *pipe.ScopeTypeError
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
			call: func(s *pipe.Scope) (any, error) { return s.GetInt("value") },
			assert: func(t *testing.T, v any, err error) {
				var typeErr *pipe.ScopeTypeError
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
			call: func(s *pipe.Scope) (any, error) { return s.GetString("empty") },
			assert: func(t *testing.T, v any, err error) {
				var typeErr *pipe.ScopeTypeError
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
			call: func(s *pipe.Scope) (any, error) { return s.GetInt("value") },
			assert: func(t *testing.T, v any, err error) {
				var typeErr *pipe.ScopeTypeError
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
			call: func(s *pipe.Scope) (any, error) { return s.GetSlice("data") },
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
			s := pipe.NewScope(tc.seed)

			got, err := tc.call(s)
			tc.assert(t, got, err)
		})
	}
}

// TestGetCoerce exercises the coercing numeric getters (Spec 019). Each case
// drives one method through the public API; the assert closure checks the
// coerced value or the typed error (*ScopeTypeError / ErrPathNotFound).
func TestGetCoerce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		seed   map[string]any
		call   func(s *pipe.Scope) (any, error)
		assert func(*testing.T, any, error)
	}{
		// --- GetIntCoerce ---
		{
			name: "GetIntCoerce int fits",
			seed: map[string]any{"v": int(7)},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, 7, v)
			},
		},
		{
			name: "GetIntCoerce int64 fits",
			seed: map[string]any{"v": int64(7)},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, 7, v)
			},
		},
		{
			name: "GetIntCoerce uint64 fits",
			seed: map[string]any{"v": uint64(7)},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, 7, v)
			},
		},
		{
			name: "GetIntCoerce uint64 above MaxInt64 overflows",
			seed: map[string]any{"v": uint64(math.MaxUint64)},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
				require.Equal(t, "int", te.Expected)
			},
		},
		{
			name: "GetIntCoerce integral float64",
			seed: map[string]any{"v": float64(3)},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, 3, v)
			},
		},
		{
			name: "GetIntCoerce non-integral float64 errors",
			seed: map[string]any{"v": 3.14},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
				require.Equal(t, "int", te.Expected)
			},
		},
		{
			name: "GetIntCoerce NaN float errors",
			seed: map[string]any{"v": math.NaN()},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
			},
		},
		{
			name: "GetIntCoerce +Inf float errors",
			seed: map[string]any{"v": math.Inf(1)},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
			},
		},
		{
			name: "GetIntCoerce json.Number integer",
			seed: map[string]any{"v": json.Number("42")},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, 42, v)
			},
		},
		{
			name: "GetIntCoerce json.Number non-integer errors",
			seed: map[string]any{"v": json.Number("1.5")},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
			},
		},
		{
			name: "GetIntCoerce numeric string",
			seed: map[string]any{"v": "42"},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, 42, v)
			},
		},
		{
			name: "GetIntCoerce non-numeric string errors",
			seed: map[string]any{"v": "abc"},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
			},
		},
		{
			name: "GetIntCoerce bool errors",
			seed: map[string]any{"v": true},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
			},
		},
		{
			name: "GetIntCoerce missing path",
			seed: map[string]any{},
			call: func(s *pipe.Scope) (any, error) { return s.GetIntCoerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.ErrorIs(t, err, pipe.ErrPathNotFound)
			},
		},
		// --- GetInt64Coerce ---
		{
			name: "GetInt64Coerce large int64",
			seed: map[string]any{"v": int64(9000000000000000000)},
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, int64(9000000000000000000), v)
			},
		},
		{
			name: "GetInt64Coerce uint64 overflow errors",
			seed: map[string]any{"v": uint64(math.MaxUint64)},
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
				require.Equal(t, "int64", te.Expected)
			},
		},
		{
			name: "GetInt64Coerce integral float64",
			seed: map[string]any{"v": float64(100)},
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, int64(100), v)
			},
		},
		{
			name: "GetInt64Coerce string",
			seed: map[string]any{"v": "123"},
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, int64(123), v)
			},
		},
		{
			name: "GetInt64Coerce nil errors",
			seed: map[string]any{"v": nil},
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
			},
		},
		{
			name: "GetInt64Coerce missing path",
			seed: map[string]any{},
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.ErrorIs(t, err, pipe.ErrPathNotFound)
			},
		},
		{
			name: "GetInt64Coerce integral float overflowing int64 errors",
			seed: map[string]any{"v": 1e19},
			call: func(s *pipe.Scope) (any, error) { return s.GetInt64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
				require.Equal(t, "int64", te.Expected)
			},
		},
		// --- GetFloat64Coerce ---
		{
			name: "GetFloat64Coerce float64 pass-through",
			seed: map[string]any{"v": 3.14},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, 3.14, v)
			},
		},
		{
			name: "GetFloat64Coerce stored NaN passes through",
			seed: map[string]any{"v": math.NaN()},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				f, ok := v.(float64)
				require.True(t, ok)
				require.True(t, math.IsNaN(f))
			},
		},
		{
			name: "GetFloat64Coerce int widens",
			seed: map[string]any{"v": int(5)},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, float64(5), v)
			},
		},
		{
			name: "GetFloat64Coerce uint64 widens",
			seed: map[string]any{"v": uint64(5)},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, float64(5), v)
			},
		},
		{
			name: "GetFloat64Coerce json.Number",
			seed: map[string]any{"v": json.Number("3.14")},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, 3.14, v)
			},
		},
		{
			name: "GetFloat64Coerce non-finite json.Number errors",
			seed: map[string]any{"v": json.Number("NaN")},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
				require.Equal(t, "float64", te.Expected)
			},
		},
		{
			name: "GetFloat64Coerce invalid json.Number errors",
			seed: map[string]any{"v": json.Number("abc")},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
				require.Equal(t, "float64", te.Expected)
			},
		},
		{
			name: "GetFloat64Coerce numeric string",
			seed: map[string]any{"v": "3.14"},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.NoError(t, err)
				require.Equal(t, 3.14, v)
			},
		},
		{
			name: "GetFloat64Coerce non-numeric string errors",
			seed: map[string]any{"v": "abc"},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
				require.Equal(t, "float64", te.Expected)
			},
		},
		{
			name: "GetFloat64Coerce NaN string errors",
			seed: map[string]any{"v": "NaN"},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
			},
		},
		{
			name: "GetFloat64Coerce Inf string errors",
			seed: map[string]any{"v": "Inf"},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
			},
		},
		{
			name: "GetFloat64Coerce bool errors",
			seed: map[string]any{"v": true},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				var te *pipe.ScopeTypeError
				require.ErrorAs(t, err, &te)
			},
		},
		{
			name: "GetFloat64Coerce missing path",
			seed: map[string]any{},
			call: func(s *pipe.Scope) (any, error) { return s.GetFloat64Coerce("v") },
			assert: func(t *testing.T, v any, err error) {
				require.ErrorIs(t, err, pipe.ErrPathNotFound)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := pipe.NewScope(tc.seed)
			got, err := tc.call(s)
			tc.assert(t, got, err)
		})
	}
}

func BenchmarkGetInt(b *testing.B) {
	s := pipe.NewScope(map[string]any{"count": int(42)})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.GetInt("count")
	}
}

func BenchmarkGetString(b *testing.B) {
	s := pipe.NewScope(map[string]any{"name": "Alice"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.GetString("name")
	}
}
