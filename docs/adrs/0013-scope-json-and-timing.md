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
2. **JSON envelope** — see the JSON section added alongside the codec.

## Consequences

- Every run pays two clock reads; negligible against µs-scale evaluation.
- Timing is stamped in exactly one place (Pipeline.Run), so every engine
  (`Engine`, `BareEngine`, direct `Run`) gets it for free.
