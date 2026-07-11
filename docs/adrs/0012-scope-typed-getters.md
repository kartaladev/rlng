# ADR-0012 — Strict typed Scope getters via a generic helper

- **Status:** Accepted
- **Date:** 2026-07-11
- **Prompted by:** Spec 006 (docs/specs/006-scope-provenance-and-getters.md)

## Context

`Scope` stores `map[string]any`; callers extracting a concrete value hand-roll a
`Get` + type assertion + not-found/type checks each time. We want convenience
getters (`GetInt`, `GetString`, …) with typed, debuggable errors. Two decisions:
whether to coerce numeric kinds, and how to implement the family without
duplicating the assertion/error logic per type.

## Decision

1. **Strict type match, no coercion.** `GetInt` succeeds only if the stored
   value is exactly an `int`; a `float64` yields a `*ScopeTypeError`, not a
   converted value. Predictability wins: coercion rules (which numeric kinds
   convert, overflow/truncation handling) are subtle and easy to get subtly
   wrong, and a strict getter's failure names the exact expected/actual types —
   more debuggable than a silent conversion. A coercing variant can be added
   later without breaking the strict API.

2. **One generic implementation, named wrappers.** A single generic
   `GetAs[T any](s *Scope, path string) (T, error)` does the work — `Get`, then a
   generic type assertion `v.(T)` (no reflection). The named methods
   (`GetInt/GetInt64/GetFloat64/GetString/GetBool/GetSlice/GetMap`) delegate to
   it. `GetAs` is exported so callers can extract types without a named method
   (a custom struct, a `[]byte`, …). This keeps the assertion/error logic in one
   place and the named set is pure ergonomics.

3. **Typed errors.** A missing path returns the sentinel `ErrPathNotFound`; a
   present-but-wrong-type value returns `*ScopeTypeError{Path, Expected, Actual}`
   (`Expected`/`Actual` are `%T` of the zero `T` and of the stored value),
   matching the project's field-naming error discipline.

## Consequences

- Callers get ergonomic, type-safe access with clear failures; no numeric
  surprises. Since `expr` freely mixes `int` and `float64`, callers must request
  the type a value actually has (e.g. `GetFloat64` for `price * qty`); the typed
  error tells them which when they guess wrong.
- The generic method-via-`GetAs` pattern (methods can't have their own type
  parameters, but can call a generic package function) avoids N copies of the
  same logic.
- A future `GetIntCoerce`/`WithCoercion` can layer on without touching the strict
  getters — a superseding/extending ADR if added.
