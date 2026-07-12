package rlng_test

import (
	"context"
	"sync"
	"testing"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/pipe"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type engineInput struct {
	Price int `mapstructure:"price"`
	Qty   int `mapstructure:"qty"`
}

func buildEngine(tb testing.TB, opts ...rlng.Option) *rlng.Engine {
	tb.Helper()
	base, err := pipe.NewSingleExpr("base", "price * qty")
	require.NoError(tb, err)
	taxed, err := pipe.NewSingleExpr("taxed", "base * 1.1", pipe.WithDependsOn("base"))
	require.NoError(tb, err)
	p, err := pipe.NewPipeline(base, taxed)
	require.NoError(tb, err)
	e, err := rlng.New(p, opts...)
	require.NoError(tb, err)
	return e
}

func TestEngineEvaluate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		engine func(tb testing.TB) *rlng.Engine
		input  any
		ctx    func(ctx context.Context) context.Context
		assert func(t *testing.T, out map[string]any, err error)
	}

	cases := []testCase{
		{
			name:   "struct input returns accumulated map",
			engine: func(tb testing.TB) *rlng.Engine { return buildEngine(tb) },
			input:  engineInput{Price: 10, Qty: 2},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 20, out["base"])
				assert.InDelta(t, 22.0, out["taxed"], 1e-9)
			},
		},
		{
			name:   "map input passes through",
			engine: func(tb testing.TB) *rlng.Engine { return buildEngine(tb) },
			input:  map[string]any{"price": 10, "qty": 3},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.NoError(t, err)
				assert.Equal(t, 30, out["base"])
			},
		},
		{
			name: "pipeline stage error surfaces",
			engine: func(tb testing.TB) *rlng.Engine {
				boom, err := pipe.NewSingleExpr("x", "qty % 0")
				require.NoError(tb, err)
				p, err := pipe.NewPipeline(boom)
				require.NoError(tb, err)
				e, err := rlng.New(p)
				require.NoError(tb, err)
				return e
			},
			input: map[string]any{"qty": 2},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.Error(t, err)
				assert.Nil(t, out)
			},
		},
		{
			name:   "canceled context short-circuits",
			engine: func(tb testing.TB) *rlng.Engine { return buildEngine(tb) },
			input:  map[string]any{"price": 10, "qty": 2},
			ctx: func(ctx context.Context) context.Context {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				return cctx
			},
			assert: func(t *testing.T, out map[string]any, err error) {
				require.ErrorIs(t, err, context.Canceled)
			},
		},
		{
			name:   "non-flattenable input is an error",
			engine: func(tb testing.TB) *rlng.Engine { return buildEngine(tb) },
			input:  42, // a bare int cannot be decoded into a map[string]any seed
			assert: func(t *testing.T, out map[string]any, err error) {
				require.Error(t, err)
				assert.Nil(t, out)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := tc.engine(t)
			ctx := t.Context()
			if tc.ctx != nil {
				ctx = tc.ctx(ctx)
			}
			out, err := e.Evaluate(ctx, tc.input)
			tc.assert(t, out, err)
		})
	}
}

func TestEngineEvaluateScope(t *testing.T) {
	t.Parallel()

	e := buildEngine(t)
	sc, err := e.EvaluateScope(t.Context(), map[string]any{"price": 10, "qty": 2})
	require.NoError(t, err)

	_, ok := sc.Duration()
	assert.True(t, ok, "EvaluateScope exposes timing")
	v, err := sc.GetFloat64("taxed")
	require.NoError(t, err)
	assert.InDelta(t, 22.0, v, 1e-9)
}

func TestEngineScopeOptions(t *testing.T) {
	t.Parallel()

	// WithScopeOptions must flow through New to the per-Evaluate Scope.
	e := buildEngine(t, rlng.WithScopeOptions(pipe.WithProvenance()))
	sc, err := e.EvaluateScope(t.Context(), map[string]any{"price": 10, "qty": 2})
	require.NoError(t, err)
	assert.True(t, sc.TracksProvenance())
}

// TestEngineConcurrentEvaluateMapInputIsolation reproduces the aliasing/data-race
// (a stage writing a nested path of a shared map[string]any input) and confirms
// the deep-copy seed fix: concurrent Evaluate is race-free (under -race) and the
// caller's nested input map is never mutated.
func TestEngineConcurrentEvaluateMapInputIsolation(t *testing.T) {
	t.Parallel()

	s, err := pipe.NewSingleExpr("rate", "price * 2", pipe.WithOutput("cfg.rate"))
	require.NoError(t, err)
	p, err := pipe.NewPipeline(s)
	require.NoError(t, err)
	eng, err := rlng.New(p)
	require.NoError(t, err)

	input := map[string]any{"price": 10, "cfg": map[string]any{"existing": 1}}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := eng.Evaluate(context.Background(), input)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	cfg := input["cfg"].(map[string]any)
	_, rateWritten := cfg["rate"]
	assert.False(t, rateWritten, "engine must not mutate the caller's nested input map")
}

// TestNewRejectsNilPipeline covers the fail-fast guard: a nil pipeline is a
// construction-time error, not a deferred nil deref on the first Evaluate.
func TestNewRejectsNilPipeline(t *testing.T) {
	t.Parallel()

	e, err := rlng.New(nil)
	assert.Nil(t, e)
	require.ErrorIs(t, err, rlng.ErrNilPipeline)
}

// decimalFidelityLoan is a seed struct exercising every restoreDecimals and
// mapstructureKey path: a tagged decimal field, an untagged decimal field
// (mapstructure key falls back to the Go field name), a nested-struct decimal
// field, a non-decimal int64 field that must survive flatten untouched (not
// widened to float64), a decimal field tagged with a trailing mapstructure
// option (comma-separated key), a decimal field whose tag has an empty name
// segment (falls back to the field name), a plain map field (recursion must
// stop at a non-struct child rather than panic), and an unexported field
// (restoreDecimals must skip it, mirroring mapstructure's own behavior).
type decimalFidelityLoan struct {
	Principal    decimal.Decimal `mapstructure:"principal"`
	Untagged     decimal.Decimal
	Term         int64                 `mapstructure:"term"`
	Nested       decimalFidelityNested `mapstructure:"nested"`
	CommaTag     decimal.Decimal       `mapstructure:"comma_tag,omitempty"`
	EmptyNameTag decimal.Decimal       `mapstructure:",omitempty"`
	Meta         map[string]any        `mapstructure:"meta"`
	secret       string                // unexported: exercises restoreDecimals' PkgPath skip branch
}

type decimalFidelityNested struct {
	Fee decimal.Decimal `mapstructure:"fee"`
}

// decimalFidelityPointerLoan exercises restoreDecimals' pointer-dereference
// loop on a RECURSIVE call: a non-nil *decimalFidelityNested field. Because
// decimalFidelityNested has a mapstructure-tagged field, mapstructure.Decode
// dereferences a non-nil pointer here and decomposes it into a nested
// map[string]any (see dereferencePtrToStructIfNeeded in the mapstructure
// source) — restoreDecimals must then restore the decimal inside it, not
// leave it un-decomposed.
type decimalFidelityPointerLoan struct {
	Nested *decimalFidelityNested `mapstructure:"nested"`
}

// decimalFidelityLineItem/decimalFidelityLines exercise a struct-nested SLICE
// of structs, each holding a decimal.Decimal field — see
// TestEngineSeedPreservesDecimalInSliceOfStructs for why restoreDecimals does
// not need to (and does not) walk into []any: mapstructure.Decode, called
// without the Deep config flag (as flatten does), never decomposes a
// slice-of-structs field into []any of maps in the first place — it leaves
// the raw typed slice in the seed map, decimal fields intact.
type decimalFidelityLineItem struct {
	Fee decimal.Decimal `mapstructure:"fee"`
}

type decimalFidelityLines struct {
	Lines []decimalFidelityLineItem `mapstructure:"lines"`
}

// buildTrivialEngine returns an Engine whose single stage always succeeds
// regardless of input shape, so seeding fidelity can be observed on the Scope
// without any stage computation getting in the way.
func buildTrivialEngine(tb testing.TB) *rlng.Engine {
	tb.Helper()
	noop, err := pipe.NewSingleExpr("noop", "1")
	require.NoError(tb, err)
	p, err := pipe.NewPipeline(noop)
	require.NoError(tb, err)
	e, err := rlng.New(p)
	require.NoError(tb, err)
	return e
}

// TestEngineSeedPreservesDecimal covers flatten's decimal restore (engine.go):
// mapstructure decomposes a struct-nested decimal.Decimal into an empty map
// and no DecodeHook fires for it, so flatten must reflect-walk the seed
// struct and rewrite the real decimal value back in. A map[string]any seed
// bypasses mapstructure entirely and so preserves decimals for free; that
// path is covered here too for symmetry.
func TestEngineSeedPreservesDecimal(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		input  any
		assert func(t *testing.T, sc *pipe.Scope)
	}

	cases := []testCase{
		{
			name: "tagged struct decimal field preserved and int64 not widened to float",
			input: decimalFidelityLoan{
				Principal: decimal.NewFromInt(250000),
				Term:      360,
			},
			assert: func(t *testing.T, sc *pipe.Scope) {
				v, ok := sc.Get("principal")
				require.True(t, ok)
				d, ok := v.(decimal.Decimal)
				require.True(t, ok, "scope value must be decimal.Decimal, got %T", v)
				assert.True(t, decimal.NewFromInt(250000).Equal(d))

				tv, ok := sc.Get("term")
				require.True(t, ok)
				ti, ok := tv.(int64)
				require.True(t, ok, "scope value must remain int64, got %T", tv)
				assert.Equal(t, int64(360), ti)
			},
		},
		{
			name: "untagged struct decimal field preserved under its Go field name",
			input: decimalFidelityLoan{
				Untagged: decimal.NewFromInt(5),
			},
			assert: func(t *testing.T, sc *pipe.Scope) {
				v, ok := sc.Get("Untagged")
				require.True(t, ok)
				d, ok := v.(decimal.Decimal)
				require.True(t, ok, "scope value must be decimal.Decimal, got %T", v)
				assert.True(t, decimal.NewFromInt(5).Equal(d))
			},
		},
		{
			name: "nested struct decimal field preserved",
			input: decimalFidelityLoan{
				Nested: decimalFidelityNested{Fee: decimal.NewFromInt(7)},
			},
			assert: func(t *testing.T, sc *pipe.Scope) {
				v, ok := sc.Get("nested.fee")
				require.True(t, ok)
				d, ok := v.(decimal.Decimal)
				require.True(t, ok, "scope value must be decimal.Decimal, got %T", v)
				assert.True(t, decimal.NewFromInt(7).Equal(d))
			},
		},
		{
			name:  "map seed decimal preserved untouched",
			input: map[string]any{"principal": decimal.NewFromInt(99)},
			assert: func(t *testing.T, sc *pipe.Scope) {
				v, ok := sc.Get("principal")
				require.True(t, ok)
				d, ok := v.(decimal.Decimal)
				require.True(t, ok, "scope value must be decimal.Decimal, got %T", v)
				assert.True(t, decimal.NewFromInt(99).Equal(d))
			},
		},
		{
			name: "comma-tagged decimal field preserved under its trimmed tag name",
			input: decimalFidelityLoan{
				CommaTag: decimal.NewFromInt(11),
			},
			assert: func(t *testing.T, sc *pipe.Scope) {
				v, ok := sc.Get("comma_tag")
				require.True(t, ok)
				d, ok := v.(decimal.Decimal)
				require.True(t, ok, "scope value must be decimal.Decimal, got %T", v)
				assert.True(t, decimal.NewFromInt(11).Equal(d))
			},
		},
		{
			name: "empty-name-tag decimal field falls back to its Go field name",
			input: decimalFidelityLoan{
				EmptyNameTag: decimal.NewFromInt(13),
			},
			assert: func(t *testing.T, sc *pipe.Scope) {
				v, ok := sc.Get("EmptyNameTag")
				require.True(t, ok)
				d, ok := v.(decimal.Decimal)
				require.True(t, ok, "scope value must be decimal.Decimal, got %T", v)
				assert.True(t, decimal.NewFromInt(13).Equal(d))
			},
		},
		{
			// A plain map field's recursion target is Kind Map, not Struct, so
			// restoreDecimals must stop there instead of panicking; an unexported
			// field must be skipped the same way mapstructure itself skips it.
			name: "map field and unexported field do not disrupt restore",
			input: decimalFidelityLoan{
				Principal: decimal.NewFromInt(1),
				Meta:      map[string]any{"note": "x"},
				secret:    "hidden",
			},
			assert: func(t *testing.T, sc *pipe.Scope) {
				v, ok := sc.Get("meta.note")
				require.True(t, ok)
				assert.Equal(t, "x", v)
				_, ok = sc.Get("secret")
				assert.False(t, ok, "unexported field must not leak into the Scope")
			},
		},
		{
			// A non-nil top-level pointer input exercises restoreDecimals'
			// pointer-dereference loop (flatten already rejects a nil pointer
			// before restoreDecimals is ever called).
			name:  "pointer to struct top-level input dereferences correctly",
			input: &decimalFidelityLoan{Principal: decimal.NewFromInt(42)},
			assert: func(t *testing.T, sc *pipe.Scope) {
				v, ok := sc.Get("principal")
				require.True(t, ok)
				d, ok := v.(decimal.Decimal)
				require.True(t, ok, "scope value must be decimal.Decimal, got %T", v)
				assert.True(t, decimal.NewFromInt(42).Equal(d))
			},
		},
		{
			// A non-nil POINTER-typed nested struct field: mapstructure
			// dereferences it into a nested map[string]any (the pointee has a
			// tagged field), and restoreDecimals' own pointer-dereference loop
			// must restore the decimal nested inside it, on the recursive call
			// — not just at the top level.
			name: "non-nil pointer-typed nested struct field decimal is restored",
			input: decimalFidelityPointerLoan{
				Nested: &decimalFidelityNested{Fee: decimal.NewFromInt(77)},
			},
			assert: func(t *testing.T, sc *pipe.Scope) {
				v, ok := sc.Get("nested.fee")
				require.True(t, ok)
				d, ok := v.(decimal.Decimal)
				require.True(t, ok, "scope value must be decimal.Decimal, got %T", v)
				assert.True(t, decimal.NewFromInt(77).Equal(d))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := buildTrivialEngine(t)
			sc, err := e.EvaluateScope(t.Context(), tc.input)
			require.NoError(t, err)
			tc.assert(t, sc)
		})
	}
}

// TestEngineSeedPreservesDecimalInSliceOfStructs is the regression guard for
// a struct-nested slice-of-structs field holding decimals (e.g.
// Lines []LineItem{ Fee decimal.Decimal }). Verified empirically (see
// mstest reproduction in the round-1 review report): mapstructure.Decode,
// invoked without the Deep config flag exactly as flatten does, does NOT
// decompose a slice-of-structs field into []any of maps at all — it leaves
// the field as the original typed Go slice, decimal fields untouched. So
// there is no restoreDecimals gap to close here (nothing was ever flattened
// away to restore); each element's decimal is read correctly straight off
// the raw slice by an expression that indexes into it, which this test
// exercises for every element to guard against a future flatten() change
// (e.g. enabling Deep) silently reintroducing the loss this was mistaken for.
func TestEngineSeedPreservesDecimalInSliceOfStructs(t *testing.T) {
	t.Parallel()

	first, err := pipe.NewSingleExpr("first", "lines[0].Fee")
	require.NoError(t, err)
	second, err := pipe.NewSingleExpr("second", "lines[1].Fee")
	require.NoError(t, err)
	p, err := pipe.NewPipeline(first, second)
	require.NoError(t, err)
	e, err := rlng.New(p)
	require.NoError(t, err)

	input := decimalFidelityLines{Lines: []decimalFidelityLineItem{
		{Fee: decimal.NewFromInt(3)},
		{Fee: decimal.NewFromInt(4)},
	}}
	sc, err := e.EvaluateScope(t.Context(), input)
	require.NoError(t, err)

	v0, ok := sc.Get("first")
	require.True(t, ok)
	d0, ok := v0.(decimal.Decimal)
	require.True(t, ok, "scope value must be decimal.Decimal, got %T", v0)
	assert.True(t, decimal.NewFromInt(3).Equal(d0))

	v1, ok := sc.Get("second")
	require.True(t, ok)
	d1, ok := v1.(decimal.Decimal)
	require.True(t, ok, "scope value must be decimal.Decimal, got %T", v1)
	assert.True(t, decimal.NewFromInt(4).Equal(d1))
}
