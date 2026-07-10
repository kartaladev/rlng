# ADR-0001 — Module path and rule-vs-calc naming

- **Status:** Accepted
- **Date:** 2026-07-10
- **Prompted by:** Spec 001 (docs/specs/001-expression-core.md)

## Context

rlng is a general-purpose engine spanning two tracks — rule evaluation (decision
tables) and calculation pipelines. The git remote is github.com/kartaladev/rlng.
Two naming questions were open: the Go module path, and whether the top-level
API should read as calculation (`Calculator`/`Calculate`, per the seed reference)
or something neutral.

## Decision

- Module path is `github.com/kartaladev/rlng` (ratifies the existing git remote).
- Top-level facade (increment 5) will be named neutrally: `Engine` / `Evaluate`,
  not `Calculator` / `Calculate`, because the library serves calculations *and*
  rules.
- The atomic evaluator layer uses rule/DMN vocabulary: `Predicate.Test` and
  `Function.Apply`.

## Consequences

- Consumers import `github.com/kartaladev/rlng`; a future major version is the
  only way to change the path.
- Naming is consistent across both tracks; the calc-reference names are adapted,
  not copied.
- Supersede this ADR rather than editing it if the naming changes.
