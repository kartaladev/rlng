# ADR-0020 — Fail-fast engine constructors

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 009 (docs/specs/009-api-naming-refinement.md)

## Context

Before this change, `New` and `NewTypedEngine` returned only the engine value.
A nil required argument (a nil `*pipe.Pipeline`, or a nil `*Mapper[R]` for the
typed engine) was accepted silently and only surfaced later as a nil-pointer
deref inside the first `Evaluate` — far from the mistake, and as a panic rather
than a typed error. That contradicts the library's debuggability goal, where
construction-time validation that returns a typed error is a first-class part of
the error surface.

## Decision

Both constructors validate their required arguments and return `(T, error)`:

- `New(pipeline, opts...) (*Engine, error)` — returns `errNilPipeline` when
  `pipeline` is nil.
- `NewTypedEngine[I, R](pipeline, mapper, opts...) (*TypedEngine[I, R], error)` —
  returns `errNilPipeline` when `pipeline` is nil, or `errNilMapper` when
  `mapper` is nil.

Options remain optional (variadic). The errors are unexported sentinels
(`errNilPipeline`, `errNilMapper`), matching the existing `errNilInput` /
`errEmptyMappingKey` style, and are `errors.Is`-comparable.

## Consequences

- Breaking signature change (`*Engine` → `(*Engine, error)` and the typed
  equivalent) — acceptable and cheap pre-`v0.0.1`, done together with the
  naming refinement (ADR-0019) so callers absorb one API break, not two.
- Misconstruction fails immediately with a clear, typed error instead of a
  deferred panic; each guard branch is covered by a test.
- Consistent with the config path, where `PipelineDef.Build` and the stage
  constructors already fail fast with typed errors.
