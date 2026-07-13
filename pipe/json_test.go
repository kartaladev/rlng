package pipe_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeJSONRoundTrip(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() *pipe.Scope
		assert func(t *testing.T, blob []byte, reloaded *pipe.Scope)
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []testCase{
		{
			name: "data only, no run, no provenance",
			build: func() *pipe.Scope {
				sc := pipe.NewScope(map[string]any{"a": "x"})
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *pipe.Scope) {
				assert.NotContains(t, string(blob), "timing")
				assert.NotContains(t, string(blob), "derivations")
				v, ok := reloaded.Get("a")
				require.True(t, ok)
				assert.Equal(t, "x", v)
			},
		},
		{
			name: "timing present after a run",
			build: func() *pipe.Scope {
				sc := pipe.NewScope(map[string]any{"a": 1}, pipe.WithClock(fixedClock{t: start}))
				p, _ := pipe.NewPipeline(nil)
				_ = p.Run(context.Background(), sc)
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *pipe.Scope) {
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
			build: func() *pipe.Scope {
				sc := pipe.NewScope(map[string]any{"price": 10}, pipe.WithProvenance())
				require.NoError(t, sc.Derive("base", 20, pipe.Derivation{
					Stage: "base", StageType: pipe.TypeSingleExpr, Operation: "eval",
					Expression: "price * 2", Inputs: map[string]any{"price": 10},
				}))
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *pipe.Scope) {
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
			build: func() *pipe.Scope {
				sc := pipe.NewScope(map[string]any{"a": 1.5, "b": "y"}, pipe.WithClock(fixedClock{t: start}))
				p, _ := pipe.NewPipeline(nil)
				_ = p.Run(context.Background(), sc)
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *pipe.Scope) {
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

			var reloaded pipe.Scope
			require.NoError(t, json.Unmarshal(blob, &reloaded))
			tc.assert(t, blob, &reloaded)
		})
	}
}

// TestScopeJSONRoundTripsRulesetAndFiring covers the Scope JSON envelope's
// ruleset/firing members: present and restored when the Scope was stamped
// and fired rules, omitted from the wire when absent. Same SUT shape as
// TestScopeJSONRoundTrip (build -> marshal -> unmarshal -> assert on blob and
// reloaded), so both scenarios are folded into one table rather than two
// standalone TestXxx functions.
func TestScopeJSONRoundTripsRulesetAndFiring(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func(t *testing.T) *pipe.Scope
		assert func(t *testing.T, blob []byte, reloaded *pipe.Scope)
	}

	cases := []testCase{
		{
			name: "ruleset stamp and firing rules round-trip",
			build: func(t *testing.T) *pipe.Scope {
				tbl, err := pipe.NewDecisionTable("denial", []pipe.Rule{
					{ID: "R1", Message: "too low", Condition: "score < 650", Decisions: map[string]pipe.Decision{"deny": {Expr: "true"}}},
				}, pipe.WithHitPolicy(pipe.HitPolicySingle))
				require.NoError(t, err)
				p, err := pipe.NewPipeline([]pipe.Stage{tbl},
					pipe.WithRuleset(pipe.RulesetIdentity{Hash: "h123", Version: "v1"}))
				require.NoError(t, err)

				sc := pipe.NewScope(map[string]any{"score": 600})
				require.NoError(t, p.Run(t.Context(), sc))
				return sc
			},
			assert: func(t *testing.T, blob []byte, reloaded *pipe.Scope) {
				// Guards the critical constraint: v2 data-value tagging must not
				// disturb the Spec-013 ruleset/firing round-trip riding in the
				// same envelope.
				assert.Contains(t, string(blob), "\"v\":2")

				id, ok := reloaded.Ruleset()
				require.True(t, ok)
				assert.Equal(t, pipe.RulesetIdentity{Hash: "h123", Version: "v1"}, id)

				fired := reloaded.FiringRulesFor("denial")
				require.Len(t, fired, 1)
				assert.Equal(t, "R1", fired[0].RuleID)
				assert.Equal(t, "too low", fired[0].Message)
			},
		},
		{
			name: "absent ruleset and firing are omitted from the wire",
			build: func(t *testing.T) *pipe.Scope {
				return pipe.NewScope(map[string]any{"x": 1})
			},
			assert: func(t *testing.T, blob []byte, reloaded *pipe.Scope) {
				assert.NotContains(t, string(blob), "ruleset")
				assert.NotContains(t, string(blob), "firing")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sc := tc.build(t)

			blob, err := json.Marshal(sc)
			require.NoError(t, err)

			var reloaded pipe.Scope
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

		sc := pipe.NewScope(map[string]any{"cents": big})
		blob, err := json.Marshal(sc)
		require.NoError(t, err)

		var reloaded pipe.Scope
		require.NoError(t, json.Unmarshal(blob, &reloaded))

		got, err := reloaded.GetInt64("cents")
		require.NoError(t, err)
		assert.Equal(t, big, got)
	})

	t.Run("large integer inside a derivation round-trips exactly", func(t *testing.T) {
		t.Parallel()
		const big int64 = 9007199254740993 // 2^53 + 1

		sc := pipe.NewScope(map[string]any{}, pipe.WithProvenance())
		require.NoError(t, sc.Derive("cents", big, pipe.Derivation{
			Stage: "calc", StageType: pipe.TypeSingleExpr, Operation: "eval",
			Expression: "amount", Inputs: map[string]any{"amount": big},
		}))
		blob, err := json.Marshal(sc)
		require.NoError(t, err)

		var reloaded pipe.Scope
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

	t.Run("reloaded int is readable by GetInt; v2 no longer blurs it into a float", func(t *testing.T) {
		t.Parallel()
		sc := pipe.NewScope(map[string]any{"n": 20})
		blob, err := json.Marshal(sc)
		require.NoError(t, err)

		var reloaded pipe.Scope
		require.NoError(t, json.Unmarshal(blob, &reloaded))

		i, err := reloaded.GetInt("n")
		require.NoError(t, err)
		assert.Equal(t, 20, i)

		// Pre-014, a reloaded number was an ambiguous json.Number that both
		// GetInt and GetFloat64 could read. ADR-0038's exact-kind guarantee
		// removes that ambiguity: an int64-kind value is int64, not a value
		// GetFloat64 (which requires float64) can also silently widen.
		_, err = reloaded.GetFloat64("n")
		var typeErr *pipe.ScopeTypeError
		require.ErrorAs(t, err, &typeErr)
		assert.Equal(t, "n", typeErr.Path)
		assert.Equal(t, "float64", typeErr.Expected)
	})

	t.Run("GetInt64 on reloaded non-integer json.Number errors", func(t *testing.T) {
		t.Parallel()
		sc := pipe.NewScope(map[string]any{"r": 1.5})
		blob, err := json.Marshal(sc)
		require.NoError(t, err)

		var reloaded pipe.Scope
		require.NoError(t, json.Unmarshal(blob, &reloaded))

		_, err = reloaded.GetInt64("r")
		var typeErr *pipe.ScopeTypeError
		require.ErrorAs(t, err, &typeErr)
		assert.Equal(t, "r", typeErr.Path)
		assert.Equal(t, "int64", typeErr.Expected)
	})

	t.Run("GetFloat64 on reloaded out-of-range json.Number errors", func(t *testing.T) {
		t.Parallel()
		// A legacy (pre-014, envelope version 0) blob: data values reload as
		// bare json.Number, exercising GetFloat64's json.Number-out-of-range
		// branch directly. (Under v2, a value this far out of float64 range
		// fails at Marshal time instead — see
		// TestScopeJSONEncodeUnrepresentableNumberErrors — because v2 must
		// commit the value to a concrete kind up front rather than deferring
		// the range check to whichever typed getter is called later.)
		legacy := []byte(`{"data":{"huge":1e400}}`)
		var reloaded pipe.Scope
		require.NoError(t, json.Unmarshal(legacy, &reloaded))

		_, err := reloaded.GetFloat64("huge")
		var typeErr *pipe.ScopeTypeError
		require.ErrorAs(t, err, &typeErr)
		assert.Equal(t, "huge", typeErr.Path)
		assert.Equal(t, "float64", typeErr.Expected)
	})
}

// TestScopeJSONEncodeUnrepresentableNumberErrors covers encodeValue's
// json.Number branch failing closed: a value that is neither a valid int64
// nor within float64 range cannot be committed to any of the eight canonical
// kinds, so Marshal returns an error (wrapping ErrMalformedScopeValue) rather
// than silently accepting a value a later typed-getter call would choke on.
func TestScopeJSONEncodeUnrepresentableNumberErrors(t *testing.T) {
	t.Parallel()
	sc := pipe.NewScope(map[string]any{"huge": json.Number("1e400")})
	_, err := json.Marshal(sc)
	require.Error(t, err)
	assert.ErrorIs(t, err, pipe.ErrMalformedScopeValue)
}

// TestScopeGetIntOnReloadedErrors covers GetInt's error branches on a value
// restored from JSON: a non-integer json.Number (e.g. 1.5) and a non-numeric
// value (e.g. a string), both of which must surface as *ScopeTypeError.
func TestScopeGetIntOnReloadedErrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		path   string
		build  func() *pipe.Scope
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name: "non-integer json.Number",
			path: "r",
			build: func() *pipe.Scope {
				sc := pipe.NewScope(map[string]any{"r": 1.5})
				blob, err := json.Marshal(sc)
				require.NoError(t, err)

				var reloaded pipe.Scope
				require.NoError(t, json.Unmarshal(blob, &reloaded))
				return &reloaded
			},
			assert: func(t *testing.T, err error) {
				var typeErr *pipe.ScopeTypeError
				require.ErrorAs(t, err, &typeErr)
				assert.Equal(t, "r", typeErr.Path)
				assert.Equal(t, "int", typeErr.Expected)
			},
		},
		{
			name: "non-numeric value via default branch",
			path: "s",
			build: func() *pipe.Scope {
				sc := pipe.NewScope(map[string]any{"s": "not a number"})
				blob, err := json.Marshal(sc)
				require.NoError(t, err)

				var reloaded pipe.Scope
				require.NoError(t, json.Unmarshal(blob, &reloaded))
				return &reloaded
			},
			assert: func(t *testing.T, err error) {
				var typeErr *pipe.ScopeTypeError
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
		assert func(t *testing.T, s *pipe.Scope, err error)
	}

	cases := []testCase{
		{
			name:  "malformed json is an error",
			input: `{bad`,
			assert: func(t *testing.T, s *pipe.Scope, err error) {
				require.Error(t, err)
			},
		},
		{
			// Structurally valid JSON that dispatches to UnmarshalJSON but fails
			// the inner Decode (timing must be an object, not a number).
			name:  "type-mismatched envelope is a decode error",
			input: `{"data":{},"timing":5}`,
			assert: func(t *testing.T, s *pipe.Scope, err error) {
				require.Error(t, err)
			},
		},
		{
			name:  "absent data yields empty (not nil) map",
			input: `{"timing":{"started_at":"2026-01-01T00:00:00Z","duration_ns":0}}`,
			assert: func(t *testing.T, s *pipe.Scope, err error) {
				require.NoError(t, err)
				assert.NotNil(t, s.Snapshot())
				assert.Empty(t, s.Snapshot())
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var s pipe.Scope
			err := json.Unmarshal([]byte(tc.input), &s)
			tc.assert(t, &s, err)
		})
	}
}

// TestScopeJSONKindRoundTrip covers Spec 014 / ADR-0038's canonical
// type-tagged Scope JSON (v2): each data value kind must reload as itself
// (not as json.Number/map/etc.), including recursively inside a nested
// list/map.
func TestScopeJSONKindRoundTrip(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		set    func(sc *pipe.Scope)
		path   string
		assert func(t *testing.T, v any, ok bool)
	}

	tests := []testCase{
		{
			name: "int64 reloads as int64",
			set:  func(sc *pipe.Scope) { _ = sc.Set("n", int64(9007199254740993)) },
			path: "n",
			assert: func(t *testing.T, v any, ok bool) {
				require.True(t, ok)
				assert.Equal(t, int64(9007199254740993), v)
			},
		},
		{
			name: "decimal reloads as decimal",
			set:  func(sc *pipe.Scope) { _ = sc.Set("fee", decimal.RequireFromString("18125.00")) },
			path: "fee",
			assert: func(t *testing.T, v any, ok bool) {
				require.True(t, ok)
				d, isDec := v.(decimal.Decimal)
				require.True(t, isDec)
				// shopspring/decimal's String() (the wire payload — Task 1's
				// established decimal wire form) trims trailing zeros by
				// design (confirmed against decimal.Decimal.String's own
				// source, and matches the library's own MarshalJSON), so a
				// round trip is mathematically exact but not
				// trailing-zero-verbatim; assert value equality rather than
				// text equality with the pre-trim input.
				assert.True(t, decimal.RequireFromString("18125.00").Equal(d))
				assert.Equal(t, "18125", d.String())
			},
		},
		{
			name: "float64 reloads as float64",
			set:  func(sc *pipe.Scope) { _ = sc.Set("r", 0.0725) },
			path: "r",
			assert: func(t *testing.T, v any, ok bool) {
				require.True(t, ok)
				assert.Equal(t, 0.0725, v)
			},
		},
		{
			name: "nested map + list preserve element kinds",
			set: func(sc *pipe.Scope) {
				_ = sc.Set("box", map[string]any{"amt": decimal.RequireFromString("1.5"), "cnt": int64(2)})
			},
			path: "box",
			assert: func(t *testing.T, v any, ok bool) {
				require.True(t, ok)
				m, isMap := v.(map[string]any)
				require.True(t, isMap)
				assert.IsType(t, decimal.Decimal{}, m["amt"])
				assert.Equal(t, int64(2), m["cnt"])
			},
		},
		{
			name: "list of mixed kinds preserves element kinds",
			set: func(sc *pipe.Scope) {
				_ = sc.Set("items", []any{int64(1), 2.5, "x", decimal.RequireFromString("3.25"), true, nil})
			},
			path: "items",
			assert: func(t *testing.T, v any, ok bool) {
				require.True(t, ok)
				l, isList := v.([]any)
				require.True(t, isList)
				require.Len(t, l, 6)
				assert.Equal(t, int64(1), l[0])
				assert.Equal(t, 2.5, l[1])
				assert.Equal(t, "x", l[2])
				assert.IsType(t, decimal.Decimal{}, l[3])
				assert.Equal(t, true, l[4])
				assert.Nil(t, l[5])
			},
		},
		{
			name: "time.Time reloads as time.Time",
			set: func(sc *pipe.Scope) {
				_ = sc.Set("at", time.Date(2026, 7, 13, 12, 30, 0, 0, time.UTC))
			},
			path: "at",
			assert: func(t *testing.T, v any, ok bool) {
				require.True(t, ok)
				tm, isTime := v.(time.Time)
				require.True(t, isTime)
				assert.True(t, time.Date(2026, 7, 13, 12, 30, 0, 0, time.UTC).Equal(tm))
			},
		},
		{
			name: "bool", set: func(sc *pipe.Scope) { _ = sc.Set("b", true) }, path: "b",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, true, v) },
		},
		{
			name: "string", set: func(sc *pipe.Scope) { _ = sc.Set("s", "hi") }, path: "s",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, "hi", v) },
		},
		{
			name: "nil reloads as nil",
			set:  func(sc *pipe.Scope) { _ = sc.Set("z", nil) },
			path: "z",
			assert: func(t *testing.T, v any, ok bool) {
				require.True(t, ok)
				assert.Nil(t, v)
			},
		},
		// The remaining cases cover every other sized-integer/float Go kind
		// encodeValue accepts (all fold to the int64/float64 wire kind), plus
		// json.Number — the shape a value already round-tripped once through
		// a legacy blob arrives in — re-encoding stably as int64 or float64.
		{
			name: "int8 reloads as int64", set: func(sc *pipe.Scope) { _ = sc.Set("n8", int8(-5)) }, path: "n8",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, int64(-5), v) },
		},
		{
			name: "int16 reloads as int64", set: func(sc *pipe.Scope) { _ = sc.Set("n16", int16(-500)) }, path: "n16",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, int64(-500), v) },
		},
		{
			name: "int32 reloads as int64", set: func(sc *pipe.Scope) { _ = sc.Set("n32", int32(-70000)) }, path: "n32",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, int64(-70000), v) },
		},
		{
			name: "uint reloads as int64", set: func(sc *pipe.Scope) { _ = sc.Set("u", uint(7)) }, path: "u",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, int64(7), v) },
		},
		{
			name: "uint8 reloads as int64", set: func(sc *pipe.Scope) { _ = sc.Set("u8", uint8(200)) }, path: "u8",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, int64(200), v) },
		},
		{
			name: "uint16 reloads as int64", set: func(sc *pipe.Scope) { _ = sc.Set("u16", uint16(50000)) }, path: "u16",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, int64(50000), v) },
		},
		{
			name: "uint32 reloads as int64", set: func(sc *pipe.Scope) { _ = sc.Set("u32", uint32(4000000000)) }, path: "u32",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, int64(4000000000), v) },
		},
		{
			name: "uint64 reloads as int64", set: func(sc *pipe.Scope) { _ = sc.Set("u64", uint64(9000000000000000000)) }, path: "u64",
			assert: func(t *testing.T, v any, ok bool) {
				require.True(t, ok)
				assert.Equal(t, int64(9000000000000000000), v)
			},
		},
		{
			name: "float32 reloads as float64", set: func(sc *pipe.Scope) { _ = sc.Set("f32", float32(1.5)) }, path: "f32",
			assert: func(t *testing.T, v any, ok bool) {
				require.True(t, ok)
				assert.Equal(t, float64(float32(1.5)), v)
			},
		},
		{
			name: "integral json.Number re-encodes stably as int64",
			set:  func(sc *pipe.Scope) { _ = sc.Set("jn", json.Number("42")) }, path: "jn",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, int64(42), v) },
		},
		{
			name: "non-integral json.Number re-encodes stably as float64",
			set:  func(sc *pipe.Scope) { _ = sc.Set("jn", json.Number("1.25")) }, path: "jn",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, 1.25, v) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sc := pipe.NewScope(nil)
			tt.set(sc)
			b, err := json.Marshal(sc)
			require.NoError(t, err)
			var back pipe.Scope
			require.NoError(t, json.Unmarshal(b, &back))
			v, ok := back.Get(tt.path)
			tt.assert(t, v, ok)
		})
	}
}

// TestScopeJSONDeterministic covers Spec 014's determinism requirement: two
// consecutive marshals of the same Scope produce byte-identical output, so a
// tagged v2 blob is safe to hash/diff.
func TestScopeJSONDeterministic(t *testing.T) {
	t.Parallel()
	sc := pipe.NewScope(map[string]any{"b": int64(2), "a": decimal.RequireFromString("1.0")})
	b1, err := json.Marshal(sc)
	require.NoError(t, err)
	b2, err := json.Marshal(sc)
	require.NoError(t, err)
	assert.Equal(t, string(b1), string(b2)) // byte-identical
}

// TestScopeJSONLegacyBlobLoads covers ADR-0038 D3's backward-compatible read
// path: a pre-014 (untagged, envelope version 0) blob still loads, with
// numbers surfacing as json.Number exactly as before this task.
func TestScopeJSONLegacyBlobLoads(t *testing.T) {
	t.Parallel()
	legacy := []byte(`{"data":{"count":3,"name":"x"}}`)
	var sc pipe.Scope
	require.NoError(t, json.Unmarshal(legacy, &sc))
	n, err := sc.GetInt("count")
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	s, _ := sc.GetString("name")
	assert.Equal(t, "x", s)
}

// TestScopeJSONMalformedTaggedValue covers decodeValue's error branch: a v2
// envelope whose data carries a tagged object that is malformed (unknown
// kind, missing payload, or an unparseable payload for its kind) surfaces as
// pipe.ErrMalformedScopeValue rather than a panic or a silently wrong value.
func TestScopeJSONMalformedTaggedValue(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name  string
		input string
	}

	cases := []testCase{
		{
			name:  "unknown kind",
			input: `{"v":2,"data":{"x":{"$k":"bogus","v":1}}}`,
		},
		{
			name:  "missing payload",
			input: `{"v":2,"data":{"x":{"$k":"int64"}}}`,
		},
		{
			name:  "unparseable decimal payload",
			input: `{"v":2,"data":{"x":{"$k":"decimal","v":"not-a-number"}}}`,
		},
		{
			name:  "unparseable time payload",
			input: `{"v":2,"data":{"x":{"$k":"time","v":"not-a-time"}}}`,
		},
		{
			name:  "malformed value nested inside a list",
			input: `{"v":2,"data":{"x":{"$k":"list","v":[{"$k":"bogus","v":1}]}}}`,
		},
		{
			name:  "malformed value nested inside a map",
			input: `{"v":2,"data":{"x":{"$k":"map","v":{"a":{"$k":"bogus","v":1}}}}}`,
		},
		{
			name:  "$k is not a string",
			input: `{"v":2,"data":{"x":{"$k":5,"v":1}}}`,
		},
		{
			name:  "bool payload wrong type",
			input: `{"v":2,"data":{"x":{"$k":"bool","v":"not-a-bool"}}}`,
		},
		{
			name:  "string payload wrong type",
			input: `{"v":2,"data":{"x":{"$k":"string","v":5}}}`,
		},
		{
			name:  "int64 payload wrong type",
			input: `{"v":2,"data":{"x":{"$k":"int64","v":"5"}}}`,
		},
		{
			name:  "int64 payload is a non-integer number",
			input: `{"v":2,"data":{"x":{"$k":"int64","v":1.5}}}`,
		},
		{
			name:  "float64 payload wrong type",
			input: `{"v":2,"data":{"x":{"$k":"float64","v":"5"}}}`,
		},
		{
			name:  "float64 payload out of range",
			input: `{"v":2,"data":{"x":{"$k":"float64","v":1e400}}}`,
		},
		{
			name:  "decimal payload wrong type",
			input: `{"v":2,"data":{"x":{"$k":"decimal","v":5}}}`,
		},
		{
			name:  "time payload wrong type",
			input: `{"v":2,"data":{"x":{"$k":"time","v":5}}}`,
		},
		{
			name:  "list payload wrong type",
			input: `{"v":2,"data":{"x":{"$k":"list","v":5}}}`,
		},
		{
			name:  "map payload wrong type",
			input: `{"v":2,"data":{"x":{"$k":"map","v":5}}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var sc pipe.Scope
			err := json.Unmarshal([]byte(tc.input), &sc)
			require.Error(t, err)
			assert.ErrorIs(t, err, pipe.ErrMalformedScopeValue)
		})
	}
}

// TestScopeJSONV2PassthroughForUntaggedValue covers decodeValue's
// backward-compat fallback: inside an otherwise-v2 envelope, a data value
// that is not itself a type-tagged object (no "$k", including a bare scalar
// or an object missing the tag) loads unchanged rather than erroring.
func TestScopeJSONV2PassthroughForUntaggedValue(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		input  string
		assert func(t *testing.T, sc *pipe.Scope)
	}

	cases := []testCase{
		{
			name:  "bare scalar (no tag object at all)",
			input: `{"v":2,"data":{"x":5}}`,
			assert: func(t *testing.T, sc *pipe.Scope) {
				n, err := sc.GetInt("x")
				require.NoError(t, err)
				assert.Equal(t, 5, n)
			},
		},
		{
			name:  "object missing the $k tag",
			input: `{"v":2,"data":{"x":{"foo":"bar"}}}`,
			assert: func(t *testing.T, sc *pipe.Scope) {
				m, err := sc.GetMap("x")
				require.NoError(t, err)
				assert.Equal(t, "bar", m["foo"])
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var sc pipe.Scope
			require.NoError(t, json.Unmarshal([]byte(tc.input), &sc))
			tc.assert(t, &sc)
		})
	}
}

// TestScopeJSONEncodeIntegerOverflowErrors covers encodeValue's out-of-contract
// guard: a uint/uint64 value exceeding math.MaxInt64 cannot be represented by
// the int64 wire kind (ADR-0038 D1's sole integer kind), so Marshal fails
// closed with ErrMalformedScopeValue rather than silently truncating or
// wrapping. It also covers the json.Number branch of the same guard: an
// integer-format json.Number that overflows int64 (e.g. a large ID surviving
// a legacy v0 blob) must fail closed too, rather than silently downgrading to
// a lossy float64 approximation (the review-round-1 fix for Task 3).
func TestScopeJSONEncodeIntegerOverflowErrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		set  func(sc *pipe.Scope)
	}

	cases := []testCase{
		{name: "uint exceeding math.MaxInt64", set: func(sc *pipe.Scope) { _ = sc.Set("x", uint(math.MaxInt64)+1) }},
		{name: "uint64 exceeding math.MaxInt64", set: func(sc *pipe.Scope) { _ = sc.Set("x", uint64(math.MaxInt64)+1) }},
		{name: "json.Number integer exceeding int64 range", set: func(sc *pipe.Scope) { _ = sc.Set("x", json.Number("99999999999999999999")) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sc := pipe.NewScope(nil)
			tc.set(sc)
			_, err := json.Marshal(sc)
			require.Error(t, err)
			assert.ErrorIs(t, err, pipe.ErrMalformedScopeValue)
		})
	}
}

// unsupportedScopeValue is a Go type with no encodeValue kind mapping, used to
// exercise encodeValue's default (unsupported-type) branch and its error
// propagation out of the list/map recursion.
type unsupportedScopeValue struct{ X int }

// TestScopeJSONEncodeUnsupportedTypeErrors covers encodeValue's default
// branch (a Go type outside the eight canonical kinds) both at the top level
// and propagating an error out of the list/map recursion, rather than
// silently dropping or mis-tagging the value.
func TestScopeJSONEncodeUnsupportedTypeErrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		set  func(sc *pipe.Scope)
	}

	cases := []testCase{
		{
			name: "unsupported type at top level",
			set:  func(sc *pipe.Scope) { _ = sc.Set("x", unsupportedScopeValue{X: 1}) },
		},
		{
			name: "unsupported type nested inside a list",
			set:  func(sc *pipe.Scope) { _ = sc.Set("x", []any{1, unsupportedScopeValue{X: 1}}) },
		},
		{
			name: "unsupported type nested inside a map",
			set:  func(sc *pipe.Scope) { _ = sc.Set("x", map[string]any{"a": unsupportedScopeValue{X: 1}}) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sc := pipe.NewScope(nil)
			tc.set(sc)
			_, err := json.Marshal(sc)
			require.Error(t, err)
			assert.ErrorIs(t, err, pipe.ErrMalformedScopeValue)
		})
	}
}

// TestScopeJSONEncodeNonFiniteFloatErrors covers encodeTagged's
// json.Marshal-failure branch, reached naturally when a float64/float32
// value is NaN or +/-Inf: encoding/json itself rejects non-finite floats, so
// Marshal must fail closed (wrapped as ErrMalformedScopeValue) rather than
// produce invalid JSON.
func TestScopeJSONEncodeNonFiniteFloatErrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		set  func(sc *pipe.Scope)
	}

	cases := []testCase{
		{name: "NaN", set: func(sc *pipe.Scope) { _ = sc.Set("x", math.NaN()) }},
		{name: "+Inf", set: func(sc *pipe.Scope) { _ = sc.Set("x", math.Inf(1)) }},
		{name: "-Inf", set: func(sc *pipe.Scope) { _ = sc.Set("x", math.Inf(-1)) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sc := pipe.NewScope(nil)
			tc.set(sc)
			_, err := json.Marshal(sc)
			require.Error(t, err)
			assert.ErrorIs(t, err, pipe.ErrMalformedScopeValue)
		})
	}
}

// TestScopeJSONDecimalScalePreserved guards the whole-branch review finding: a
// decimal's scale (trailing zeros / exponent) must survive the round-trip, so a
// money value of 18125.00 does not silently reload as 18125.
func TestScopeJSONDecimalScalePreserved(t *testing.T) {
	t.Parallel()

	sc := pipe.NewScope(nil)
	require.NoError(t, sc.Set("fee", decimal.RequireFromString("18125.00")))

	blob, err := json.Marshal(sc)
	require.NoError(t, err)
	assert.Contains(t, string(blob), "18125.00") // scale present in the wire form

	var back pipe.Scope
	require.NoError(t, json.Unmarshal(blob, &back))
	d, err := pipe.GetAs[decimal.Decimal](&back, "fee")
	require.NoError(t, err)
	assert.Equal(t, int32(-2), d.Exponent(), "scale (exponent) must be preserved")
	assert.Equal(t, "18125.00", d.StringFixed(2))
}
