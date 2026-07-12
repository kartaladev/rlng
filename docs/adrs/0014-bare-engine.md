# ADR-0014 — Mapper-less BareEngine

- **Status:** Accepted; naming superseded by ADR-0019 (BareEngine -> Engine)
- **Date:** 2026-07-11
- **Prompted by:** Spec 007 (docs/specs/007-scope-json-timing-and-bare-engine.md)

## Context

Engine[I, R] projects the final Scope into a typed R via a Mapper. Some
consumers want the raw accumulated values and no mapping — e.g. to serialize
the Scope directly, or to work dynamically with map[string]any.

## Decision

Add BareEngine: constructed from a compiled Pipeline, `Evaluate(ctx, input any)
(map[string]any, error)` returns the Scope snapshot; `EvaluateScope` returns the
full *stage.Scope (timing, JSON, provenance). It reuses the existing `flatten`
(map passthrough / struct via mapstructure) and `WithScopeOptions`. `input` is
`any` — a BareEngine is not parameterized on input or result type.

## Consequences

- Two engines share the seeding/scope machinery; only the tail differs (map vs
  Mapper.Map). No new dependency.
- `Evaluate` returning `map[string]any` is the documented shape; `EvaluateScope`
  is the escape hatch for the richer Scope capabilities.
