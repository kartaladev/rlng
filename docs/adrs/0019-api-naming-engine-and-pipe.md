# ADR-0019 — Engine naming and the `pipe` package

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 009 (docs/specs/009-api-naming-refinement.md)
- **Supersedes:** the engine naming in ADR-0009 (`Engine[I, R]` facade) and
  ADR-0014 (`BareEngine`); revises the rule-vs-calc naming in ADR-0001 (the
  module path in ADR-0001 stands).

## Context

Pre-`v0.0.1`, the exported surface is still free to change. Three names read
poorly (see Spec 009): `BareEngine` is awkward for what is the more fundamental
engine; the generic `Engine[I, R]` grabbed the unqualified name though it is a
convenience layer; and `stage.Stage` stutters.

## Decision

- **Promote the map engine.** `BareEngine` → `Engine`; `NewBareEngine` → `New`.
  `rlng.New(pipeline, opts...)` returns the primary `*Engine` (map-in / map-out).
- **Qualify the typed engine.** `Engine[I, R]` → `TypedEngine[I, R]`;
  `New[I, R]` → `NewTypedEngine[I, R]`. The shared `Option`, `WithScopeOptions`,
  `engineConfig`, and `flatten` live with the base `Engine` in `engine.go`;
  `TypedEngine` lives in `typed_engine.go`.
- **Rename the package `stage` → `pipe`.** Directory `stage/` → `pipe/`,
  `package stage` → `package pipe`; exported identifiers are unchanged, so
  `pipe.Stage`, `pipe.Scope`, `pipe.Pipeline`, `pipe.SingleExpr`, etc. `pipe`
  beat `pipeline` (which stutters on `pipeline.Pipeline`) and `graph` (which
  over-emphasizes the DAG and reads oddly with `Scope`/expr types).

## Consequences

- Breaking rename of the exported API — acceptable and cheap now (no release
  tag). After `v0.0.1` this would require a major bump; it is done deliberately
  before the first tag.
- The canonical quick-start is `rlng.New(pipeline).Evaluate(ctx, input)` →
  `map[string]any`; typed callers opt into `rlng.NewTypedEngine[I, R]`.
- ADR-0009 and ADR-0014 are superseded for naming; their design rationale
  (typed facade + mapper; mapper-less engine) is unchanged and still applies
  under the new names.
- No behavior change: the rename is mechanical, verified by the unchanged
  (green, `-race`) test suite.
