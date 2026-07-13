package expr_test

import (
	"testing"

	"github.com/kartaladev/rlng/expr"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecimalArithmetic(t *testing.T) {
	tests := []struct {
		name string
		src  string
		// env overrides the empty-map default env used to Apply; strictEnv
		// additionally compiles with expr.WithEnv(env) for cases that need
		// type information available only through a declared environment
		// (e.g. an int64-typed variable, which a compile-time literal cannot
		// produce).
		env       map[string]any
		strictEnv bool
		// applyEnv, when non-nil, is passed to Apply INSTEAD of env, while env
		// (with strictEnv) still governs compilation. This lets a case compile
		// against one declared env (so the type-checker statically resolves an
		// operator overload, e.g. decimal*decimal -> mulDecimal) and then Apply
		// against a different runtime env whose operand holds a bad-typed
		// value — reaching the dd wrapper's toDecimal error branches, which are
		// unreachable if compile env and apply env are always the same map.
		applyEnv map[string]any
		assert   func(t *testing.T, got any, err error)
	}{
		{
			name: "float-inexact sum is exact via decimal",
			src:  `decimal("0.1") + decimal("0.2")`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("0.3").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "mixed decimal plus int literal",
			src:  `decimal("100") + 3`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("103").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "mixed int literal plus decimal (operand order)",
			src:  `3 + decimal("100")`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("103").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "principal times rate is exact (wrapped operands)",
			src:  `decimal("250000") * decimal("0.0725")`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("18125").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "mixed decimal times int literal",
			src:  `decimal("100") * 3`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("300").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "mixed int literal times decimal (operand order)",
			src:  `3 * decimal("100")`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("300").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "decimal minus decimal (subDecimal operator)",
			src:  `decimal("5") - decimal("2")`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("3").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "mixed decimal minus int literal",
			src:  `decimal("5") - 2`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("3").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "mixed int literal minus decimal (operand order)",
			src:  `5 - decimal("2")`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("3").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "decimal divided by decimal (divDecimal operator)",
			src:  `decimal("10") / decimal("4")`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("2.5").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "mixed decimal divided by int literal",
			src:  `decimal("10") / 4`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("2.5").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "mixed int literal divided by decimal (operand order)",
			src:  `10 / decimal("4")`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("2.5").Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "roundBank half-even to 2 places",
			src:  `roundBank(decimal("2.005"), 2)`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				// StringFixed(2), not String(): decimal.Decimal.String() trims
				// trailing zeros (e.g. "2.00" -> "2"), so a fixed-scale
				// assertion needs StringFixed to preserve the rounded scale.
				assert.Equal(t, "2.00", got.(decimal.Decimal).StringFixed(2)) // half-even: 2.005 -> 2.00
			},
		},
		{
			name: "round half-away to 2 places",
			src:  `round(decimal("2.005"), 2)`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, "2.01", got.(decimal.Decimal).String())
			},
		},
		{
			name: "decimal from bad string errors at eval",
			src:  `decimal("not-a-number")`,
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)
			},
		},
		{
			name: "decimal from float64 literal",
			src:  `decimal(3.14)`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.NewFromFloat(3.14).Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "decimal from int64 via strict env (covers the int64 toDecimal branch)",
			src:  `decimal(x)`,
			env:  map[string]any{"x": int64(42)},
			// Strict env is required here: a lenient/undeclared variable
			// carries no static type, so int64 can only reach toDecimal when
			// the declared env says the identifier's type is int64.
			strictEnv: true,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.True(t, decimal.NewFromInt(42).Equal(got.(decimal.Decimal)))
			},
		},
		{
			name: "decimal from unsupported type errors at eval (lenient mode)",
			src:  `decimal(x)`,
			env:  map[string]any{"x": true},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)
			},
		},
		{
			name: "round from unsupported type errors at eval (lenient mode)",
			src:  `round(x, 2)`,
			env:  map[string]any{"x": true},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)
			},
		},
		{
			name: "roundBank from unsupported type errors at eval (lenient mode)",
			src:  `roundBank(x, 2)`,
			env:  map[string]any{"x": true},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)
			},
		},
		{
			// dd wrapper branch A (first operand): compile resolves `principal *
			// rate` to the mulDecimal(decimal, decimal) overload via the strict
			// env below; Apply then runs against a DIFFERENT env where
			// principal is a non-numeric string, so toDecimal(p[0]) errors.
			name:      "dd wrapper branch A: first operand bad string errors at eval",
			src:       `principal * rate`,
			env:       map[string]any{"principal": decimal.Decimal{}, "rate": decimal.Decimal{}},
			strictEnv: true,
			applyEnv:  map[string]any{"principal": "not-a-number", "rate": decimal.NewFromInt(1)},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)
			},
		},
		{
			name:      "dd wrapper branch A: first operand bool errors at eval",
			src:       `principal * rate`,
			env:       map[string]any{"principal": decimal.Decimal{}, "rate": decimal.Decimal{}},
			strictEnv: true,
			applyEnv:  map[string]any{"principal": true, "rate": decimal.NewFromInt(1)},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)
			},
		},
		{
			// dd wrapper branch B (second operand): first operand is a valid
			// decimal, so toDecimal(p[0]) succeeds and toDecimal(p[1]) is the
			// one that fails.
			name:      "dd wrapper branch B: second operand bad string errors at eval",
			src:       `principal * rate`,
			env:       map[string]any{"principal": decimal.Decimal{}, "rate": decimal.Decimal{}},
			strictEnv: true,
			applyEnv:  map[string]any{"principal": decimal.NewFromInt(1), "rate": "not-a-number"},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)
			},
		},
		{
			name:      "dd wrapper branch B: second operand bool errors at eval",
			src:       `principal * rate`,
			env:       map[string]any{"principal": decimal.Decimal{}, "rate": decimal.Decimal{}},
			strictEnv: true,
			applyEnv:  map[string]any{"principal": decimal.NewFromInt(1), "rate": true},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)
			},
		},
		{
			// Belt-and-braces: round's p[1].(int) type assertion, reached the
			// same way (compile declares places as int so the call type-checks;
			// Apply supplies a non-int). expr's VM recovers the assertion
			// panic into a clean eval error rather than propagating a panic.
			name:      "round with non-int places recovers to an eval error",
			src:       `round(amount, places)`,
			env:       map[string]any{"amount": decimal.Decimal{}, "places": 0},
			strictEnv: true,
			applyEnv:  map[string]any{"amount": decimal.NewFromInt(2), "places": "not-an-int"},
			assert: func(t *testing.T, got any, err error) {
				require.Error(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := tt.env
			if env == nil {
				env = map[string]any{}
			}
			var opts []expr.Option
			if tt.strictEnv {
				opts = append(opts, expr.WithEnv(env))
			}
			fn, err := expr.NewFunction("t", tt.src, opts...)
			require.NoError(t, err)
			applyEnv := any(env)
			if tt.applyEnv != nil {
				applyEnv = tt.applyEnv
			}
			got, err := fn.Apply(applyEnv)
			tt.assert(t, got, err)
		})
	}
}

func TestDecimalStrictEnvBareArithmetic(t *testing.T) {
	// Under strict env (WithEnv), bare-variable decimal arithmetic resolves.
	env := map[string]any{
		"principal": decimal.NewFromInt(250000),
		"rate":      decimal.RequireFromString("0.0725"),
	}
	fn, err := expr.NewFunction("fee", `roundBank(principal * rate, 2)`, expr.WithEnv(env))
	require.NoError(t, err)
	got, err := fn.Apply(env)
	require.NoError(t, err)
	// StringFixed(2), not String(): see the equivalent note in
	// TestDecimalArithmetic's roundBank case.
	assert.Equal(t, "18125.00", got.(decimal.Decimal).StringFixed(2))
}
