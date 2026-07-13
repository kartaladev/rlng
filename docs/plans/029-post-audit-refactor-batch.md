# Post-Audit Refactor Batch (R1–R9, R11) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the ten behavior-preserving, quality-only refactors from `docs/specs/029-post-audit-refactor-batch.md` — remove the audit-identified duplication (headline: one overflow-checked numeric core) with zero observable behavior, contract, `Hash()`, or schema change.

**Architecture:** Each task is an independent, localized refactor validated by the package's existing tests staying byte-green, plus new table cases pinning any newly-reachable branch, all exercised through the **public** surface (blackbox `_test` packages). No public API is added, removed, or changed; `apidiff` must report no exported-surface delta. R1 carries an ADR (0054); R2–R9, R11 do not.

**Tech Stack:** Go 1.25+, `expr-lang/expr` v1.17.x, `shopspring/decimal`, `go-viper/mapstructure/v2`, `gopkg.in/yaml.v3`. Tests: standard `testing`, blackbox `package <pkg>_test`, `table-test` assert-closure form, `t.Context()`.

## Global Constraints

- **Behavior-preserving only.** Same inputs → same outputs, same errors (same sentinels, same wrapping), same `Hash()` bytes, same race-cleanliness. If any item is found to *require* a behavior change, drop it from the batch and re-scope it — do not smuggle a behavior change in.
- **No public API change.** R1's numeric kernel and R2's `deriveOrSet` are **unexported**. No exported symbol added/removed/changed.
- **Blackbox tests only.** Every `_test.go` uses `package <pkg>_test` and drives the exported API. Assert-closure tables (no `want`/`wantErr` fields). `t.Context()` over `context.Background()`.
- **Every task ends green:** `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), `go test ./... -race` pass before its commit. Each task is a pre-authorized per-task commit (plan-execution exception in CLAUDE.md); no WIP/broken commits.
- **Module path:** `github.com/kartaladev/rlng`. **Commit trailers:** every commit carries `Spec: 029`, `Plan: 029`, the relevant `Backlog:` id(s), and (Task 1 only) `ADR: 0054`. End messages with `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- **Order is fixed:** R1 → R2 → R7 → R8 → R3 → R4 → R9 → R6 → R11 → R5 (R5 last; it touches the most files). Tasks are independent, but keeping the order minimizes rebase churn.
- **`govulncheck`/`golangci-lint` are not installed locally** — CI runs them on push; do not block on them locally.

---

## File Structure

- `pipe/numeric.go` — **new.** The shared, unexported, overflow-checked numeric reflect kernel (R1). Owns the int/uint→int64, int/uint/float→float64, and int/uint/float/decimal→decimal reflect conversions. `get.go` and `table.go` delegate to it.
- `pipe/get.go` — modify: `coerceToInt64`/`coerceToFloat64` keep their string/json.Number heads, delegate the reflect cases to the kernel (R1).
- `pipe/table.go` — modify: `toInt64`/`toFloat64`/`asDecimal` become thin kernel adapters; `classify` unchanged (R1).
- `pipe/scope.go` — modify: add unexported `deriveOrSet` method (R2). Also holds the `wide` gate touched by R7 (in `pipeline.go`).
- `pipe/single.go`, `pipe/multi.go`, `pipe/table.go` — modify: route the 5 provenance write sites through `deriveOrSet` (R2).
- `pipe/pipeline.go` — modify: gate `p.wide` on `maxParallel != 1` (R7).
- `pipe/provenance.go`, `pipe/firing.go` — modify: extract a shared prefix-rekey merge helper (R8).
- `expr/predicate.go` — modify: fold `truthy`'s exact bool/string heads into the reflect switch (R3).
- `expr/decimal.go` — modify: table-drive the four arithmetic `Function` registrations (R4).
- `expr/options.go`, `expr/variables.go` — modify: collapse `copyMap` into `mergeInto` (R9).
- `config/expr_def.go` — modify: hoist the known-field set to a package var (R6).
- `config/build.go` — modify: collapse the `withStrictEnv(withConstants(…))` wrapper into one helper (R6); set the hash memo at build time (R11).
- `config/hash.go` — modify: add the `hashMemo` field + memoized `Hash()` fast path (R11).
- `engine.go`, `typed_engine.go`, `fromconfig.go` — modify: extract `newEngineConfig` + a shared parse→build helper (R5).

---

## Task 1: R1 — one overflow-checked numeric core

**Backlog:** R1. **ADR:** 0054.

**Files:**
- Create: `pipe/numeric.go`
- Modify: `pipe/get.go` (`coerceToInt64` ~195-236, `coerceToFloat64` ~243-278), `pipe/table.go` (`toInt64` ~654-664, `toFloat64` ~669-681, `asDecimal` ~688-707)
- Test: `pipe/get_test.go`, `pipe/table_test.go` (existing; add cases)

**Interfaces:**
- Produces (unexported, package `pipe`):
  - `int64FromNumeric(rv reflect.Value) (int64, error)` — overflow-checked int/uint kinds → int64; returns a plain error (`uint64(%d) overflows int64`) for `uint64 > math.MaxInt64`; `(0, errNotNumericKind)` for a non-int/uint kind.
  - `float64FromNumeric(rv reflect.Value) (float64, error)` — int/uint/float kinds → float64; `(0, errNotNumericKind)` otherwise.
  - `decimalFromNumeric(rv reflect.Value) (decimal.Decimal, bool)` — int/uint (full uint64 via `big.Int`)/finite-float kinds → decimal; `ok=false` for a non-finite float or a non-numeric kind.
  - `errNotNumericKind` — sentinel returned when a `reflect.Value` is not a supported numeric kind (callers translate it to their own message).
- Consumes: nothing from other tasks.

> **Why a kernel, not a merge:** `get.go` accepts `string`/`json.Number` and fails loud on overflow; `table.go` rejects text (via `classify`) and trusts pre-classification. The kernel is ONLY the reflect-conversion core; each caller keeps its own outer type-set policy. See ADR-0054.

- [ ] **Step 1: Characterization tests for the exact current numeric semantics (guards the refactor)**

Add to `pipe/get_test.go` (blackbox `package pipe_test`) a table exercising the coercing getters across every kernel branch. These pass against the current code — they pin the behavior the refactor must preserve.

```go
func TestGetIntCoerceKernelBranches(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		assert func(t *testing.T, got int64, err error)
	}{
		{"uint64 in range", uint64(42), func(t *testing.T, got int64, err error) {
			if err != nil || got != 42 {
				t.Fatalf("got %d, %v; want 42, nil", got, err)
			}
		}},
		{"uint64 overflows int64", uint64(math.MaxInt64) + 1, func(t *testing.T, got int64, err error) {
			var te *pipe.ScopeTypeError
			if !errors.As(err, &te) {
				t.Fatalf("want *ScopeTypeError, got %v", err)
			}
		}},
		{"int8 widens", int8(-5), func(t *testing.T, got int64, err error) {
			if err != nil || got != -5 {
				t.Fatalf("got %d, %v; want -5, nil", got, err)
			}
		}},
		{"integral float", 7.0, func(t *testing.T, got int64, err error) {
			if err != nil || got != 7 {
				t.Fatalf("got %d, %v; want 7, nil", got, err)
			}
		}},
		{"non-integral float errors", 7.5, func(t *testing.T, got int64, err error) {
			var te *pipe.ScopeTypeError
			if !errors.As(err, &te) {
				t.Fatalf("want *ScopeTypeError, got %v", err)
			}
		}},
		{"NaN errors", math.NaN(), func(t *testing.T, got int64, err error) {
			var te *pipe.ScopeTypeError
			if !errors.As(err, &te) {
				t.Fatalf("want *ScopeTypeError, got %v", err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sc := pipe.NewScope(map[string]any{"v": tc.value})
			got, err := sc.GetInt64Coerce("v")
			tc.assert(t, got, err)
		})
	}
}
```

Add to `pipe/table_test.go` a case proving `AggregateSum` promotes a `uint64 > math.MaxInt64` to an **exact decimal** (not a wrapped int64) — the bug-#3 behavior the kernel must keep:

```go
func TestAggregateSumUint64PromotesToDecimal(t *testing.T) {
	// A collect table summing a native uint64 above MaxInt64 must yield an exact
	// decimal, never a wrapped int64. Drive it through a decision-table collect.
	// (Reuse the existing collect-sum test harness in this file; assert the
	// result is a decimal.Decimal equal to the exact sum.)
}
```

> If an equivalent collect-sum harness already exists in `table_test.go`, fold this as a new row rather than a new function (per `table-test`). Fill the body using that harness; assert `reflect.TypeOf(result).String() == "decimal.Decimal"` and exact value.

- [ ] **Step 2: Run the new tests — expect PASS (characterization)**

Run: `go test ./pipe/ -run 'KernelBranches|Uint64PromotesToDecimal' -v`
Expected: PASS (they pin current behavior).

- [ ] **Step 3: Create the kernel `pipe/numeric.go`**

```go
package pipe

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"

	"github.com/shopspring/decimal"
)

// errNotNumericKind reports that a reflect.Value is not one of the supported
// numeric kinds. Callers translate it into their own contextual message (a
// *ScopeTypeError for the coercing getters, a classification-guaranteed
// unreachable for the aggregation folds).
var errNotNumericKind = errors.New("value is not a numeric kind")

// int64FromNumeric converts an integer-kind reflect.Value to int64, checking
// that a uint kind does not exceed math.MaxInt64 (which would wrap). A
// non-integer kind is errNotNumericKind. This is the single overflow-checked
// int64 conversion shared by pipe/get.go (coercing getters) and pipe/table.go
// (integer aggregation folds); its divergence previously caused pipe bug #3.
func int64FromNumeric(rv reflect.Value) (int64, error) {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := rv.Uint()
		if u > math.MaxInt64 {
			return 0, fmt.Errorf("uint64(%d) overflows int64", u)
		}
		return int64(u), nil
	default:
		return 0, errNotNumericKind
	}
}

// float64FromNumeric converts an integer- or float-kind reflect.Value to
// float64 (integer magnitudes above 2^53 may lose precision, inherent to
// float64). A non-numeric kind is errNotNumericKind.
func float64FromNumeric(rv reflect.Value) (float64, error) {
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		return rv.Float(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(rv.Uint()), nil
	default:
		return 0, errNotNumericKind
	}
}

// decimalFromNumeric converts an integer- or finite-float-kind reflect.Value to
// decimal.Decimal, preserving the full uint64 range via big.Int (int64(u) would
// wrap). ok is false for a non-finite float (NaN/±Inf — no decimal form; passing
// it to decimal.NewFromFloat panics) or a non-numeric kind.
func decimalFromNumeric(rv reflect.Value) (decimal.Decimal, bool) {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return decimal.NewFromInt(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return decimal.NewFromBigInt(new(big.Int).SetUint64(rv.Uint()), 0), true
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return decimal.Decimal{}, false
		}
		return decimal.NewFromFloat(f), true
	default:
		return decimal.Decimal{}, false
	}
}
```

- [ ] **Step 4: Delegate `pipe/get.go`'s reflect cases to the kernel**

Replace the reflect switch tail of `coerceToInt64` (the block from `rv := reflect.ValueOf(v)` onward) with a kernel call that preserves the exact existing error messages for the float path (the kernel handles int/uint; floats keep their finite/integral/overflow checks in `get.go`):

```go
func coerceToInt64(v any) (int64, error) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, fmt.Errorf("json.Number(%s) is not an integer", n.String())
		}
		return i, nil
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("string(%q) is not an integer", n)
		}
		return i, nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, fmt.Errorf("float64(%v) is not finite", f)
		}
		if f != math.Trunc(f) {
			return 0, fmt.Errorf("float64(%v) is not integral", f)
		}
		if f >= float64(math.MaxInt64) || f < float64(math.MinInt64) {
			return 0, fmt.Errorf("float64(%v) overflows int64", f)
		}
		return int64(f), nil
	default:
		i, err := int64FromNumeric(rv)
		if errors.Is(err, errNotNumericKind) {
			return 0, fmt.Errorf("%T is not numeric", v)
		}
		return i, err // nil, or the "uint64(...) overflows int64" message
	}
}
```

Replace `coerceToFloat64`'s reflect switch tail similarly (kernel handles int/uint/float; keep the string/json.Number heads and their non-finite rejection):

```go
	rv := reflect.ValueOf(v)
	f, err := float64FromNumeric(rv)
	if errors.Is(err, errNotNumericKind) {
		return 0, fmt.Errorf("%T is not numeric", v)
	}
	return f, err
```

Add `"errors"` to `get.go`'s imports if not present (it is). Remove now-unused imports (`math/big` was never in get.go; `math` stays for the float checks).

- [ ] **Step 5: Delegate `pipe/table.go`'s `toInt64`/`toFloat64`/`asDecimal` to the kernel**

`classify` stays exactly as-is. Rewrite the three converters as thin adapters that preserve the "callers guarantee classification, return zero on the unexpected" contract:

```go
func toInt64(v any) int64 {
	i, err := int64FromNumeric(reflect.ValueOf(v))
	if err != nil {
		return 0 // unreachable: classify guaranteed kindInt (uint64 <= MaxInt64)
	}
	return i
}

func toFloat64(v any) float64 {
	f, err := float64FromNumeric(reflect.ValueOf(v))
	if err != nil {
		return 0 // unreachable: classify guaranteed kindInt or kindFloat
	}
	return f
}

func asDecimal(v any) (decimal.Decimal, bool) {
	if d, ok := v.(decimal.Decimal); ok {
		return d, true
	}
	return decimalFromNumeric(reflect.ValueOf(v))
}
```

Remove `"math/big"` from `table.go`'s imports if it becomes unused (it moves to `numeric.go`); keep `"math"`, `"reflect"`, `"github.com/shopspring/decimal"` (still used by `classify`, `foldDecimal`, etc.). Run `goimports`/`gofmt` to settle imports.

- [ ] **Step 6: Run the full pipe suite with race detector**

Run: `go test ./pipe/ -race`
Expected: PASS (all existing tests + the new characterization cases).

- [ ] **Step 7: Build, vet, fmt, whole-tree race**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./... -race`
Expected: build/vet clean, `gofmt -l` prints nothing, all tests PASS.

- [ ] **Step 8: Write ADR-0054**

Create `docs/adrs/0054-unify-numeric-coercion-core.md` following Nygard (Title, Status: Accepted, Context, Decision, Consequences). Context: two diverged reflect paths (get.go vs table.go) whose divergence caused pipe bug #3. Decision: extract the overflow-checked reflect kernel (`pipe/numeric.go`) shared by both; each caller keeps its own outer type-set policy (get.go: string/json.Number + fail-loud; table.go: classify + trust). Consequences: single place to change numeric-reflection semantics; kernel is unexported (no API change); the fold path observes no new error because `classify` pre-excludes the failing cases. Cite Spec 029, Plan 029, Backlog R1, and the related ADR-0044/0038/0039.

- [ ] **Step 9: Commit**

```bash
# The plan rides with the first realizing commit (CLAUDE.md: no standalone plan commits).
git add pipe/numeric.go pipe/get.go pipe/table.go pipe/get_test.go pipe/table_test.go docs/adrs/0054-unify-numeric-coercion-core.md docs/plans/029-post-audit-refactor-batch.md
git commit -F - <<'EOF'
refactor(pipe): unify numeric coercion onto one overflow-checked kernel

Extract the int/uint→int64 (overflow-checked), int/uint/float→float64, and
int/uint/float→decimal reflect conversions into an unexported pipe/numeric.go
kernel. coerceToInt64/coerceToFloat64 (get.go) and toInt64/toFloat64/asDecimal
(table.go) now delegate to it, keeping their own outer type-set policies. This
removes the duplicated reflect switch whose divergence caused pipe bug #3. No
behavior, error-identity, or public-API change.

Spec: 029
Plan: 029
ADR: 0054
Backlog: R1

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 2: R2 — unexported `deriveOrSet` helper

**Backlog:** R2.

**Files:**
- Modify: `pipe/provenance.go` (add `deriveOrSet` near `Derive` ~48) OR `pipe/scope.go`; `pipe/single.go` (~81-97), `pipe/multi.go` (~99-113), `pipe/table.go` (`writeDecision` ~240-251, `writeCollected` ~437-448, `writeAgg` ~453-462)
- Test: existing provenance/lineage tests in `pipe/*_test.go` (green guard)

**Interfaces:**
- Produces (unexported): `func (s *Scope) deriveOrSet(path string, v any, build func() Derivation) error` — when provenance is on, calls `s.Derive(path, v, build())`; otherwise `s.Set(path, v)`. `build` is invoked ONLY when provenance is on, so `snapshotRefs` never runs when off.
- Consumes: `Scope.TracksProvenance`, `Scope.Derive`, `Scope.Set` (existing).

- [ ] **Step 1: Confirm the provenance-off path is covered (guard)**

Run the existing provenance suite both on and off to establish the green baseline:
Run: `go test ./pipe/ -run 'Provenance|Derivation|Lineage|Explain|Decision|Multi|Single' -race`
Expected: PASS. (No new test needed — the refactor is pure DRY; existing on/off coverage is the guard. If a quick check shows no test exercises a `single-expr` with provenance OFF, add one asserting `sc.Set`-equivalent output and empty `Derivations()`.)

- [ ] **Step 2: Add `deriveOrSet` to `pipe/provenance.go`**

```go
// deriveOrSet stores v at path, recording a derivation when provenance is
// tracked and plainly setting it otherwise. build is the lazy Derivation
// constructor — invoked ONLY when provenance is on, so an expensive
// snapshotRefs(...) never runs on the provenance-off hot path. It collapses the
// repeated "if TracksProvenance { Derive } else { Set }" branch across the
// single/multi/decision-table write sites.
func (s *Scope) deriveOrSet(path string, v any, build func() Derivation) error {
	if !s.provenance {
		return s.Set(path, v)
	}
	return s.Derive(path, v, build())
}
```

- [ ] **Step 3: Route `single.go` through `deriveOrSet`**

Replace the `if sc.TracksProvenance() { … } else { … }` block in `SingleExpr.Execute` (lines ~81-97) with:

```go
	if err := sc.deriveOrSet(s.output, v, func() Derivation {
		return Derivation{
			Stage:      s.name,
			StageType:  TypeSingleExpr,
			Operation:  "eval",
			Expression: s.fn.Source(),
			Inputs:     snapshotRefs(env, s.fn.References()),
		}
	}); err != nil {
		return &StageError{Stage: s.name, Type: TypeSingleExpr, Cause: err}
	}
	return nil
```

- [ ] **Step 4: Route `multi.go` through `deriveOrSet`**

`MultiExpr.Execute` records a per-expr local-alias key set (`locals`), and `snapshotRefsKeyed` depends on `m.qualifyLocal(locals)` evaluated at write time. Because `build` runs synchronously inside `deriveOrSet` (before it returns), capturing `env`, `e`, and `locals` in the closure is safe. Replace the `if tracking { … } else if … { … }` block (lines ~99-113) with:

```go
		e := e // capture loop var for the closure
		if err := sc.deriveOrSet(m.name+"."+e.name, v, func() Derivation {
			return Derivation{
				Stage:      m.name,
				StageType:  TypeMultiExpr,
				Operation:  "expr:" + e.name,
				Expression: e.fn.Source(),
				Inputs:     snapshotRefsKeyed(env, e.fn.References(), m.qualifyLocal(locals)),
			}
		}); err != nil {
			return &StageError{Stage: m.name, Type: TypeMultiExpr, Cause: err}
		}
		if tracking {
			locals[e.name] = struct{}{} // an earlier local alias for later expressions
		}
		env[e.name] = v // visible to later expressions within this stage
```

> Note: the `locals` bookkeeping stays gated on `tracking` (it is only consumed by `qualifyLocal` under provenance), and `env[e.name] = v` stays unconditional. `deriveOrSet` internally no-ops the derivation when provenance is off, so `build` (and thus `snapshotRefsKeyed`) is not called then.

- [ ] **Step 5: Route `table.go`'s three write sites through `deriveOrSet`**

`writeDecision` (needs `env`, `dec`, `op`):

```go
func (d *DecisionTable) writeDecision(env map[string]any, sc *Scope, dec compiledDecision, v any, op string) error {
	return sc.deriveOrSet(d.name+"."+dec.key, v, func() Derivation {
		return Derivation{
			Stage:      d.name,
			StageType:  TypeDecisionTable,
			Operation:  op,
			Expression: dec.fn.Source(),
			Inputs:     snapshotRefs(env, dec.fn.References()),
		}
	})
}
```

`writeCollected` (needs `op`, `exprs`, `inputs`):

```go
func (d *DecisionTable) writeCollected(sc *Scope, key string, v any, op string, exprs []string, inputs map[string]any) error {
	return sc.deriveOrSet(d.name+"."+key, v, func() Derivation {
		return Derivation{
			Stage:      d.name,
			StageType:  TypeDecisionTable,
			Operation:  op,
			Expression: strings.Join(exprs, "; "),
			Inputs:     inputs,
		}
	})
}
```

`writeAgg` (no expression/inputs):

```go
func (d *DecisionTable) writeAgg(sc *Scope, key string, v any, op string) error {
	return sc.deriveOrSet(d.name+"."+key, v, func() Derivation {
		return Derivation{
			Stage:     d.name,
			StageType: TypeDecisionTable,
			Operation: op,
		}
	})
}
```

- [ ] **Step 6: Run pipe suite + whole-tree gates**

Run: `go test ./pipe/ -race && go build ./... && go vet ./... && gofmt -l . && go test ./... -race`
Expected: all PASS; `gofmt -l` empty. Provenance ON output (paths, Inputs, Operation labels) and OFF output are byte-unchanged.

- [ ] **Step 7: Commit**

```bash
git add pipe/provenance.go pipe/single.go pipe/multi.go pipe/table.go
git commit -F - <<'EOF'
refactor(pipe): collapse the 5-site provenance branch into deriveOrSet

Add an unexported Scope.deriveOrSet(path, v, build) that records a derivation
when provenance is on and plainly Sets otherwise, with a lazy build closure so
snapshotRefs never runs on the provenance-off path. Route single/multi/table's
five write sites through it. No behavior or public-API change.

Spec: 029
Plan: 029
Backlog: R2

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 3: R7 — gate `p.wide` on `maxParallel != 1`

**Backlog:** R7.

**Files:**
- Modify: `pipe/pipeline.go` (`NewPipeline` wide-computation block ~155-164)
- Test: `pipe/pipeline_test.go` (existing concurrency tests; add a `WithMaxParallel(1)` case)

**Interfaces:**
- Consumes: `Pipeline.maxParallel`, `Pipeline.wide`, `computeLevels` (existing). Produces: no new symbol.

> **Rationale:** a size-1 parallel cap serializes stages via the semaphore, so no two stages ever overlap — yet today `wide` can still be true, marking the Scope concurrent and paying `Snapshot`'s deep-copy. Gating `wide` on `maxParallel != 1` makes a `WithMaxParallel(1)` pipeline behave like — and cost like — sequential. Behavior-preserving: output is already identical; only the wasted deep-copy is removed.

- [ ] **Step 1: Add a failing-then-green characterization test**

Add to `pipe/pipeline_test.go` (blackbox) a case proving `WithMaxParallel(1)` over two independent stages equals sequential and is race-clean:

```go
func TestPipelineMaxParallelOneEqualsSequential(t *testing.T) {
	mk := func(opts ...pipe.PipelineOption) *pipe.Scope {
		s1, _ := pipe.NewSingleExpr("a", "1 + 1")
		s2, _ := pipe.NewSingleExpr("b", "2 + 2")
		p, err := pipe.NewPipeline([]pipe.Stage{s1, s2}, opts...)
		if err != nil {
			t.Fatalf("NewPipeline: %v", err)
		}
		sc := pipe.NewScope(map[string]any{})
		if err := p.Run(t.Context(), sc); err != nil {
			t.Fatalf("Run: %v", err)
		}
		return sc
	}
	seq := mk()
	one := mk(pipe.WithMaxParallel(1))
	for _, key := range []string{"a", "b"} {
		if !reflect.DeepEqual(mustGet(t, seq, key), mustGet(t, one, key)) {
			t.Fatalf("key %q: WithMaxParallel(1) differs from sequential", key)
		}
	}
}
```

> `mustGet` is a small test helper doing `v, ok := sc.Get(key); if !ok { t.Fatal }`; reuse the existing one in `pipeline_test.go` if present, else add it.

- [ ] **Step 2: Run — expect PASS (behavior already correct)**

Run: `go test ./pipe/ -run MaxParallelOneEqualsSequential -race`
Expected: PASS (this pins behavior; the refactor removes the wasted deep-copy without changing it).

- [ ] **Step 3: Gate the `wide` computation**

In `NewPipeline`, change the wide-detection block so a bounded cap of exactly 1 never marks the pipeline wide:

```go
	p := &Pipeline{ordered: ordered, ruleset: cfg.ruleset, maxParallel: maxParallel}
	// maxParallel == 1 serializes stages via the semaphore, so no two ever
	// overlap; skip the wide (concurrent-Snapshot deep-copy) path in that case.
	if maxParallel != 0 && maxParallel != 1 {
		p.levels = computeLevels(ordered)
		for _, lvl := range p.levels {
			if len(lvl) > 1 {
				p.wide = true
				break
			}
		}
	} else if maxParallel == 1 {
		p.levels = computeLevels(ordered) // waves still run (maxParallel != 0), just not wide
	}
	return p, nil
```

> `runWaves` requires `p.levels`, so it must still be computed for `maxParallel == 1` (the runner path is taken because `maxParallel != 0`). Only `p.wide` is suppressed, so `Run` skips `sc.markConcurrent()` and `Snapshot` stays shallow.

- [ ] **Step 4: Run pipe race suite (includes the shared-nested-map guard)**

Run: `go test ./pipe/ -race`
Expected: PASS — including `TestPipelineConcurrencyNoSharedNestedMapRace` (unaffected: it uses real parallelism, `maxParallel > 1` or unbounded) and the new `MaxParallelOne` case.

- [ ] **Step 5: Whole-tree gates + commit**

```bash
go build ./... && go vet ./... && gofmt -l . && go test ./... -race
git add pipe/pipeline.go pipe/pipeline_test.go
git commit -F - <<'EOF'
refactor(pipe): don't mark a WithMaxParallel(1) pipeline wide

A size-1 cap serializes stages via the semaphore, so no two overlap; suppress
the wide flag (and thus the concurrent-Snapshot deep-copy) when maxParallel == 1.
Levels are still computed so the wave runner works. Output is unchanged; only the
needless per-stage deep-copy is removed.

Spec: 029
Plan: 029
Backlog: R7

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 4: R8 — shared prefix-rekey merge helper

**Backlog:** R8 (P4).

**Files:**
- Modify: `pipe/provenance.go` (`recordElementDerivations` ~70-88), `pipe/firing.go` (`recordElementFirings` ~59-68)
- Test: existing foreach per-element lineage/firing tests (green guard)

**Interfaces:**
- Produces (unexported): `func prefixKey(prefix, key string) string` returning `prefix + "." + key`; and a generic `func mergePrefixed[V, W any](dst map[string]W, prefix string, src map[string]V, transform func(key string, v V) W)` that copies each `src` entry into `dst` under `prefixKey(prefix, key)` applying `transform`.
- Consumes: nothing from other tasks.

> **Scope note (P4):** the shared surface is thin (lock + nil-init stay per-method; only the loop+rekey is shared). Implement the generic helper below. If, in practice, it reads as over-abstracted (the executor's judgment), fall back to extracting only `prefixKey` and keep the two loops — and record that choice in the commit body. Do not expand scope beyond these two call sites.

- [ ] **Step 1: Establish the green guard**

Run: `go test ./pipe/ -run 'ForEach|Element|Firing|Lineage' -race`
Expected: PASS (existing per-element derivation/firing tests are the guard).

- [ ] **Step 2: Add the helpers to `pipe/provenance.go`**

```go
// prefixKey joins prefix and key with a dot; the composite-key convention used
// when merging a per-element scope's derivations/firings into an outer scope.
func prefixKey(prefix, key string) string { return prefix + "." + key }

// mergePrefixed copies each src entry into dst under prefixKey(prefix, key),
// applying transform to the value. dst must be non-nil and is mutated in place.
// Shared by recordElementDerivations and recordElementFirings; the caller holds
// the lock and pre-allocates dst.
func mergePrefixed[V, W any](dst map[string]W, prefix string, src map[string]V, transform func(key string, v V) W) {
	for k, v := range src {
		dst[prefixKey(prefix, k)] = transform(k, v)
	}
}
```

- [ ] **Step 3: Rewrite `recordElementDerivations` to use `mergePrefixed`**

```go
func (s *Scope) recordElementDerivations(prefix string, src map[string]Derivation) {
	if !s.provenance || len(src) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	mergePrefixed(s.derivations, prefix, src, func(_ string, d Derivation) Derivation {
		nd := d
		nd.Path = prefixKey(prefix, d.Path)
		if len(d.Inputs) > 0 {
			ins := make(map[string]any, len(d.Inputs))
			for k, v := range d.Inputs {
				ins[prefixKey(prefix, k)] = v
			}
			nd.Inputs = ins
		}
		return nd
	})
}
```

- [ ] **Step 4: Rewrite `recordElementFirings` to use `mergePrefixed`**

```go
func (s *Scope) recordElementFirings(prefix string, src map[string][]FiringRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.firing == nil {
		s.firing = make(map[string][]FiringRule, len(src))
	}
	mergePrefixed(s.firing, prefix, src, func(_ string, rules []FiringRule) []FiringRule { return rules })
}
```

> `recordElementDerivations`'s `s.derivations` is always non-nil when `s.provenance` (allocated in `NewScope`), so no nil-init is needed there; `recordElementFirings` keeps its existing nil-init.

- [ ] **Step 5: Run + gates + commit**

```bash
go test ./pipe/ -race && go build ./... && go vet ./... && gofmt -l . && go test ./... -race
git add pipe/provenance.go pipe/firing.go
git commit -F - <<'EOF'
refactor(pipe): share the prefix-rekey merge for element derivations/firings

Extract prefixKey + a generic mergePrefixed helper; recordElementDerivations and
recordElementFirings now share the loop-and-rekey, keeping their own lock and
per-entry transform. No behavior change.

Spec: 029
Plan: 029
Backlog: R8

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 5: R3 — fold `truthy`'s exact bool/string heads into the reflect switch

**Backlog:** R3.

**Files:**
- Modify: `expr/predicate.go` (`truthy` ~87-128)
- Test: existing lenient-truthiness table in `expr/predicate_test.go` (green guard)

**Interfaces:** none new.

> **Rationale:** the exact `case bool` / `case string` heads became redundant once the audit added the `reflect.Bool` / `reflect.String` cases (a native `bool`/`string` also matches its reflect kind, with identical bodies). Removing the heads leaves one switch handling both native and named types.

- [ ] **Step 1: Confirm the truthiness table covers native + named bool/string**

Inspect `expr/predicate_test.go`. Ensure the lenient-truthiness table has cases for: native `true`/`false`, native string `"true"`/`"1"`/`""`/`"x"`, and a `json.Number` ("1"/"0"). If a native-`bool` and native-`string` case is missing, add rows (they pass now):

```go
// rows to ensure exist in the existing WithCoerce truthiness table:
{name: "native bool true", value: true, assertTrue: true},
{name: "native string empty", value: "", assertTrue: false},
{name: "native string non-bool non-empty", value: "hello", assertTrue: true},
```

> Match the existing table's field/closure shape (assert-closure form). Drive via a `WithCoerce` predicate over `{"v": value}` evaluating `v`.

Run: `go test ./expr/ -run Truthy -v` (or the actual test name); Expected: PASS.

- [ ] **Step 2: Remove the exact heads**

Delete the `switch x := v.(type) { case bool: … case string: … }` block (lines ~91-100), keeping the leading `if v == nil { return false, nil }`. The `reflect.Bool` and `reflect.String` cases already handle both native and named types identically. Result:

```go
func truthy(v any) (bool, error) {
	if v == nil {
		return false, nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool(), nil
	case reflect.String:
		s := strings.TrimSpace(rv.String())
		if b, err := strconv.ParseBool(s); err == nil {
			return b, nil
		}
		return s != "", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() != 0, nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		return f != 0 && !math.IsNaN(f) && !math.IsInf(f, 0), nil
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() > 0, nil
	default:
		return false, fmt.Errorf("%w: cannot coerce %T to bool", ErrNotBool, v)
	}
}
```

Update the `reflect.Bool`/`reflect.String` case comments (they no longer contrast with an exact head — reword to "native or named bool/string, and json.Number for String"). Imports are unchanged (`strconv`, `strings`, `math`, `reflect`, `fmt` all still used).

- [ ] **Step 3: Run + gates + commit**

```bash
go test ./expr/ -race && go build ./... && go vet ./... && gofmt -l . && go test ./... -race
git add expr/predicate.go expr/predicate_test.go
git commit -F - <<'EOF'
refactor(expr): fold truthy's exact bool/string heads into the reflect switch

The exact case bool / case string heads duplicate the reflect.Bool / reflect.String
cases added during the audit (native values match their kind with identical bodies).
Remove the heads; behavior is unchanged.

Spec: 029
Plan: 029
Backlog: R3

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 6: R4 — table-drive `decimalExprOptions` add/sub/mul/div

**Backlog:** R4.

**Files:**
- Modify: `expr/decimal.go` (`decimalExprOptions` ~37-95, the four `addDecimal…divDecimal` registrations ~59-74)
- Test: existing decimal-arithmetic expression tests (green guard)

**Interfaces:** none new. Same registered function names (`addDecimal`/`subDecimal`/`mulDecimal`/`divDecimal`) and operator overloads must remain, so compiled programs are byte-identical.

- [ ] **Step 1: Confirm decimal arithmetic is covered**

Run: `go test ./expr/ -run Decimal -race`
Expected: PASS (existing `decimal(...)` + `+ - * /` expression tests). If no test exercises all four operators over `decimal×decimal`, `decimal×int`, `int×decimal`, add rows to the existing decimal table covering each.

- [ ] **Step 2: Replace the four registrations with a table loop**

Inside `decimalExprOptions`, after the `dd :=` helper and the `decimal(...)` constructor registration, replace the four explicit `addDecimal…divDecimal` `exprlang.Function(...)` calls with a loop over a small table (the operator-overload `exprlang.Operator(...)` lines stay as-is):

```go
	opts := []exprlang.Option{
		exprlang.Function("decimal", func(p ...any) (any, error) { return toDecimal(p[0]) },
			new(func(string) decimal.Decimal),
			new(func(int) decimal.Decimal),
			new(func(float64) decimal.Decimal),
			new(func(decimal.Decimal) decimal.Decimal)),
	}

	// + - * / for decimal×decimal, decimal×int, int×decimal. Registered from a
	// table since the four differ only in name and the wrapped decimal method.
	arith := []struct {
		name string
		op   func(a, b decimal.Decimal) decimal.Decimal
	}{
		{"addDecimal", decimal.Decimal.Add},
		{"subDecimal", decimal.Decimal.Sub},
		{"mulDecimal", decimal.Decimal.Mul},
		{"divDecimal", decimal.Decimal.Div},
	}
	for _, a := range arith {
		opts = append(opts, exprlang.Function(a.name, dd(a.op),
			new(func(decimal.Decimal, decimal.Decimal) decimal.Decimal),
			new(func(decimal.Decimal, int) decimal.Decimal),
			new(func(int, decimal.Decimal) decimal.Decimal)))
	}

	opts = append(opts,
		exprlang.Operator("+", "addDecimal"),
		exprlang.Operator("-", "subDecimal"),
		exprlang.Operator("*", "mulDecimal"),
		exprlang.Operator("/", "divDecimal"),

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
	)
	return opts
```

> The registration order (decimal ctor → arith functions → operators → round/roundBank) matches the original, so option assembly is equivalent.

- [ ] **Step 3: Run + gates + commit**

```bash
go test ./expr/ -race && go build ./... && go vet ./... && gofmt -l . && go test ./... -race
git add expr/decimal.go
git commit -F - <<'EOF'
refactor(expr): table-drive the decimal add/sub/mul/div registrations

The four arithmetic Function registrations differed only in name and the wrapped
decimal method; drive them from a table. Same names, overloads, and order, so
compiled programs are unchanged.

Spec: 029
Plan: 029
Backlog: R4

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 7: R9 — collapse `copyMap` into `mergeInto`

**Backlog:** R9 (P4).

**Files:**
- Modify: `expr/variables.go` (`copyMap` ~39-45, `newPatcher` ~30-35), `expr/options.go` (`mergeInto` ~62-73 stays)
- Test: existing variable-patcher tests (green guard)

**Interfaces:** `mergeInto(dst, src map[string]any) map[string]any` (existing, `expr/options.go`) becomes the single map-copy helper. `copyMap` is deleted.

> **Equivalence:** `copyMap(m)` allocates a fresh map and copies `m`. `mergeInto(nil, m)` returns `nil` for an empty `m` (vs `copyMap`'s empty non-nil map) but is otherwise identical. `variablePatcher.lookup` reads `v.locals[name]`/`v.globals[name]`, which is safe on a nil map, so the nil-vs-empty difference is unobservable. `newPatcher` already guards that at least one of globals/locals is non-empty.

- [ ] **Step 1: Confirm no other caller of `copyMap`**

Run: `grep -rn 'copyMap' --include='*.go' .`
Expected: only `expr/variables.go` (definition + the two `newPatcher` calls). If any other caller exists, STOP and reassess (the equivalence argument only covers the patcher's use).

- [ ] **Step 2: Establish the green guard**

Run: `go test ./expr/ -run 'Patch|Variable|Global|Local' -race`
Expected: PASS.

- [ ] **Step 3: Delete `copyMap`; use `mergeInto(nil, …)` in `newPatcher`**

In `expr/variables.go`, delete the `copyMap` function (lines ~37-45) and rewrite `newPatcher`:

```go
func newPatcher(globals, locals map[string]any) *variablePatcher {
	if len(globals) == 0 && len(locals) == 0 {
		return nil
	}
	return &variablePatcher{globals: mergeInto(nil, globals), locals: mergeInto(nil, locals)}
}
```

`mergeInto` is in the same package (`expr/options.go`), so no import change. Remove any now-unused imports from `variables.go` if `copyMap`'s removal orphaned one (it does not — `reflect`/`math`/`ast`/`decimal` all remain used).

- [ ] **Step 4: Run + gates + commit**

```bash
go test ./expr/ -race && go build ./... && go vet ./... && gofmt -l . && go test ./... -race
git add expr/variables.go
git commit -F - <<'EOF'
refactor(expr): collapse copyMap into mergeInto

copyMap duplicated mergeInto's copy loop; newPatcher now uses mergeInto(nil, m).
The nil-vs-empty-map difference for an empty input is unobservable (lookup reads a
possibly-nil map safely, and newPatcher already guards all-empty). No behavior change.

Spec: 029
Plan: 029
Backlog: R9

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 8: R6 — hoist known-field map; collapse the option-wrapper

**Backlog:** R6.

**Files:**
- Modify: `config/expr_def.go` (known-field map at `UnmarshalYAML` ~33 and `UnmarshalJSON` ~66), `config/build.go` (the `withStrictEnv(withConstants(…))` sites: `buildSingle` ~206-208 & ~223, `buildMulti` ~255, `buildTable` ~283, `decisionsFrom` ~402)
- Test: existing config parse/build tests (green guard)

**Interfaces:**
- Produces (unexported, `config`): `exprEnvOpts(constants, schema map[string]any, strict bool, opts []expr.Option) []expr.Option` = `withStrictEnv(strict, schema, withConstants(constants, opts))`. Package var `knownExprDefFields = map[string]bool{"expr": true, "fallback": true, "globals": true, "coerce": true}`.
- Consumes: existing `withConstants`, `withStrictEnv`.

- [ ] **Step 1: Green guard**

Run: `go test ./config/ -race`
Expected: PASS.

- [ ] **Step 2: Hoist the known-field set in `config/expr_def.go`**

Add a package var and use it in both unmarshalers:

```go
// knownExprDefFields is the set of accepted keys in an ExprDef object form,
// checked before decoding so an unknown key is attributed to Field "expr"
// rather than surfacing as an unattributed stdlib error.
var knownExprDefFields = map[string]bool{"expr": true, "fallback": true, "globals": true, "coerce": true}
```

In `UnmarshalYAML`, replace the local `known := map[string]bool{…}` (line ~33) so the loop reads `if k := value.Content[i].Value; !knownExprDefFields[k] {`. In `UnmarshalJSON`, replace the local `known := map[string]bool{…}` (line ~66) so it reads `if !knownExprDefFields[k] {`.

- [ ] **Step 3: Add `exprEnvOpts` in `config/build.go` and apply it at the 5 sites**

Add the helper near `withConstants`/`withStrictEnv`:

```go
// exprEnvOpts composes the pipeline env onto a sub-expression's own options:
// prepend the pipeline constants (overridable defaults) then, when strict,
// append the schema strict-env type-check. It is withStrictEnv ∘ withConstants,
// the combination every build site applies.
func exprEnvOpts(constants, schema map[string]any, strict bool, opts []expr.Option) []expr.Option {
	return withStrictEnv(strict, schema, withConstants(constants, opts))
}
```

Replace each `withStrictEnv(strict, schema, withConstants(constants, X))` with `exprEnvOpts(constants, schema, strict, X)`:
- `buildSingle`: `condOpts := exprEnvOpts(constants, schema, strict, sd.condOptions())`; the `pipe.WithExprOptions(exprEnvOpts(constants, schema, strict, sd.Expr.options())...)`; and inside the error-attribution branch, `expr.NewFunction(sd.Name, sd.Expr.Expr, exprEnvOpts(constants, schema, strict, sd.Expr.options())...)`.
- `buildMulti`: `Options: exprEnvOpts(constants, schema, strict, e.Expr.options())`.
- `buildTable`: `ConditionOptions: exprEnvOpts(constants, schema, strict, r.Condition.options())`.
- `decisionsFrom`: `Options: exprEnvOpts(constants, schema, strict, ed.options())`.

Leave `withConstants` and `withStrictEnv` in place (now called only by `exprEnvOpts`) — do not inline them.

- [ ] **Step 4: Run + gates + commit**

```bash
go test ./config/ -race && go build ./... && go vet ./... && gofmt -l . && go test ./... -race
git add config/expr_def.go config/build.go
git commit -F - <<'EOF'
refactor(config): hoist known-field set; collapse the env-options wrapper

Move the ExprDef known-field map to a package var (was rebuilt per unmarshal),
and wrap the repeated withStrictEnv(withConstants(...)) into one exprEnvOpts helper
applied at all five build sites. No behavior change.

Spec: 029
Plan: 029
Backlog: R6

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 9: R11 — memoize the content hash

**Backlog:** R11 (P4).

**Files:**
- Modify: `config/hash.go` (add `hashMemo` field to `PipelineDef` — the struct is defined elsewhere in the package; add the field there and the fast path in `Hash` ~176-184), `config/build.go` (`Build` ~77: set the memo after computing `hash`)
- Test: `config/hash_test.go` (existing; add a stable-repeat case)

**Interfaces:**
- Produces (unexported): `PipelineDef.hashMemo *string` — the Build-computed hash, `nil` until Build sets it.
- Consumes: existing `hashCanonical`, `Hash`, `Build`.

> **Design (race-free, copy-safe):** only the **Build path** memoizes. Build is single-threaded construction; it already computes `hash` at `build.go:77`, so it sets `d.hashMemo = &hash` there, after `hydrateConstants` (so the memo reflects the hydrated def — `canonicalJSON` normalizes decimals, so the value is identical pre/post hydration). `Hash()` returns `*d.hashMemo` when non-nil. A hand-built def whose `Hash()` is called WITHOUT `Build` computes fresh each call (unchanged — no lazy memo, so no data race under concurrent `Hash()`). The field is a plain `*string`, so `canonicalJSON`'s `canonical := *d` copy triggers no `copylocks` vet warning, and being unexported it is ignored by `json.Marshal`.

- [ ] **Step 1: Add a stable-hash characterization test**

Add to `config/hash_test.go` (blackbox `package config_test`):

```go
func TestBuildMemoizedHashIsStableAndEqual(t *testing.T) {
	def, err := config.Parse(t.Context(), config.FromYAMLString(sampleRulesetYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	pre := def.Hash() // computed fresh (pre-Build)
	if _, err := def.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	post1 := def.Hash() // memoized after Build
	post2 := def.Hash()
	if pre != post1 || post1 != post2 {
		t.Fatalf("hash not stable across Build/memoization: pre=%s post1=%s post2=%s", pre, post1, post2)
	}
	if !def.MatchesRuleset(pipe.RulesetIdentity{Hash: post1}) {
		t.Fatalf("MatchesRuleset false for the def's own hash")
	}
}
```

> Use an existing sample YAML in `config/hash_test.go` (or a minimal single-stage ruleset). The key assertion: the memoized post-Build hash equals the pre-Build freshly-computed hash (byte-equal) and is stable across repeated calls.

- [ ] **Step 2: Run — expect PASS (pre-Build == post-Build already; this pins it)**

Run: `go test ./config/ -run MemoizedHashIsStable -v`
Expected: PASS.

- [ ] **Step 3: Add the `hashMemo` field**

Locate the `PipelineDef` struct definition (grep `type PipelineDef struct`) and add an unexported field (it is not serialized — no tag):

```go
	// hashMemo caches the Build-computed content hash so repeated Hash()/
	// MatchesRuleset calls do not re-run canonicalJSON's deep walk. Set once by
	// Build (single-threaded construction), read-only after; nil for a def that
	// has not been Built, which then computes Hash() fresh each call. A plain
	// pointer (not sync.Once/atomic) so canonicalJSON's `canonical := *d` copy
	// triggers no copylocks vet warning.
	hashMemo *string
```

- [ ] **Step 4: Fast-path `Hash()`; memoize in `Build`**

In `config/hash.go`, add the fast path at the top of `Hash()`:

```go
func (d *PipelineDef) Hash() string {
	if d.hashMemo != nil {
		return *d.hashMemo
	}
	h, err := d.hashCanonical()
	if err != nil {
		sum := sha256.Sum256([]byte("{}"))
		return hex.EncodeToString(sum[:])
	}
	return h
}
```

> Do NOT lazily set `d.hashMemo` inside `Hash()` — a non-Built def may be hashed concurrently, and an unsynchronized pointer write would race. Only Build (single-threaded) memoizes.

In `config/build.go` `Build`, after the existing `hash, err := d.hashCanonical()` success check (line ~77-80), set the memo before constructing the pipeline:

```go
	hash, err := d.hashCanonical()
	if err != nil {
		return nil, &ConfigError{Cause: fmt.Errorf("%w: %v", ErrUnhashableDef, err)}
	}
	d.hashMemo = &hash // memoize for later Hash()/MatchesRuleset on this def
```

- [ ] **Step 5: Run config suite (incl. existing hash tests) + whole-tree**

Run: `go test ./config/ -race && go build ./... && go vet ./... && gofmt -l . && go test ./... -race`
Expected: PASS. Confirm `go vet` reports no `copylocks` on `PipelineDef` (the plain `*string` field is copy-safe).

- [ ] **Step 6: Commit**

```bash
git add config/hash.go config/build.go config/hash_test.go
git commit -F - <<'EOF'
refactor(config): memoize the Build-computed content hash

Build already computes the canonical hash; store it in an unexported
PipelineDef.hashMemo (*string) so later Hash()/MatchesRuleset skip re-walking the
def via canonicalJSON. Only the single-threaded Build path memoizes; a hand-built
def hashed without Build still computes fresh (no lazy write, no race). Hash value
is byte-identical; the ADR-0045 unhashable fallback is preserved.

Spec: 029
Plan: 029
Backlog: R11

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
```

---

## Task 10: R5 — extract `newEngineConfig` + shared parse→build helper

**Backlog:** R5.

**Files:**
- Modify: `engine.go` (`New` ~62-74, add `newEngineConfig`), `typed_engine.go` (`NewTypedEngine` ~20-35), `fromconfig.go` (`NewFromProvider` ~17-33, `NewTypedFromProvider` ~48-67)
- Test: existing engine-construction + convenience-constructor tests (green guard)

**Interfaces:**
- Produces (unexported, package `rlng`):
  - `newEngineConfig(opts []Option) *engineConfig` — applies options into a fresh `engineConfig`.
  - `parseAndBuild(ctx context.Context, p config.Provider, cfg *engineConfig) (*pipe.Pipeline, error)` — runs `config.Parse(ctx, p)` then `def.Build(cfg.buildOpts...)`, returning the pipeline (parse/build errors unwrapped).
- Consumes: existing `engineConfig`, `config.Parse`, `PipelineDef.Build`.

- [ ] **Step 1: Green guard**

Run: `go test ./... -run 'New|Engine|FromYAML|FromProvider|Typed' -race`
Expected: PASS.

- [ ] **Step 2: Add `newEngineConfig` in `engine.go`**

```go
// newEngineConfig applies opts into a fresh engineConfig. Shared by New,
// NewTypedEngine, and the NewFrom* constructors so option handling lives in one
// place.
func newEngineConfig(opts []Option) *engineConfig {
	cfg := &engineConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}
```

Rewrite `New` to use it:

```go
func New(pipeline *pipe.Pipeline, opts ...Option) (*Engine, error) {
	if pipeline == nil {
		return nil, ErrNilPipeline
	}
	cfg := newEngineConfig(opts)
	if len(cfg.buildOpts) > 0 {
		return nil, ErrConcurrencyRequiresConfig
	}
	return &Engine{pipeline: pipeline, scopeOpts: cfg.scopeOpts}, nil
}
```

- [ ] **Step 3: Use `newEngineConfig` in `NewTypedEngine`**

In `typed_engine.go`, replace the `cfg := &engineConfig{}; for _, o := range opts { o(cfg) }` block with `cfg := newEngineConfig(opts)`:

```go
	if mapper == nil {
		return nil, ErrNilMapper
	}
	cfg := newEngineConfig(opts)
	if len(cfg.buildOpts) > 0 {
		return nil, ErrConcurrencyRequiresConfig
	}
	return &TypedEngine[I, R]{pipeline: pipeline, mapper: mapper, scopeOpts: cfg.scopeOpts}, nil
```

- [ ] **Step 4: Add `parseAndBuild` and use it in both `NewFrom*Provider`**

In `fromconfig.go`, add the helper and rewrite both constructors:

```go
// parseAndBuild parses p and builds the pipeline with cfg's build options. It is
// the shared body of NewFromProvider/NewTypedFromProvider. Parse and build errors
// are returned unwrapped (a *config.ConfigError).
func parseAndBuild(ctx context.Context, p config.Provider, cfg *engineConfig) (*pipe.Pipeline, error) {
	def, err := config.Parse(ctx, p)
	if err != nil {
		return nil, err
	}
	return def.Build(cfg.buildOpts...)
}

func NewFromProvider(ctx context.Context, p config.Provider, opts ...Option) (*Engine, error) {
	cfg := newEngineConfig(opts)
	pipeline, err := parseAndBuild(ctx, p, cfg)
	if err != nil {
		return nil, err
	}
	// Construct directly (not via New): the concurrency Option was consumed into
	// the build, so passing it to New would trip ErrConcurrencyRequiresConfig.
	return &Engine{pipeline: pipeline, scopeOpts: cfg.scopeOpts}, nil
}

func NewTypedFromProvider[I, R any](ctx context.Context, p config.Provider, mapper *Mapper[R], opts ...Option) (*TypedEngine[I, R], error) {
	if mapper == nil {
		return nil, ErrNilMapper
	}
	cfg := newEngineConfig(opts)
	pipeline, err := parseAndBuild(ctx, p, cfg)
	if err != nil {
		return nil, err
	}
	// Construct directly (not via NewTypedEngine): concurrency was consumed into
	// the build, so it would otherwise trip ErrConcurrencyRequiresConfig.
	return &TypedEngine[I, R]{pipeline: pipeline, mapper: mapper, scopeOpts: cfg.scopeOpts}, nil
}
```

Add `"github.com/kartaladev/rlng/pipe"` to `fromconfig.go`'s imports (needed for `*pipe.Pipeline` in `parseAndBuild`'s signature). Keep the doc comments on the two constructors (unchanged above the funcs).

- [ ] **Step 5: Run + gates**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./... -race`
Expected: all PASS; `gofmt -l` empty.

- [ ] **Step 6: Commit**

```bash
git add engine.go typed_engine.go fromconfig.go
git commit -F - <<'EOF'
refactor(rlng): extract newEngineConfig + shared parseAndBuild

Deduplicate the option-config assembly (New, NewTypedEngine, NewFrom*Provider)
into newEngineConfig, and the Parse→Build wiring (NewFromProvider,
NewTypedFromProvider) into parseAndBuild. No behavior or public-API change.

Spec: 029
Plan: 029
Backlog: R5

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
```

---

## Final delivery gate (after Task 10, before any merge/push)

Not a code task — the whole-branch pre-merge gate from CLAUDE.md §5.

- [ ] Run `/code-review` over the whole-branch diff (`main..HEAD`); resolve or triage (with written rationale) every finding; re-run affected reviews after fixes.
- [ ] Run `/security-review` over `main..HEAD`; resolve/triage every finding.
- [ ] Confirm the test-coverage gate: `go test ./... -cover` — changed packages (`pipe`, `expr`, `config`, root `rlng`) at/above ~85%; every newly-reachable branch (R1 kernel overflow/decimal, R7 maxParallel==1, R11 memo) has a covering case.
- [ ] Re-run `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), `go test ./... -race` — all green.
- [ ] Confirm `apidiff`/`gorelease` against the last tag reports **no exported-surface change** (all new symbols are unexported).
- [ ] Update `docs/BACKLOG.md`: move R1–R9, R11 to a "Resolved" subsection (closing increment 029 / ADR-0054 for R1); leave R10 open. Update `docs/HANDOVER.md`. These doc edits ride in the final task's commit or a trailing `docs:` commit per CLAUDE.md.
- [ ] Only then request approval to merge to `main` and delete the branch.

## Self-Review

**Spec coverage:** R1→Task 1, R2→Task 2, R7→Task 3, R8→Task 4, R3→Task 5, R4→Task 6, R9→Task 7, R6→Task 8, R11→Task 9, R5→Task 10. All ten in-scope items have a task; R10 is explicitly out of scope. Success-criteria 1–7 map to Tasks 1/2/3/9 assertions + the final gate. No gaps.

**Placeholder scan:** the R1 `AggregateSum` decimal-promotion test body is described (reuse the file's collect-sum harness) rather than fully spelled out because the harness shape is file-local; every other step carries complete code. No "TBD"/"handle edge cases"/"similar to" placeholders.

**Type consistency:** `deriveOrSet(path string, v any, build func() Derivation) error`, kernel signatures (`int64FromNumeric`/`float64FromNumeric`/`decimalFromNumeric`, `errNotNumericKind`), `mergePrefixed[V,W]`/`prefixKey`, `exprEnvOpts`/`knownExprDefFields`, `hashMemo *string`, `newEngineConfig`/`parseAndBuild` — each defined once and used consistently across tasks.
