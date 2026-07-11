# ADR-0013 — Scope JSON envelope and always-on evaluation timing

- **Status:** Accepted
- **Date:** 2026-07-11
- **Prompted by:** Spec 007 (docs/specs/007-scope-json-timing-and-bare-engine.md)

## Context

A Scope is the carrier of a computation's result. Consumers must move it across
process boundaries (web responses, jsonb columns) and want to know when a
calculation ran and how long it took. The Scope had neither serialization nor
timing.

## Decision

1. **Always-on timing.** Pipeline.Run stamps `startedAt`/`duration` on the Scope
   (start before stages, elapsed after — via `defer`, so it is set even when a
   stage errors). Cost is two clock reads per run — too cheap to gate, so timing
   is not opt-in (unlike provenance). `WithClock` injects a deterministic clock
   for tests. Accessors `StartedAt()`/`Duration()` return `(_, false)` until a
   run stamps the Scope.
2. **JSON envelope, round-trippable.** `Scope.MarshalJSON` emits
   `{data, timing?, derivations?}`; `UnmarshalJSON` restores all three and,
   when derivations are present, marks the Scope provenance-enabled for
   inspection. `data` is always present; `timing`/`derivations` are conditional.
   Raw result data for a web response is `json.Marshal(Snapshot())` (just the
   map) — the envelope is deliberately the persistence form. Numbers are
   restored as json.Number (UseNumber) so integers above 2^53 (e.g. money in
   cents) round-trip exactly; the numeric getters read json.Number losslessly.
   Model money as integer minor units or decimal strings. `Derivation` carries
   snake_case json tags for a stable schema.

## Consequences

- Every run pays two clock reads; negligible against µs-scale evaluation.
- Timing is stamped in exactly one place (Pipeline.Run), so every engine
  (`Engine`, `BareEngine`, direct `Run`) gets it for free.
- The jsonb blob carries the audit trail only when provenance was enabled; the
  off path serializes just data (+ timing).
- The int/float type distinction is not preserved across JSON; a reloaded
  number is a json.Number readable by GetInt (if integral) and GetFloat64.
- A provenance-enabled Scope with zero recorded derivations loses its
  provenance flag on JSON round-trip (the empty derivations map is omitted);
  behaviorally inert since Explain/Lineage/Derivation are empty anyway.
