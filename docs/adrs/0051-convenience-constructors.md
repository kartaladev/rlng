# ADR-0051 — convenience constructors: build an engine from a config source

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 026 / Plan 026, graduating backlog B10 (constructors half).

## Context

Building an engine from declarative config was three explicit steps (config.Parse -> PipelineDef.Build ->
rlng.New). ADR-0009 kept `rlng` free of a `config` dependency (the declarative and programmatic paths
converge at *pipe.Pipeline) but explicitly anticipated a `rlng.NewFromYAML` convenience "added additively if
desired (deferred, YAGNI)." B10 is that follow-through. The B10 backlog line also named a second feature —
`Pipeline` implementing `Stage` (nested pipelines) — which ADR-0005 deliberately excluded.

## Decision

- **Four convenience constructors in the root `rlng` package** (`fromconfig.go`): `NewFromProvider`,
  `NewFromYAML`, `NewTypedFromProvider[I,R]`, `NewTypedFromYAML[I,R]`. Each composes
  Parse -> Build -> New/NewTypedEngine, is ctx-first, calls `Build()` with default options, threads engine
  `Option`s, and returns the first error unwrapped. `NewFromProvider` accepts any `config.Provider`, so no
  per-format constructor family is needed.
- **`rlng` now imports `config`** — the additive convenience ADR-0009 anticipated. `config` is same-module,
  its transitive deps are already in `go.mod` (no new external dependency; `go mod tidy` stays a no-op), the
  import is acyclic (`config` does not import `rlng`), and dead-code elimination drops `config` from a
  consumer's binary when the constructors go unused.
- **Build-time options stay on the explicit path.** The constructors use default `Build()`; strict schema /
  lint-as-error / version override use `config.Parse -> Build -> New` directly. No new error type — parse,
  build, and nil-mapper errors are threaded through unchanged.

## Consequences

- The common "engine from YAML/JSON" path is a single call; the three-layer path remains for advanced build
  options. Purely additive public surface (no breaking change; no Hash()/eval/schema change).
- `rlng` gains a `config` package import (in-module, acyclic, no new external dep).
- **Pipeline-as-Stage (the other half of B10) is re-deferred, deliberately.** Reversing ADR-0005 has
  marginal value: `foreach` already owns per-element sub-pipelines, and a flat nested pipeline is ≈ inlining
  its stages (shared scope + DAG). It would add naming, shared-scope bookkeeping, and collision semantics
  for no concrete demand. Kept as a documented backlog note; revisit with a superseding ADR to ADR-0005 if a
  real composition need appears.

## Traceability

Spec: 026. Plan: 026. Extends: ADR-0009 (the anticipated additive `NewFromYAML`). Related: ADR-0019
(TypedEngine naming), ADR-0005 (Pipeline-not-a-Stage, the re-deferred non-goal), increment 016
(config.Provider abstraction consumed here).
