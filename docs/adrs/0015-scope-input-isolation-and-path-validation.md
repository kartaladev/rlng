# ADR-0015 — Scope input isolation and path-segment validation

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 008 (docs/specs/008-production-hardening.md), audit findings B1, M4, M5, M7.

## Context

`stage.Scope` is documented as "concurrency-safe," and `Engine`/`BareEngine` as
"safe for concurrent use after construction." The audit disproved this for
`map[string]any` inputs: `flatten` returns the caller's map by reference and
`NewScope` shallow-copies only the top level, so nested `map[string]any` values
stay aliased to caller state. A stage write (`Set("cfg.rate", …)`) then mutates
the caller's map, and concurrent `Evaluate` calls sharing one input data-race
(verified at `scope.go:94`). Three smaller `Scope` defects share the same
dot-path machinery: `Set` accepts empty path segments (`"a..b"`) and silently
corrupts the namespace (M4); dotted **seed** keys are stored as dead literal
top-level keys instead of being nested to match expr addressing (M5); and `Set`
on a zero-value `&Scope{}` panics on a nil map (M7).

## Decision

- **Own every writable map (B1).** `NewScope` deep-copies the nested
  `map[string]any` spine of the seed (recursively through maps). Slices, structs,
  and scalars stay shared — `Set` only ever traverses/creates `map[string]any`
  nodes, so maps are the entire writable surface; copying them makes the Scope
  the sole owner of anything it can mutate. Cost is paid once per seed (not on the
  per-stage hot path). The copy is depth-bounded (`maxCloneDepth`): a
  pathologically deep seed map (e.g. decoded from deeply-nested untrusted JSON)
  shares rather than copies beyond the bound, so the copy itself can never
  overflow the stack — isolation lapses only at a depth no dot-path stage write
  could reach.
- **Store seed keys verbatim (M5 declined).** `NewScope` keeps each seed key as a
  literal top-level key (deep-copying its value). Nesting dotted seed keys — the
  originally-proposed M5 fix — was **rejected**: because Go randomizes map
  iteration order, a seed that mixes a scalar key with a dotted key sharing its
  prefix (`{"a": 1, "a.b": 2}`) would non-deterministically lose a value when the
  two collide during nesting. Literal storage is lossless and deterministic; a
  dotted seed key is simply not addressable via a nested `Get` (matching the expr
  environment, and mattering only for callers seeding raw dotted keys, since
  struct flattening produces dot-free keys).
- **Validate path segments (M4).** `Set` returns `ErrEmptyPathSegment` when any
  dot-separated segment is empty.
- **Lazy-init the map (M7).** `Set` allocates `s.data` if nil, so a zero-value
  `Scope` is usable (matching `UnmarshalJSON`, which already lazy-inits).

## Consequences

- The concurrency guarantee becomes true for map inputs; the caller's input map
  is no longer mutated. Existing callers that (incorrectly) relied on seeing
  stage writes in their input map will not — this is the intended fix.
- `ErrEmptyPathSegment` is a new exported sentinel; empty-segment paths that used
  to "succeed" (corrupting state) now return an error. Behavior change, but the
  prior behavior was a silent bug.
- `NewScope` keeps its `*Scope` (no error) signature and its verbatim-key
  semantics, so the change is non-breaking at the type and behavior level except
  for the added deep copy.
