# Value serialization/deserialization consistency — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the engine an exact-decimal value type usable inside expressions, fix numeric fidelity in aggregation, make the Scope JSON round-trip preserve value *kind* (not just text), and preserve kind across config decode / struct seed / result mapping — so a value means the same thing at every serde boundary and a persisted decision replays losslessly.

**Architecture:** Depend on `github.com/shopspring/decimal` and register a `decimal()` constructor, arithmetic operator overloads, and `round`/`roundBank` builtins into the shared `expr` compile options so decimal math is exact end to end. Rewrite `pipe`'s `foldNumeric` to accumulate integers in `int64` (checked overflow), promote to `decimal` when a decimal is present, and compare `HitPolicyAny` agreement numerically. Replace the Scope JSON `data` encoding with a canonically type-tagged codec (`{"$k":…,"v":…}`) behind an envelope schema version (`"v":2`) that still reads legacy untagged blobs. Preserve decimal across `flatten` (struct-seed reflection restore, since mapstructure decomposes it) and the mapper. Author ADR-0038 (value model & invariants) and ADR-0039 (exact-decimal type).

**Tech Stack:** Go 1.25, `github.com/expr-lang/expr` v1.17.8, `github.com/shopspring/decimal` v1.4.0 (NEW dep), `github.com/go-viper/mapstructure/v2`, `gopkg.in/yaml.v3`, `crypto`/`encoding/json` (stdlib), `stretchr/testify`.

## Global Constraints

- **Go 1.25+; pure Go, no cgo** (`CGO_ENABLED=0 go build ./...` must pass). `shopspring/decimal` is pure Go — verify with `go list -deps` that no cgo is pulled. Library must not panic/`os.Exit`/`log.Fatal` on caller input; return typed errors; no global logger.
- **One new direct dependency only:** `github.com/shopspring/decimal` (justified for money correctness — ADR-0039). Add nothing else. Run `go mod tidy` and `go mod verify`; keep `go.sum` minimal.
- **Blackbox tests only:** every `_test.go` uses `package <pkg>_test` and drives the exported API. Mandatory `table-test` assert-closure form (`assert func(t, …)` closures, NOT want/wantErr fields) for ≥2 same-SUT cases; fold cases into one table. `t.Context()` over `context.Background()`. Export any sentinel a test must `errors.Is` (e.g. `pipe.ErrAggregateOverflow`).
- **Every exported symbol has a godoc comment.** Target ≥85% coverage on every changed package; **every hot-path and typed-error branch has a covering test.** Hot path here = the decimal builtins/operators, `foldNumeric`, `executeAny` agreement, the Scope JSON encode/decode codec, `flatten`, and the mapper decode.
- **Additive & backward-compatible where the contract already existed.** New: decimal type, tagged JSON (reads old blobs), aggregation fidelity (same results, no longer lossy). The Scope JSON *write* format changes (v2) — a pre-014 reader cannot parse a v2 blob; this is the one deliberate format evolution (ADR-0038 consequence), detectable via the `"v"` marker. No breaking change to an exported Go symbol → normal `feat`, no `!`.
- **Traceability:** every `feat`/`refactor` commit carries `Spec: 014`, `Plan: 014`, and the relevant `ADR:` trailer (`0038` and/or `0039`). End every commit message with `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`. Implements Spec 014; see `docs/specs/014-value-serde-consistency.md` (resolved decisions D1–D5).

## Empirically verified facts (do not re-derive — the plan depends on them)

These were confirmed against the pinned versions; the code below relies on them.

1. **expr operator overloading resolves at COMPILE time by static operand type.** `decimal("0.1") + decimal("0.2")` → `0.3` in the default `AllowUndefinedVariables()` mode (the `decimal()` builtin's return-type hint makes both operands statically decimal). But bare `principal * rate` over *undefined* variables holding decimals fails at **runtime** with `invalid operation: decimal.Decimal * decimal.Decimal` — the overload never resolved. Bare-variable decimal arithmetic only resolves under **strict env mode** (`expr.WithEnv`, Spec 011), where the checker knows the variable is decimal. **Consequence:** the universal, always-works path is `decimal(...)` wrapping; bare arithmetic is the strict-env ergonomic path. Document both; do not attempt to make bare arithmetic work in lenient mode.
2. **A YAML `!decimal` custom tag collapses to a plain `string`** when decoded into `map[string]any` (the tag is lost). The object form `{"$dec":"0.0725"}` survives as a detectable nested map. **Consequence:** config decimal literals use the object form, not the YAML tag.
3. **`mapstructure` decomposes a struct-nested `decimal.Decimal` (no exported fields) into an empty `map[]`, and DecodeHooks do NOT fire for that field.** The `map[string]any` seed path (which `flatten` returns directly) preserves decimals untouched; only struct-seed via mapstructure loses them. **Consequence:** `flatten` needs an explicit reflection post-restore for decimal struct fields — a hook cannot fix it.

## File structure

- `expr/decimal.go` (NEW) — decimal builtins/operators/rounding as `exprlang.Option`s; appended in `buildExprOpts`.
- `expr/options.go` (MODIFY) — call the decimal options from `buildExprOpts`.
- `pipe/table.go` (MODIFY) — `foldNumeric` int64/decimal/float fidelity, `ErrAggregateOverflow`, `executeAny` numeric-aware agreement, `asDecimal` helper.
- `pipe/valuejson.go` (NEW) — `encodeValue`/`decodeValue` canonical type-tag codec + `valueKind` classification.
- `pipe/json.go` (MODIFY) — envelope `Version` field; encode/decode `data` (and derivation values) via the codec; legacy fallback.
- `config/decimal.go` (NEW) — `hydrateDecimals` recursive walk converting `{"$dec":"…"}` → `decimal.Decimal`; `ErrDecimalLiteral`.
- `config/build.go` (MODIFY) — hydrate `Constants` (and per-stage globals) at build.
- `engine.go` (MODIFY) — `flatten` decimal struct-restore.
- `mapper.go` (MODIFY) — decimal-aware decode + lossy-narrow error.
- `errors.go` (MODIFY) — `ErrLossyResultNarrowing` (mapper).
- `examples/decimal_money_test.go` (NEW) — the $250k @ 7.25% end-to-end Example.
- `docs/adrs/0038-value-serde-consistency.md`, `docs/adrs/0039-exact-decimal-value-type.md` (NEW).

---

## Task 1: Decimal value type in `expr` (ADR-0038, ADR-0039; G1, G4-core)

Add the dependency, wire decimal into every compiled expression (builtin, operators, rounding), and author both ADRs. This is the foundation; every later task consumes it.

**Files:**
- Create: `expr/decimal.go`, `expr/decimal_test.go`
- Modify: `expr/options.go:126-142` (`buildExprOpts`)
- Docs: `docs/adrs/0038-value-serde-consistency.md`, `docs/adrs/0039-exact-decimal-value-type.md`

**Interfaces:**
- Produces: decimal is available in ALL `expr.NewFunction`/`expr.NewPredicate` expressions with no extra option — builtins `decimal(x)`, `round(x, places)`, `roundBank(x, places)`; operator overloads for `+ - * /` over `decimal×decimal`, `decimal×int`, `int×decimal`. Canonical Go type across the engine is `github.com/shopspring/decimal`.`Decimal` (used directly, not wrapped).
- Reserved identifier names (document): `decimal`, `round`, `roundBank`.

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/shopspring/decimal@v1.4.0`
Then verify pure-Go: `go list -deps github.com/shopspring/decimal | wc -l` and `CGO_ENABLED=0 go build ./...` (expected: build OK).

- [ ] **Step 2: Write the failing test** (`expr/decimal_test.go`, `package expr_test`)

```go
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
		name   string
		src    string
		assert func(t *testing.T, got any, err error)
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
			name: "roundBank half-even to 2 places",
			src:  `roundBank(decimal("2.005"), 2)`,
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, "2.00", got.(decimal.Decimal).String()) // half-even: 2.005 -> 2.00
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, err := expr.NewFunction("t", tt.src)
			require.NoError(t, err)
			got, err := fn.Apply(map[string]any{})
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
	assert.Equal(t, "18125.00", got.(decimal.Decimal).String())
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./expr/ -run 'TestDecimal' -v`
Expected: FAIL — `decimal`/`round`/`roundBank` unknown, or `invalid operation` on `*`.

- [ ] **Step 4: Implement `expr/decimal.go`**

```go
package expr

import (
	"fmt"

	exprlang "github.com/expr-lang/expr"
	"github.com/shopspring/decimal"
)

// toDecimal converts a supported scalar to a decimal.Decimal. A string is parsed
// exactly; int/float are converted; a decimal passes through. Anything else is an
// error, surfaced from the expression as an eval error.
func toDecimal(v any) (decimal.Decimal, error) {
	switch x := v.(type) {
	case string:
		return decimal.NewFromString(x)
	case int:
		return decimal.NewFromInt(int64(x)), nil
	case int64:
		return decimal.NewFromInt(x), nil
	case float64:
		return decimal.NewFromFloat(x), nil
	case decimal.Decimal:
		return x, nil
	default:
		return decimal.Decimal{}, fmt.Errorf("decimal: unsupported type %T", v)
	}
}

// decimalExprOptions returns the expr-lang options that make the exact-decimal
// value type usable inside every compiled expression: a decimal(x) constructor,
// arithmetic operator overloads (decimal×decimal and mixed decimal×int / int×
// decimal), and rounding builtins. Operator overloads resolve at COMPILE time by
// static operand type: decimal(...) yields a statically decimal value, so wrapped
// arithmetic is exact in any mode; bare-variable arithmetic requires strict env
// (WithEnv). The names decimal, round, roundBank are reserved.
func decimalExprOptions() []exprlang.Option {
	dd := func(f func(a, b decimal.Decimal) decimal.Decimal) func(...any) (any, error) {
		return func(p ...any) (any, error) {
			a, err := toDecimal(p[0])
			if err != nil {
				return nil, err
			}
			b, err := toDecimal(p[1])
			if err != nil {
				return nil, err
			}
			return f(a, b), nil
		}
	}
	return []exprlang.Option{
		exprlang.Function("decimal", func(p ...any) (any, error) { return toDecimal(p[0]) },
			new(func(string) decimal.Decimal),
			new(func(int) decimal.Decimal),
			new(func(float64) decimal.Decimal),
			new(func(decimal.Decimal) decimal.Decimal)),

		// + - * / for decimal×decimal, decimal×int, int×decimal.
		exprlang.Function("addDecimal", dd(decimal.Decimal.Add),
			new(func(decimal.Decimal, decimal.Decimal) decimal.Decimal),
			new(func(decimal.Decimal, int) decimal.Decimal),
			new(func(int, decimal.Decimal) decimal.Decimal)),
		exprlang.Function("subDecimal", dd(decimal.Decimal.Sub),
			new(func(decimal.Decimal, decimal.Decimal) decimal.Decimal),
			new(func(decimal.Decimal, int) decimal.Decimal),
			new(func(int, decimal.Decimal) decimal.Decimal)),
		exprlang.Function("mulDecimal", dd(decimal.Decimal.Mul),
			new(func(decimal.Decimal, decimal.Decimal) decimal.Decimal),
			new(func(decimal.Decimal, int) decimal.Decimal),
			new(func(int, decimal.Decimal) decimal.Decimal)),
		exprlang.Function("divDecimal", dd(decimal.Decimal.Div),
			new(func(decimal.Decimal, decimal.Decimal) decimal.Decimal),
			new(func(decimal.Decimal, int) decimal.Decimal),
			new(func(int, decimal.Decimal) decimal.Decimal)),
		exprlang.Operator("+", "addDecimal"),
		exprlang.Operator("-", "subDecimal"),
		exprlang.Operator("*", "mulDecimal"),
		exprlang.Operator("/", "divDecimal"),

		// round (half-away-from-zero) and roundBank (half-even / banker's).
		exprlang.Function("round", func(p ...any) (any, error) {
			d, err := toDecimal(p[0])
			if err != nil {
				return nil, err
			}
			return d.Round(int32(p[1].(int))), nil
		}, new(func(decimal.Decimal, int) decimal.Decimal)),
		exprlang.Function("roundBank", func(p ...any) (any, error) {
			d, err := toDecimal(p[0])
			if err != nil {
				return nil, err
			}
			return d.RoundBank(int32(p[1].(int))), nil
		}, new(func(decimal.Decimal, int) decimal.Decimal)),
	}
}
```

Note on `Operator` + operand order: registering the mixed `int, decimal` overload variant on `addDecimal`/`mulDecimal` handles `3 * decimal("100")`. The `dd` wrapper coerces both operands via `toDecimal`, so it is order-agnostic at runtime; the extra type hints let the checker resolve the overload.

- [ ] **Step 5: Wire into `buildExprOpts`** (`expr/options.go`)

In `buildExprOpts`, after the undefined-vars/env branch and before returning, append the decimal options so they apply to every compiled program:

```go
func buildExprOpts(cfg *config) []exprlang.Option {
	var opts []exprlang.Option
	if cfg.hasEnv {
		opts = append(opts, exprlang.Env(strictEnv(cfg)))
	} else {
		opts = append(opts, exprlang.AllowUndefinedVariables())
	}
	if p := newPatcher(cfg.globals, cfg.locals); p != nil {
		opts = append(opts, exprlang.Patch(p))
	}
	for _, f := range cfg.functions {
		opts = append(opts, exprlang.Function(f.name, f.fn))
	}
	opts = append(opts, decimalExprOptions()...) // exact-decimal type, always available
	return opts
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./expr/ -run 'TestDecimal' -v` (expected: PASS)
Then the full package + race: `go test ./expr/ -race` (expected: PASS — confirm no existing expr test regressed, especially operator behavior on native int/float and any WithEnv strict tests).

- [ ] **Step 7: Author ADR-0038 and ADR-0039**

`docs/adrs/0038-value-serde-consistency.md` (Nygard format): Context = the six-boundary value-fidelity problem (cite Spec 014); Decision = the canonical value kinds (bool, string, int64, float64, decimal, time, list, map) and the fidelity contract per boundary, plus **full canonical type tagging** in Scope JSON with a version marker and legacy-read fallback; Consequences = a v2 blob is not readable by a pre-014 reader (one-way format evolution, detectable via `"v"`); numeric-aware aggregation may change a previously-lossy result to the correct one.
`docs/adrs/0039-exact-decimal-value-type.md`: Context = float money blocker + ADR-0030 minor-units interim breaks on rates; Decision = depend on `shopspring/decimal`, expose `decimal()` + operator overloads + `round`/`roundBank`; **operator overloading resolves at compile time — wrapped `decimal(...)` is universal, bare arithmetic needs strict env**; reserved names `decimal`/`round`/`roundBank`. Consequences = one new dependency (justified), pure-Go preserved. Link both to Spec 014 and back from the spec is already present (Anticipated ADRs).

- [ ] **Step 8: Commit**

```bash
go mod tidy && go mod verify
git add expr/decimal.go expr/decimal_test.go expr/options.go go.mod go.sum \
  docs/adrs/0038-value-serde-consistency.md docs/adrs/0039-exact-decimal-value-type.md
git commit  # message with: feat(expr): exact-decimal value type + arithmetic/rounding builtins
```
Trailers: `Spec: 014`, `Plan: 014`, `ADR: 0038`, `ADR: 0039`.

---

## Task 2: Numeric fidelity in aggregation (G2)

Rewrite `foldNumeric` so integer sums stay exact in `int64` (checked overflow), decimals fold in decimal, `min`/`max` return the actual matched element, and `HitPolicyAny` agreement compares values numerically.

**Files:**
- Modify: `pipe/table.go` (`foldNumeric:462-488`, `asFloat:492-504`, `executeAny:292-333`, error vars `48-58`, `AggregateSum` doc `38-43`)
- Test: `pipe/table_test.go` (add to existing `package pipe_test` tables)

**Interfaces:**
- Consumes: `decimal.Decimal` (Task 1's canonical type) via `github.com/shopspring/decimal`.
- Produces: `pipe.ErrAggregateOverflow` (exported sentinel, `Cause` of a `StageError`). `foldNumeric` returns `int64` for all-int folds, `decimal.Decimal` when any decimal is present, else `float64`; `min`/`max` return the matched element as-is.

- [ ] **Step 1: Write the failing tests** (`pipe/table_test.go`, `package pipe_test`) — add a table:

```go
func TestDecisionTableCollectAggregationFidelity(t *testing.T) {
	tests := []struct {
		name   string
		agg    pipe.CollectAggregation
		values []int64 // decision returns these per matching rule (via literals)
		assert func(t *testing.T, got any, err error)
	}{
		{
			name:   "int sum above 2^53 stays exact int64",
			agg:    pipe.AggregateSum,
			values: []int64{9007199254740993, 1}, // 2^53+1 then +1
			assert: func(t *testing.T, got any, err error) {
				require.NoError(t, err)
				assert.Equal(t, int64(9007199254740994), got)
			},
		},
		{
			name:   "int sum overflow errors, not garbage",
			agg:    pipe.AggregateSum,
			values: []int64{9223372036854775807, 1}, // maxint64 + 1
			assert: func(t *testing.T, got any, err error) {
				require.ErrorIs(t, err, pipe.ErrAggregateOverflow)
			},
		},
	}
	// ... build a HitPolicyCollect table whose rules each return one value from tt.values,
	// run Execute against an empty-ish scope, read name.<key>, assert.
	_ = tests
}
```

Add focused tests (fold into the existing collect/any test tables where present):
- **Decimal sum**: two rules return `decimal("0.1")` and `decimal("0.2")` → aggregate sum is `decimal("0.3")` (exact).
- **Mixed int + decimal sum** promotes to decimal per contract.
- **min/max return the exact matched element**: values `decimal("1.5")`, `decimal("2.5")` → max is the `decimal("2.5")` value (a `decimal.Decimal`, not a float).
- **HitPolicyAny no false conflict**: two matching rules set the same key to `10` (int) and `10.0` (float) → agreement, no error; the written value is defined (keep the first-seen).
- **HitPolicyAny genuine disagreement** (`10` vs `11`) still `ErrConflictingMatches`.
- **Non-numeric aggregate** still `ErrNonNumericAggregate`.

Use the existing decision-table test helpers/builders in `pipe/table_test.go`; express per-rule values via decision expressions (e.g. `decimal("0.1")`, `10`, `10.0`). All cases use the `assert` closure form.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pipe/ -run 'TestDecisionTableCollectAggregationFidelity|Any' -v`
Expected: FAIL (overflow returns garbage; decimal sum errors as non-numeric; int/float false conflict).

- [ ] **Step 3: Add the overflow sentinel** (`pipe/table.go`, near line 58)

```go
// ErrAggregateOverflow is the Cause of a StageError when an integer sum/min/max
// aggregation exceeds the range of int64. The engine errors rather than
// silently wrapping or degrading to float64.
var ErrAggregateOverflow = errors.New("decision-table: integer aggregation overflows int64")
```

- [ ] **Step 4: Rewrite `foldNumeric` + helpers**

Replace `foldNumeric`/`asFloat` (lines 459-504) with a kind-aware fold. Three tiers: if any value is `decimal.Decimal`, fold in decimal; else if any is float, fold in float64; else fold in int64 with checked overflow. `min`/`max` return the actual element.

```go
// numKind ranks numeric kinds so a fold promotes to the widest present kind:
// int64 < float64 < decimal.
type numKind int

const (
	kindInt numKind = iota
	kindFloat
	kindDecimal
	kindNonNumeric
)

func classify(v any) numKind {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return kindInt
	case float32, float64:
		return kindFloat
	case decimal.Decimal:
		return kindDecimal
	default:
		return kindNonNumeric
	}
}

// foldNumeric folds vals (non-empty) by op with kind fidelity: an all-integer
// fold stays int64 (checked overflow -> ErrAggregateOverflow); any decimal
// present folds in decimal; otherwise float64. min/max return the actual matched
// element. A non-numeric value is ErrNonNumericAggregate.
func foldNumeric(vals []any, op numericOp) (any, error) {
	widest := kindInt
	for _, v := range vals {
		k := classify(v)
		if k == kindNonNumeric {
			return nil, fmt.Errorf("%w: %v (%T)", ErrNonNumericAggregate, v, v)
		}
		if k > widest {
			widest = k
		}
	}
	switch widest {
	case kindDecimal:
		return foldDecimal(vals, op)
	case kindFloat:
		return foldFloat(vals, op)
	default:
		return foldInt(vals, op)
	}
}
```

Then `foldInt` (checked add via `math/bits` or overflow test), `foldFloat`, `foldDecimal`. For sum, detect int64 overflow:

```go
func foldInt(vals []any, op numericOp) (any, error) {
	acc := toInt64(vals[0])
	best := acc // for min/max, best is the winning element's int64
	bestIdx := 0
	for i := 1; i < len(vals); i++ {
		n := toInt64(vals[i])
		switch op {
		case aggSum:
			sum := acc + n
			// overflow iff sign of both operands equal and differs from result
			if (acc > 0 && n > 0 && sum < 0) || (acc < 0 && n < 0 && sum > 0) {
				return nil, ErrAggregateOverflow
			}
			acc = sum
		case aggMin:
			if n < best {
				best, bestIdx = n, i
			}
		case aggMax:
			if n > best {
				best, bestIdx = n, i
			}
		}
	}
	if op == aggSum {
		return acc, nil
	}
	return vals[bestIdx], nil // return the ACTUAL matched element
}
```

`foldDecimal`: convert each via a `asDecimal(v any) (decimal.Decimal, bool)` helper (handles int/float/decimal), `Add` for sum, `Cmp` for min/max returning `vals[bestIdx]`. `foldFloat`: mirror `foldInt` in float64, return `vals[bestIdx]` for min/max. Add `toInt64(any) int64` and `asDecimal(any) (decimal.Decimal, bool)` helpers. Import `github.com/shopspring/decimal`.

Overflow note: the sign-based check above catches sum overflow without `math/bits`; if you prefer, use `math/bits.Add64` on the two's-complement — either is fine, but the sign check is simplest and testable. For min/max there is no overflow.

- [ ] **Step 5: Make `HitPolicyAny` agreement numeric-aware** (`executeAny`, line 315)

Replace the `reflect.DeepEqual(prev, v)` conflict check with a numeric-aware comparison: if both `prev` and `v` are numeric (per `classify` != `kindNonNumeric`), compare by magnitude (promote both to decimal and `Cmp`, or compare as float when neither is decimal); otherwise fall back to `reflect.DeepEqual`.

```go
if !valuesAgree(prev, v) {
	return d.stageErr(fmt.Errorf("%w: key %q has %v and %v", ErrConflictingMatches, dec.key, prev, v))
}
```
```go
// valuesAgree reports whether two decision values agree for HitPolicyAny. Numeric
// values agree by magnitude regardless of int/float/decimal kind (so 10 and 10.0
// agree); non-numeric values fall back to reflect.DeepEqual.
func valuesAgree(a, b any) bool {
	ka, kb := classify(a), classify(b)
	if ka != kindNonNumeric && kb != kindNonNumeric {
		da, _ := asDecimal(a)
		db, _ := asDecimal(b)
		return da.Equal(db)
	}
	return reflect.DeepEqual(a, b)
}
```

Update the `AggregateSum` godoc (line 38-43) to say "sums the matched numeric values (int64 if all integers, decimal if any decimal, else float64; overflow errors)".

- [ ] **Step 6: Run tests to verify pass**

Run: `go test ./pipe/ -run 'Aggregation|Any|Collect' -race -v` (expected: PASS)
Then `go test ./pipe/ -race` (whole package green).

- [ ] **Step 7: Commit**

```bash
git add pipe/table.go pipe/table_test.go
git commit  # fix(pipe): int64/decimal aggregation fidelity + numeric-aware any agreement
```
Trailers: `Spec: 014`, `Plan: 014`, `ADR: 0038`.

---

## Task 3: Scope JSON canonical type tagging (G3)

Replace the `data` (and derivation-value) JSON representation with a canonically type-tagged codec behind an envelope version, preserving kind on reload while still reading legacy untagged blobs.

**Files:**
- Create: `pipe/valuejson.go`, and tests in `pipe/json_test.go` (existing `package pipe_test`)
- Modify: `pipe/json.go` (`scopeJSON` struct, `MarshalJSON`, `UnmarshalJSON`)

**Interfaces:**
- Consumes: `decimal.Decimal`; `pipe` value kinds from Task 1/2.
- Produces: `scopeJSON.Version int` (`json:"v"`); `encodeValue(any) any` and `decodeValue(any) (any, error)` codec. Wire tags: `bool`,`string`,`int64`,`float64`,`decimal`,`time`,`list`,`map`. Envelope: `{"v":2,"data":{…tagged…},"timing"?,…}`.

- [ ] **Step 1: Write the failing round-trip test** (`pipe/json_test.go`)

```go
func TestScopeJSONKindRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		set    func(sc *pipe.Scope)
		path   string
		assert func(t *testing.T, v any, ok bool)
	}{
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
				assert.Equal(t, "18125.00", d.String()) // scale preserved
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
				m := v.(map[string]any)
				assert.IsType(t, decimal.Decimal{}, m["amt"])
				assert.Equal(t, int64(2), m["cnt"])
			},
		},
		{name: "bool", set: func(sc *pipe.Scope) { _ = sc.Set("b", true) }, path: "b",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, true, v) }},
		{name: "string", set: func(sc *pipe.Scope) { _ = sc.Set("s", "hi") }, path: "s",
			assert: func(t *testing.T, v any, ok bool) { require.True(t, ok); assert.Equal(t, "hi", v) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func TestScopeJSONDeterministic(t *testing.T) {
	sc := pipe.NewScope(map[string]any{"b": int64(2), "a": decimal.RequireFromString("1.0")})
	b1, _ := json.Marshal(sc)
	b2, _ := json.Marshal(sc)
	assert.Equal(t, string(b1), string(b2)) // byte-identical
}

func TestScopeJSONLegacyBlobLoads(t *testing.T) {
	// A pre-014 (untagged) blob still loads; numbers come back as json.Number.
	legacy := []byte(`{"data":{"count":3,"name":"x"}}`)
	var sc pipe.Scope
	require.NoError(t, json.Unmarshal(legacy, &sc))
	n, err := sc.GetInt("count")
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	s, _ := sc.GetString("name")
	assert.Equal(t, "x", s)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pipe/ -run 'TestScopeJSON' -v`
Expected: FAIL — decimal marshals to a bare number/object and reloads as json.Number/map, not decimal; int64 reloads as json.Number.

- [ ] **Step 3: Implement `pipe/valuejson.go`**

```go
package pipe

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// taggedValue is the canonical on-wire form of a single scope value: its kind and
// payload. Full type tagging preserves kind across a JSON round-trip so a
// reloaded decision reproduces the same result (Spec 014 / ADR-0038).
type taggedValue struct {
	Kind string          `json:"$k"`
	V    json.RawMessage `json:"v"`
}

// encodeValue converts a scope value into its type-tagged wire form, recursing
// through maps and slices. Unknown/complex kinds fall back to a "json" tag whose
// payload is the default JSON encoding (best-effort, lossy on reload to any).
func encodeValue(v any) (any, error) { /* switch on concrete type -> {"$k":kind,"v":payload} */ }

// decodeValue inverts encodeValue for a v2 blob. A value that is NOT a tagged
// object (legacy/untagged) is returned as-is (numbers arrive as json.Number
// because the decoder uses UseNumber), preserving backward-compatible reads.
func decodeValue(v any) (any, error) { /* detect {"$k","v"} map -> rehydrate by kind; else passthrough */ }
```

Encoding rules (implement exactly):
- `bool` → `{"$k":"bool","v":true}`
- `string` → `{"$k":"string","v":"…"}`
- `int,int8..int64,uint..` → `{"$k":"int64","v":<number>}` (store as int64; document uint>maxint64 is out of contract → error)
- `float32/float64` → `{"$k":"float64","v":<number>}`
- `decimal.Decimal` → `{"$k":"decimal","v":"<d.String()>"}` (string payload = exact)
- `time.Time` → `{"$k":"time","v":"<RFC3339Nano>"}`
- `json.Number` → treat as int64 if integral else float64 (so a value that already round-tripped once re-encodes stably)
- `[]any` → `{"$k":"list","v":[<encodeValue each>]}`
- `map[string]any` → `{"$k":"map","v":{<sorted keys>: encodeValue each}}` (sort keys for determinism)
- `nil` → `{"$k":"null","v":null}`

Decoding rehydrates each `$k` to the Go kind (decimal via `decimal.NewFromString`, time via `time.Parse(time.RFC3339Nano, …)`, int64/float64 from the JSON number, list/map recursively). A malformed tag → a typed error (define `ErrMalformedScopeValue`). A non-tagged input (no `$k`) is returned unchanged — the legacy path.

- [ ] **Step 4: Wire the codec into `pipe/json.go`**

Add `Version int json:"v,omitempty"` to `scopeJSON`. In `MarshalJSON`, set `env.Version = 2` and encode `env.Data` as `map[string]any` where each value = `encodeValue(v)` (also encode derivation `.Value` fields the same way for full fidelity). In `UnmarshalJSON`, after `dec.Decode(&env)`: if `env.Version >= 2`, run each `data` value through `decodeValue`; if `env.Version == 0` (legacy), keep the current behavior (bare `json.Number` etc.). Update the `UnmarshalJSON` godoc to state that v2 preserves kind (int64/decimal/float/time) while a legacy blob still loads with numbers as `json.Number`.

Keep the map-key sort in `encodeValue` for map kind so output is deterministic; top-level `data` is already a Go map marshaled by encoding/json (which sorts keys), so determinism holds.

- [ ] **Step 5: Run tests**

Run: `go test ./pipe/ -run 'TestScopeJSON' -race -v` (expected: PASS), then `go test ./pipe/ -race`.
Confirm the Spec 013 ruleset/firing round-trip tests still pass (they ride in the same envelope).

- [ ] **Step 6: Commit**

```bash
git add pipe/valuejson.go pipe/json.go pipe/json_test.go
git commit  # feat(pipe): canonical type-tagged Scope JSON preserving value kind (v2)
```
Trailers: `Spec: 014`, `Plan: 014`, `ADR: 0038`.

---

## Task 4: Config decimal literals + seed/mapping fidelity (G5)

Let config declare decimal constants (object form), preserve struct-seeded decimals through `flatten`, and keep the mapper decimal-faithful (erroring on a genuinely lossy narrowing).

**Files:**
- Create: `config/decimal.go`, `config/decimal_test.go`
- Modify: `config/build.go` (hydrate constants), `engine.go` (`flatten:80-93`), `mapper.go` (`Map:53-71`), `errors.go` (new mapper sentinel)
- Test: `engine_test.go`, `mapper_test.go` (existing `_test` packages)

**Interfaces:**
- Consumes: `decimal.Decimal`; Task 1's `decimal()`/operators for downstream expressions.
- Produces: `config.ErrDecimalLiteral`; `rlng.ErrLossyResultNarrowing`. Config constant `{"$dec":"0.0725"}` → `decimal.Decimal`. `flatten` preserves struct decimal fields. Mapper decodes decimal→decimal fields and errors on decimal→int with a fractional part.

- [ ] **Step 1: Write failing tests**

`config/decimal_test.go` (`package config_test`): a `constants` map with `{"$dec":"0.0725"}` hydrates to a `decimal.Decimal` usable as a compile-time default; a bad `{"$dec":"nope"}` → `ErrDecimalLiteral`. Drive through `config.Build` + evaluate an expression referencing the constant (strict-env or wrapped), or assert the parsed/hydrated value via the public build path.

`engine_test.go`: seeding a **struct** with a `decimal.Decimal` field yields that decimal in the Scope (not `map[]`); seeding a `map[string]any` with a decimal preserves it; an `int64` field is not widened to float.

```go
func TestEngineSeedPreservesDecimal(t *testing.T) {
	type Loan struct {
		Principal decimal.Decimal `mapstructure:"principal"`
		Term      int64           `mapstructure:"term"`
	}
	// pipeline that copies principal through to an output via decimal(principal)
	// ... build engine, Evaluate(ctx, Loan{Principal: decimal.NewFromInt(250000), Term: 360})
	// assert scope["principal"] is decimal.Decimal equal to 250000, scope["term"] is int64(360)
}
```

`mapper_test.go`: mapping a scope holding `decimal("18125.00")` into a struct field `Fee decimal.Decimal` yields the exact decimal; mapping a fractional decimal into an `int` field returns `ErrLossyResultNarrowing`; mapping an integral decimal into `int` succeeds.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./config/ -run Decimal -v; go test . -run 'SeedPreservesDecimal|Mapper.*Decimal' -v`
Expected: FAIL (constant stays a map; struct decimal becomes `map[]`; mapper can't place decimal / doesn't guard narrowing).

- [ ] **Step 3: Implement `config/decimal.go`**

```go
package config

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// ErrDecimalLiteral is returned when a {"$dec": "…"} config literal does not
// parse as an exact decimal.
var ErrDecimalLiteral = errors.New("config: invalid $dec decimal literal")

// hydrateDecimals recursively replaces every {"$dec": "<string>"} object in m
// with a decimal.Decimal, in place. The object form is used because a YAML
// !decimal tag collapses to a plain string when decoded into map[string]any.
func hydrateDecimals(m map[string]any) error {
	for k, v := range m {
		nv, err := hydrateValue(v)
		if err != nil {
			return err
		}
		m[k] = nv
	}
	return nil
}

func hydrateValue(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		if raw, ok := decLiteral(x); ok {
			d, err := decimal.NewFromString(raw)
			if err != nil {
				return nil, fmt.Errorf("%w: %q: %v", ErrDecimalLiteral, raw, err)
			}
			return d, nil
		}
		if err := hydrateDecimals(x); err != nil {
			return nil, err
		}
		return x, nil
	case []any:
		for i := range x {
			nv, err := hydrateValue(x[i])
			if err != nil {
				return nil, err
			}
			x[i] = nv
		}
		return x, nil
	default:
		return v, nil
	}
}

// decLiteral reports whether m is exactly {"$dec": "<string>"} and returns the
// string. A map with $dec plus other keys is NOT a literal (returns false).
func decLiteral(m map[string]any) (string, bool) {
	if len(m) != 1 {
		return "", false
	}
	s, ok := m["$dec"].(string)
	return s, ok
}
```

- [ ] **Step 4: Call it in `config/build.go`**

At the point `Constants` (and any per-stage globals maps) are read for compilation, call `hydrateDecimals` on the map first, returning any `ErrDecimalLiteral` as a build error attributed to the field. (Find where `PipelineDef.Constants` feeds `expr.WithGlobals`; hydrate before wiring.)

- [ ] **Step 5: `flatten` decimal restore** (`engine.go`)

After `mapstructure.Decode`, restore struct-nested decimals (mapstructure decomposes them into `map[]` and no hook fires). Walk the input's struct fields; for each field of type `decimal.Decimal`, write the real value into `m` under the field's mapstructure key.

```go
var decimalType = reflect.TypeOf(decimal.Decimal{})

func flatten[I any](input I) (map[string]any, error) {
	if m, ok := any(input).(map[string]any); ok {
		return m, nil // map seed preserves decimals untouched
	}
	rv := reflect.ValueOf(input)
	if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		return nil, ErrNilInput
	}
	var m map[string]any
	if err := mapstructure.Decode(input, &m); err != nil {
		return nil, fmt.Errorf("rlng: seed input: %w", err)
	}
	restoreDecimals(rv, m)
	return m, nil
}

// restoreDecimals rewrites decimal.Decimal struct fields back into m, which
// mapstructure decomposes into empty maps (it has no exported fields, and decode
// hooks do not fire for it). Recurses into nested struct fields and their maps.
func restoreDecimals(rv reflect.Value, m map[string]any) {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		key := mapstructureKey(f)
		if f.Type == decimalType {
			m[key] = rv.Field(i).Interface()
			continue
		}
		if child, ok := m[key].(map[string]any); ok {
			restoreDecimals(rv.Field(i), child)
		}
	}
}

// mapstructureKey returns the map key mapstructure uses for field f: the first
// segment of the `mapstructure` tag if present, else the field name.
func mapstructureKey(f reflect.StructField) string {
	tag := f.Tag.Get("mapstructure")
	if tag == "" {
		return f.Name
	}
	if i := strings.IndexByte(tag, ','); i >= 0 {
		tag = tag[:i]
	}
	if tag == "" {
		return f.Name
	}
	return tag
}
```
Add imports `reflect` (present), `strings`, and `github.com/shopspring/decimal`. Note: mapstructure lowercases nothing by default for struct→map (it uses the tag or exact field name — verified: key was the tag `principal`), so `mapstructureKey` matches.

- [ ] **Step 6: Mapper decimal + lossy-narrow guard** (`mapper.go`, `errors.go`)

The map→struct decode already handles decimal→decimal (verified). Add a `DecodeHook` (via `mapstructure.NewDecoder` with `DecoderConfig`) that, when the source is `decimal.Decimal` and the target is an integer kind, errors `ErrLossyResultNarrowing` if the decimal has a fractional part, else returns the integer; decimal→float returns `d.InexactFloat64()`; decimal→string returns `d.String()`; decimal→decimal passes through. Replace the plain `mapstructure.Decode(out, &r)` in `Map` with the configured decoder.

```go
// errors.go
var ErrLossyResultNarrowing = errors.New("rlng: mapping would lose precision narrowing a decimal")
```
```go
// mapper.go Map(): build decoder
cfg := &mapstructure.DecoderConfig{Result: &r, DecodeHook: decimalNarrowHook}
dec, err := mapstructure.NewDecoder(cfg)
if err != nil { return zero, &MappingError{Cause: err} }
if err := dec.Decode(out); err != nil { return zero, &MappingError{Cause: err} }
```
```go
func decimalNarrowHook(from, to reflect.Type) ... // DecodeHookFuncType
// if from == decimalType: switch to.Kind() {
//   Int..Int64,Uint..: if !d.IsInteger() -> ErrLossyResultNarrowing; else return d.IntPart()
//   Float32,Float64: return d.InexactFloat64()
//   String: return d.String()
//   default (incl. struct==decimal): return data unchanged
// }
```
Note: the hook receives `data any`; type-assert `data.(decimal.Decimal)`. Return `(data, nil)` for non-decimal sources.

- [ ] **Step 7: Run tests**

Run: `go test ./config/ . -run 'Decimal|Seed|Mapper' -race -v` (expected: PASS), then `go test ./... -race`.

- [ ] **Step 8: Commit**

```bash
git add config/decimal.go config/decimal_test.go config/build.go engine.go engine_test.go mapper.go mapper_test.go errors.go
git commit  # feat(config,rlng): decimal config literals + seed/mapping kind fidelity
```
Trailers: `Spec: 014`, `Plan: 014`, `ADR: 0038`, `ADR: 0039`.

---

## Task 5: Driving acceptance example + docs (capstone)

Prove the whole invariant end to end and document it.

**Files:**
- Create: `examples/decimal_money_test.go` (`package examples_test`, runnable `Example…`)
- Modify: `README.md` (a short decimal-money pointer), `docs/adrs/0038-*`/`0039-*` cross-links if any gap.

**Interfaces:**
- Consumes: everything from Tasks 1–4.

- [ ] **Step 1: Write the acceptance Example test**

A runnable example computing a loan fee: principal `$250,000` seeded as a decimal, rate `0.0725` (config-declared `{"$dec":…}` or `decimal("0.0725")`), fee `= roundBank(principal * rate, 2)` → exactly `18125.00`. Then: marshal the Scope to JSON, unmarshal into a fresh Scope, and assert the reloaded `fee` is still `decimal("18125.00")` and reproduces the same result; map into a typed result struct with a `decimal.Decimal` `Fee` field.

```go
func ExampleTypedEngine_decimalMoney() {
	// build config/pipeline with rate as a decimal, fee = roundBank(principal * rate, 2)
	// seed Principal = decimal.NewFromInt(250000)
	// evaluate -> scope; print fee (18125.00)
	// json round-trip -> reload -> print reloaded fee (18125.00)
	// Output:
	// fee=18125.00
	// reloaded=18125.00
}
```
Use whichever engine surface fits (`TypedEngine`/`Mapper` for the mapped result, or `BareEngine`/`Engine` + `EvaluateScope` for the Scope/JSON round-trip). Wire the rate either as `decimal("0.0725")` inline (works in default mode) or via a strict-env config constant — pick the one that reads cleanly as documentation.

- [ ] **Step 2: Run it**

Run: `go test ./examples/ -run 'ExampleTypedEngine_decimalMoney' -v` (expected: PASS — the `// Output:` matches).

- [ ] **Step 3: README pointer + doc polish**

Add a brief "Exact-decimal money" subsection to `README.md` pointing at the example and stating: use `decimal("…")` (universal) or declare fields decimal + strict-env for bare arithmetic; `round`/`roundBank` for deterministic rounding. Ensure ADR-0038/0039 link Spec 014 and Plan 014.

- [ ] **Step 4: Commit**

```bash
git add examples/decimal_money_test.go README.md docs/adrs/0038-value-serde-consistency.md docs/adrs/0039-exact-decimal-value-type.md
git commit  # docs(examples): end-to-end exact-decimal money acceptance example
```
Trailers: `Spec: 014`, `Plan: 014`, `ADR: 0038`, `ADR: 0039`.

---

## Whole-branch delivery gate (before proposing merge)

After Task 5, on `claude/value-serde-014`:
1. `go build ./...` and `CGO_ENABLED=0 go build ./...` — OK.
2. `go test ./... -race` — green.
3. `go vet ./...` clean; `gofmt -l .` empty. (`golangci-lint`/`govulncheck` run in CI — let CI go green.)
4. `go mod tidy` leaves go.mod/go.sum unchanged; `go mod verify` passes; confirm `shopspring/decimal` is the ONLY new direct dep and pulls no cgo.
5. Coverage: `go test ./... -cover` — every changed package ≥85%; confirm each hot-path/typed-error branch (decimal builtins/operators, foldNumeric tiers + overflow, executeAny agreement, valuejson encode/decode per kind + malformed + legacy, flatten restore, mapper narrow hook) has a covering case.
6. `/code-review` and `/security-review` over `main..HEAD`; resolve or triage every finding (ledger the triage rationale).
7. Update `docs/HANDOVER.md`; present increment 014 for the user's merge/push decision (never push without approval).

## Self-review (author checklist — completed)

- **Spec coverage:** G1→ADR-0038 (Task 1 Step 7). G2→Task 2. G3→Task 3. G4→Task 1 + acceptance in Task 5. G5→Task 4. Hot-path branch list in the delivery gate maps to the spec's "Hot-path branches (test targets)". ✅
- **Placeholder scan:** The `encodeValue`/`decodeValue` bodies and the mapper hook are described with exact per-kind rules rather than full bodies — these are mechanical switch statements fully specified by the encoding-rules list and the hook bullet; the implementer writes the switch from the spec'd rules. No "TBD/handle edge cases" left. ✅
- **Type consistency:** `decimal.Decimal` (shopspring) is the single canonical decimal type across all tasks; `classify`/`asDecimal`/`toDecimal` names are used consistently; wire tags (`int64`,`float64`,`decimal`,`time`,`bool`,`string`,`list`,`map`,`null`) match between encode and decode. ✅
- **Deviations from spec D2 (noted):** config decimal literal is the object form `{"$dec":…}` only (the YAML `!decimal` tag collapses in `map[string]any`); bare-variable decimal arithmetic requires strict-env (compile-time overload resolution) — the `decimal(...)` wrap is the universal path. Both recorded in ADR-0039. ✅
