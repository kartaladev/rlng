# ADR-0028 — Config surface: hit policies, aggregation, default, rule metadata, constants, mapping

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 010 (docs/specs/010-business-rule-hardening.md), audit finding P2-#9.

## Context

The declarative `config` package lagged the `pipe` layer: it could not express the
new hit policies, collect aggregation, default decisions, or rule identity, and a
consumer still had to hand-wire the result mapping and any shared thresholds in
Go. The goal is that a *complete* decision service — input rules → outputs — is
authorable as one YAML/JSON document.

## Decision

Extend the schema:

- **Decision tables:** `hit_policy` gains `unique` and `any`; a new `aggregation`
  field (`list`|`sum`|`min`|`max`|`count`) drives collect reduction; a `default`
  block declares else-decisions; `RuleDef` gains `id` and `message`.
- **`constants`** (pipeline level): a `map[string]any` injected as compile-time
  `WithGlobals` defaults into **every** stage expression (conditions, values,
  decisions, and defaults), so a threshold/label is declared once and referenced
  by name, overridable by runtime input via the variable patcher.
- **`mapping`** (pipeline level): an output-path → expression map. `Build` does
  **not** consume it — result mapping lives above the pipeline in the root `rlng`
  package — but it is parsed and exposed on `PipelineDef.Mapping` (a
  `map[string]string`, directly convertible to `rlng.MappingTemplate`) so the
  whole service is one document. This keeps `config` free of a dependency on the
  root package (no import cycle); the consumer passes `def.Mapping` to
  `rlng.NewMapper`.

### Deferred within config

- **Strict env (`WithEnv`)** and **host functions (`WithFunction`)** are *not*
  declarable in YAML: an env schema needs Go types and functions are Go values.
  They stay programmatic (applied when the consumer builds the engine), which is
  where their inputs naturally live. Recorded here so the omission is deliberate.

## Consequences

- A decision service — constants, staged rules with policies/aggregation/defaults,
  identified rules, and the output mapping — is one declarative artifact.
- Constants reuse the existing variable-patcher default mechanism; no new
  evaluation semantics.
- `config` still does not import the root package; mapping is passed as plain
  data, preserving the sibling-package boundary (ADR-0009/0010 direction).
