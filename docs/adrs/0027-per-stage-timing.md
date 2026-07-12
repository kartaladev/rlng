# ADR-0027 — Per-stage timing

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 010 (docs/specs/010-business-rule-hardening.md), audit finding P2-#13.

## Context

The Scope recorded only the *total* run `Duration`. For observability a consumer
often needs to know which stage in a pipeline is slow — a per-stage breakdown —
without wiring their own timers around the engine.

## Decision

`Pipeline.Run` times each stage's `Execute` using the Scope's injected `Clock`
(ADR: injectable clock) and records the elapsed duration, in first-execution
order, on the Scope. New exported accessors: `Scope.StageDuration(name)` and
`Scope.StageTimings() []StageTiming`. Timing uses the same `Clock` as the total
duration, so a fake clock drives both deterministically in tests.

A stage that errors is still timed (the partial work took time), matching the
existing total-duration semantics where `markFinished` runs via `defer`.

## Consequences

- Per-stage latency is available for metrics/tracing with no extra consumer
  wiring; the total `Duration` is unchanged.
- Timing lives on the Scope (per-run state), consistent with provenance and
  firing rules; it is additive and race-clean under the existing mutex.
- Deterministic under an injected clock, so it is testable without wall-clock
  flakiness.
