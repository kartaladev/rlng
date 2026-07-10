# ADR-0003 — Decision-table hit policies

- **Status:** Accepted
- **Date:** 2026-07-10
- **Prompted by:** Spec 002 (docs/specs/002-scope-and-stages.md)

## Context

The DecisionTable stage evaluates ordered rules, each a condition plus a set of
output-key -> value-expression decisions. Two semantic questions needed
recording: how multiple matching rules resolve, and whether decisions within a
single rule may depend on one another.

## Decision

- **Hit policy** is selected by `WithMode`, defaulting to `ModeSingle`:
  - `ModeSingle` — first-match-wins: the first rule whose condition tests true
    has its decisions applied under `name.<outputKey>`, and evaluation stops. No
    match writes nothing.
  - `ModeCollect` — every matching rule contributes; each output key accumulates
    a `[]any` with one entry per matched rule, in rule order (DMN COLLECT
    semantics). No match writes nothing.
- **Decisions within a rule are independent**: all are evaluated against the same
  pre-rule Scope snapshot, so decision order is not significant. This is why
  `Rule.Decisions` is a plain `map[string]string`. Decisions are compiled and
  evaluated in sorted-key order purely for deterministic output.

## Consequences

- Collect output is always a list per key, even for a single match, so consumers
  read a stable shape.
- A decision cannot read another decision's output within the same rule; chain
  such dependencies across stages (or a MultiExpr) instead. Revisit with a
  superseding ADR if intra-rule decision dependencies become necessary.
