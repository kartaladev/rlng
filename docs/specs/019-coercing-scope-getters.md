# Spec 019 — numeric-coercing Scope getters

- **Status:** Draft
- **Backlog item:** B3 (`docs/BACKLOG.md`) — the "coercing Scope getters" deferral (Spec 006 non-goal;
  `pipe/get.go`).
- **Realized by:** Plan 019; ADR-0044.

## Problem

The typed `Scope` getters (`GetInt`/`GetInt64`/`GetFloat64` in `pipe/get.go`) are **strict**: they accept
only the exact target type (plus the lossless `json.Number` / `int64` shapes a JSON round-trip produces).
A `float64` at an int path, an `int` at a float path, or a numeric **string** is a `*ScopeTypeError`. That
is the right *default* — the strict contract is honest and debuggable. But callers frequently read values
that arrived through a loosely-typed edge (untyped JSON, a CSV/form field, an `expr` result that widened an
int to `float64`) and want a **best-effort numeric read** without hand-writing the type switch each time.
ADR-0040-era work recorded this as additive: a coercing variant can be added **without touching the strict
API**.

## Goal

Add **coercing numeric getters** — `GetIntCoerce`, `GetInt64Coerce`, `GetFloat64Coerce` — that accept a
wider set of stored types and convert them **safely and honestly** (ADR-0035's coercion philosophy: never
silently corrupt, never manufacture `NaN`/`±Inf`, fail loud with a typed error on a value that cannot be
converted). The strict getters are unchanged; coercion is purely opt-in additional surface.

## Decisions

- **D1 — Separate methods, not an option (API shape).** Add three new methods `GetIntCoerce` /
  `GetInt64Coerce` / `GetFloat64Coerce`, each mirroring its strict sibling's signature
  (`(path string) (T, error)`). Rationale: the strict getters are simple accessors, and a parallel method
  with its own godoc-documented coercion matrix is more discoverable and readable than a variadic
  `...GetOption` that changes every getter's signature. The `Coerce` suffix aligns with the codebase's
  existing `WithCoerce` predicate vocabulary (ADR-0035). *Alternative considered:* a `WithCoerce()` getter
  option (consistency with predicates) — rejected as heavier for a plain accessor and it would touch the
  strict signatures. *Scope:* only the three numeric getters (the backlog's named gap). `GetString`/
  `GetBool`/`GetSlice`/`GetMap` coercion is a **non-goal** (bool coercion already lives in the predicate
  `truthy` path per ADR-0035; string/slice/map coercion is lossy/ambiguous and unrequested).
- **D2 — Reuse `*ScopeTypeError` and `ErrPathNotFound`.** No new error type. A missing path stays
  `ErrPathNotFound`; an unconvertible value is a `*ScopeTypeError` whose `Expected` is the target
  (`"int"`/`"int64"`/`"float64"`) and whose `Actual` states the concrete reason (e.g.
  `float64(3.14) not integral`, `string("abc") not numeric`, `uint64(…) overflows int64`), matching how the
  strict `GetInt` already reports its int64→int overflow.
- **D3 — Integer-target coercion matrix** (`GetIntCoerce`, `GetInt64Coerce`):
  - Any signed/unsigned integer kind (`int…int64`, `uint…uint64`) → target, **overflow-checked** (a
    `uint64` above `MaxInt64`, or an `int64` above `MaxInt`/below `MinInt` on a 32-bit `int` target, is a
    `*ScopeTypeError`, never a silent wrap — golang-safety numeric rule).
  - `float32`/`float64` → target **iff integral and in range**; a non-integral float (`3.14`) is a
    `*ScopeTypeError` (**no silent truncation** — honest), and `NaN`/`±Inf`/out-of-range → `*ScopeTypeError`.
  - `json.Number` → `.Int64()` (lossless or error), then range-checked to the target.
  - `string` → `strconv.ParseInt(s, 10, 64)` then range-checked; a parse failure is a `*ScopeTypeError`.
  - `bool`, `nil`, slice, map, any other kind → `*ScopeTypeError`.
- **D4 — Float-target coercion matrix** (`GetFloat64Coerce`):
  - `float32`/`float64` → `float64` **pass-through** (a value already stored as a float is returned as the
    strict getter would — coercion widens *types*, it does not re-validate a caller-stored value, so a
    stored `NaN` is returned unchanged, consistent with strict `GetFloat64`).
  - Any integer kind → `float64` (widening; godoc **documents** that magnitudes above 2^53 may lose
    precision — inherent to `float64`, the accepted cost of coercion).
  - `json.Number` → `.Float64()`.
  - `string` → `strconv.ParseFloat(s, 64)`; a parse failure is a `*ScopeTypeError`, **and a non-finite
    result (`"NaN"`, `"Inf"`) is rejected as a `*ScopeTypeError`** — coercion never *manufactures* a
    non-finite value from text (ADR-0035 treats `NaN`/`±Inf` as invalid business values).
  - `bool`, `nil`, slice, map, any other kind → `*ScopeTypeError`.
- **D5 — Blackbox tests, table-extended.** Extend `pipe/get_test.go` (external `package pipe_test`) with
  table cases driving the new methods through the public API. Every branch in D3/D4 (each accepted source
  kind and each error branch) gets a covering case (hot-path + typed-error coverage gate).
- **D6 — Docs.** Each new method carries a godoc comment stating its exact coercion matrix and the
  fail-loud rules; ADR-0044 records the design and its ADR-0035 alignment.

## Non-goals

- Coercing `GetString`/`GetBool`/`GetSlice`/`GetMap` (bool coercion is the predicate `truthy` path,
  ADR-0035; the rest are lossy/unrequested).
- Changing or deprecating any strict getter — strictness stays the default and the unchanged contract.
- Locale-aware or thousands-separator numeric parsing (`"1,234"`); only Go's `strconv` grammar.
- A generic `GetAsCoerce[T]` — the three concrete numeric methods are the whole surface.

## Success criteria / hot-path branches to cover

1. int/uint-kind → int/int64 (incl. a `uint64` that fits, and one that overflows → error).
2. integral `float64` → int/int64; non-integral `float64` → error; `NaN`/`±Inf` float → error.
3. `json.Number` (integer) → int/int64; non-integer `json.Number` → error (for the int targets).
4. numeric `string` → int/int64/float64; non-numeric `string` → error; `"NaN"`/`"Inf"` string → error for
   the float target.
5. int/uint-kind → float64; `json.Number` → float64; `float64` pass-through (incl. stored `NaN` returned).
6. `bool`/`nil`/slice/map → `*ScopeTypeError` for every coercing getter; missing path → `ErrPathNotFound`.
7. Strict getters keep every existing test green (no behavior change).

## Traceability

Backlog: B3. Plan: 019. ADR: 0044 (records the coercion matrix; aligns with ADR-0035). Related: ADR-0035
(coercion/truthiness philosophy), ADR-0038 (int64 canonical integer kind — why `int64`/`json.Number` are
the lossless shapes), Spec 006 (where the coercing getters were first deferred).
