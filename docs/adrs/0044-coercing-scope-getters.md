# ADR-0044 — numeric-coercing Scope getters

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 019 / Plan 019, graduating backlog item **B3** — the "coercing Scope getters"
  deferral (Spec 006 non-goal; `pipe/get.go`).

## Context

The typed `Scope` getters (`GetInt`/`GetInt64`/`GetFloat64`) are strict: they accept only the exact target
type plus the lossless `json.Number`/`int64` shapes a JSON round-trip produces (ADR-0038). A `float64` at an
int path, an `int` at a float path, or a numeric **string** is a `*ScopeTypeError`. Strictness is the right
default — honest and debuggable — but callers routinely read values that arrived through a loosely-typed
edge (untyped JSON, a form/CSV field, an `expr` result that widened an int to `float64`) and end up
hand-writing the same type-switch at each read. Spec 006 recorded a coercing variant as an additive future
addition.

## Decision

Add three **opt-in coercing numeric getters** — `GetIntCoerce`, `GetInt64Coerce`, `GetFloat64Coerce` — each
mirroring its strict sibling's signature. They accept a wider set of stored types and convert **safely and
honestly**, aligned with the project's coercion philosophy (ADR-0035): never silently corrupt, never
manufacture a `NaN`/`±Inf`, fail loud with a typed error.

- **Separate methods, not a getter option** (Spec 019 D1). The strict getters are simple accessors; a
  parallel `…Coerce` method with its own godoc-documented matrix is more discoverable than a variadic option
  and leaves every strict signature untouched. The `Coerce` suffix reuses the codebase's existing
  `WithCoerce` vocabulary (ADR-0035). Scope is the three numeric getters only — the backlog's named gap;
  `GetString`/`GetBool`/`GetSlice`/`GetMap` coercion is a non-goal (bool coercion is the predicate `truthy`
  path; string/slice/map coercion is lossy and unrequested).
- **Integer-target matrix** (`GetIntCoerce`/`GetInt64Coerce`, via the shared `coerceToInt64`): any
  signed/unsigned integer kind **overflow-checked** (a `uint64 > MaxInt64`, or an `int64` outside the
  32-bit `int` range, is an error, never a silent wrap — golang-safety); a `float32`/`float64` only when
  **finite and integral** (a fractional or non-finite float is an error — no silent truncation); an integer
  `json.Number` via `.Int64()`; a base-10 numeric `string` via `strconv.ParseInt`. `GetIntCoerce`
  additionally range-checks the `int64` result to `int` (the same guard the strict `GetInt` uses).
- **Float-target matrix** (`GetFloat64Coerce`, via `coerceToFloat64`): a float passes through as-is (a
  caller-stored non-finite value is preserved — coercion widens *types*, it does not re-validate a stored
  value); any integer kind widens (precision may be lost above 2^53, inherent to `float64`, documented); a
  `json.Number` via `.Float64()`; a `string` via `strconv.ParseFloat`, **rejecting a non-finite parse
  result** (`"NaN"`/`"Inf"`) — coercion never manufactures a non-finite float from text (ADR-0035).
- **Reuse `*ScopeTypeError` / `ErrPathNotFound`** (Spec 019 D2) — no new error type. The conversion helpers
  return a plain reason error that the getter wraps into a `*ScopeTypeError` naming the path and target, so
  a failure is `errors.As`-inspectable exactly like the strict getters'.

*Alternatives considered:* a `WithCoerce()` getter option (rejected — heavier for a plain accessor, touches
strict signatures); a generic `GetAsCoerce[T]` (rejected — the three concrete methods are the whole
surface). Recorded in Spec 019 D1 / Non-goals.

## Consequences

- **Purely additive; strict API unchanged.** No strict getter, `Scope`, or existing error contract changed
  — no SemVer break. Existing getter tests stay green.
- **Debuggability preserved.** Every unconvertible value is a typed `*ScopeTypeError` whose `Actual` names
  the concrete reason (`float64(3.14) is not integral`, `uint64(…) overflows int64`, `string("abc") is not
  an integer`), consistent with the strict getters and the project's debuggability mandate.
- **Consistent with the rest of the engine's coercion.** The safe/honest rules match `WithCoerce`
  predicates (ADR-0035); a caller who wants strictness keeps using the strict getters.
- **Small surface, small maintenance.** Two shared unexported helpers (`coerceToInt64`/`coerceToFloat64`)
  back all three methods; every coercion and error branch is covered (the only untested line is the
  int64→int guard, unreachable on 64-bit, matching strict `GetInt`).

## Traceability

Spec: 019 (docs/specs/019-coercing-scope-getters.md)
Plan: 019 (docs/plans/019-coercing-scope-getters.md)
Backlog: B3 (docs/BACKLOG.md → Resolved)
Related: ADR-0035 (coercion/truthiness philosophy), ADR-0038 (int64 canonical integer kind).
