package stage

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeJSONRoundTrip(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() *Scope
		assert func(t *testing.T, blob []byte, reloaded *Scope)
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fixedClock := func() time.Time { return start }

	cases := []testCase{
		{
			name: "data only, no run, no provenance",
			build: func() *Scope {
				sc := NewScope(map[string]any{"a": "x"})
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *Scope) {
				assert.NotContains(t, string(blob), "timing")
				assert.NotContains(t, string(blob), "derivations")
				v, ok := reloaded.Get("a")
				require.True(t, ok)
				assert.Equal(t, "x", v)
			},
		},
		{
			name: "timing present after a run",
			build: func() *Scope {
				sc := NewScope(map[string]any{"a": 1}, WithClock(fixedClock))
				sc.markStarted()
				sc.markFinished()
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *Scope) {
				assert.Contains(t, string(blob), "\"timing\"")
				at, ok := reloaded.StartedAt()
				require.True(t, ok)
				assert.Equal(t, start.UTC(), at.UTC())
				d, ok := reloaded.Duration()
				require.True(t, ok)
				assert.Equal(t, time.Duration(0), d)
			},
		},
		{
			name: "provenance derivations round-trip and restore inspection",
			build: func() *Scope {
				sc := NewScope(map[string]any{"price": 10}, WithProvenance())
				require.NoError(t, sc.Derive("base", 20, Derivation{
					Stage: "base", StageType: TypeSingleExpr, Operation: "eval",
					Expression: "price * 2", Inputs: map[string]any{"price": 10},
				}))
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *Scope) {
				assert.Contains(t, string(blob), "\"derivations\"")
				assert.True(t, reloaded.TracksProvenance())
				d, ok := reloaded.Derivation("base")
				require.True(t, ok)
				assert.Equal(t, "price * 2", d.Expression)
				assert.NotEmpty(t, reloaded.Explain("base"))
			},
		},
		{
			name: "byte-stable round-trip (marshal->unmarshal->marshal)",
			build: func() *Scope {
				sc := NewScope(map[string]any{"a": 1.5, "b": "y"}, WithClock(fixedClock))
				sc.markStarted()
				sc.markFinished()
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *Scope) {
				again, err := json.Marshal(reloaded)
				require.NoError(t, err)
				assert.JSONEq(t, string(blob), string(again))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sc := tc.build()
			blob, err := json.Marshal(sc)
			require.NoError(t, err)

			var reloaded Scope
			require.NoError(t, json.Unmarshal(blob, &reloaded))
			tc.assert(t, blob, &reloaded)
		})
	}
}

func TestScopeJSONValuePreservation(t *testing.T) {
	t.Parallel()

	t.Run("large integer above 2^53 round-trips exactly", func(t *testing.T) {
		t.Parallel()
		const big int64 = 9007199254740993 // 2^53 + 1

		// Document the bug this guards against: plain float64 round-tripping
		// through JSON silently rounds this value down.
		assert.NotEqual(t, big, int64(float64(big)))
		assert.Equal(t, float64(9007199254740992), float64(big))

		sc := NewScope(map[string]any{"cents": big})
		blob, err := json.Marshal(sc)
		require.NoError(t, err)

		var reloaded Scope
		require.NoError(t, json.Unmarshal(blob, &reloaded))

		got, err := reloaded.GetInt64("cents")
		require.NoError(t, err)
		assert.Equal(t, big, got)
	})

	t.Run("large integer inside a derivation round-trips exactly", func(t *testing.T) {
		t.Parallel()
		const big int64 = 9007199254740993 // 2^53 + 1

		sc := NewScope(map[string]any{}, WithProvenance())
		require.NoError(t, sc.Derive("cents", big, Derivation{
			Stage: "calc", StageType: TypeSingleExpr, Operation: "eval",
			Expression: "amount", Inputs: map[string]any{"amount": big},
		}))
		blob, err := json.Marshal(sc)
		require.NoError(t, err)

		var reloaded Scope
		require.NoError(t, json.Unmarshal(blob, &reloaded))

		// data value exact.
		got, err := reloaded.GetInt64("cents")
		require.NoError(t, err)
		assert.Equal(t, big, got)

		// derivation Value + Inputs exact (json.Number after reload).
		d, ok := reloaded.Derivation("cents")
		require.True(t, ok)
		gotVal, err := d.Value.(json.Number).Int64()
		require.NoError(t, err)
		assert.Equal(t, big, gotVal)
		gotIn, err := d.Inputs["amount"].(json.Number).Int64()
		require.NoError(t, err)
		assert.Equal(t, big, gotIn)
	})

	t.Run("reloaded int is readable by GetInt and GetFloat64", func(t *testing.T) {
		t.Parallel()
		sc := NewScope(map[string]any{"n": 20})
		blob, err := json.Marshal(sc)
		require.NoError(t, err)

		var reloaded Scope
		require.NoError(t, json.Unmarshal(blob, &reloaded))

		i, err := reloaded.GetInt("n")
		require.NoError(t, err)
		assert.Equal(t, 20, i)

		f, err := reloaded.GetFloat64("n")
		require.NoError(t, err)
		assert.Equal(t, 20.0, f)
	})

	t.Run("GetInt64 on reloaded non-integer json.Number errors", func(t *testing.T) {
		t.Parallel()
		sc := NewScope(map[string]any{"r": 1.5})
		blob, err := json.Marshal(sc)
		require.NoError(t, err)

		var reloaded Scope
		require.NoError(t, json.Unmarshal(blob, &reloaded))

		_, err = reloaded.GetInt64("r")
		var typeErr *ScopeTypeError
		require.ErrorAs(t, err, &typeErr)
		assert.Equal(t, "r", typeErr.Path)
		assert.Equal(t, "int64", typeErr.Expected)
	})

	t.Run("GetFloat64 on reloaded out-of-range json.Number errors", func(t *testing.T) {
		t.Parallel()
		sc := NewScope(map[string]any{"huge": json.Number("1e400")})
		blob, err := json.Marshal(sc)
		require.NoError(t, err)

		var reloaded Scope
		require.NoError(t, json.Unmarshal(blob, &reloaded))

		_, err = reloaded.GetFloat64("huge")
		var typeErr *ScopeTypeError
		require.ErrorAs(t, err, &typeErr)
		assert.Equal(t, "huge", typeErr.Path)
		assert.Equal(t, "float64", typeErr.Expected)
	})
}

// TestScopeGetIntOnReloadedErrors covers GetInt's error branches on a value
// restored from JSON: a non-integer json.Number (e.g. 1.5) and a non-numeric
// value (e.g. a string), both of which must surface as *ScopeTypeError.
func TestScopeGetIntOnReloadedErrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		path   string
		build  func() *Scope
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name: "non-integer json.Number",
			path: "r",
			build: func() *Scope {
				sc := NewScope(map[string]any{"r": 1.5})
				blob, err := json.Marshal(sc)
				require.NoError(t, err)

				var reloaded Scope
				require.NoError(t, json.Unmarshal(blob, &reloaded))
				return &reloaded
			},
			assert: func(t *testing.T, err error) {
				var typeErr *ScopeTypeError
				require.ErrorAs(t, err, &typeErr)
				assert.Equal(t, "r", typeErr.Path)
				assert.Equal(t, "int", typeErr.Expected)
			},
		},
		{
			name: "non-numeric value via default branch",
			path: "s",
			build: func() *Scope {
				sc := NewScope(map[string]any{"s": "not a number"})
				blob, err := json.Marshal(sc)
				require.NoError(t, err)

				var reloaded Scope
				require.NoError(t, json.Unmarshal(blob, &reloaded))
				return &reloaded
			},
			assert: func(t *testing.T, err error) {
				var typeErr *ScopeTypeError
				require.ErrorAs(t, err, &typeErr)
				assert.Equal(t, "string", typeErr.Actual)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sc := tc.build()

			_, err := sc.GetInt(tc.path)
			tc.assert(t, err)
		})
	}
}

func TestScopeUnmarshalErrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		input  string
		assert func(t *testing.T, s *Scope, err error)
	}

	cases := []testCase{
		{
			name:  "malformed json is an error",
			input: `{bad`,
			assert: func(t *testing.T, s *Scope, err error) {
				require.Error(t, err)
			},
		},
		{
			// Structurally valid JSON that dispatches to UnmarshalJSON but fails
			// the inner Decode (timing must be an object, not a number).
			name:  "type-mismatched envelope is a decode error",
			input: `{"data":{},"timing":5}`,
			assert: func(t *testing.T, s *Scope, err error) {
				require.Error(t, err)
			},
		},
		{
			name:  "absent data yields empty (not nil) map",
			input: `{"timing":{"started_at":"2026-01-01T00:00:00Z","duration_ns":0}}`,
			assert: func(t *testing.T, s *Scope, err error) {
				require.NoError(t, err)
				assert.NotNil(t, s.Snapshot())
				assert.Empty(t, s.Snapshot())
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var s Scope
			err := json.Unmarshal([]byte(tc.input), &s)
			tc.assert(t, &s, err)
		})
	}
}
