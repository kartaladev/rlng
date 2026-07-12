# ADR-0017 — Strict config decoding and error attribution

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 008 (docs/specs/008-production-hardening.md), audit findings M2, M8, L5.

## Context

- **M2** — `ParseYAML`/`ParseJSON` decode with default (lenient) semantics, so an
  unknown/misspelled key is silently dropped. A `hitpolicy:` typo leaves
  `HitPolicy == ""` and `Build` defaults to single-hit, changing runtime
  semantics with zero diagnostics — the worst outcome for a debuggability-first
  library.
- **M8** — when a stage constructor fails, `Build` wraps as
  `ConfigError{Stage: name, Cause: err}` even though the cause is already a
  `*stage.StageError` that prefixes the stage name, producing doubled output
  (`config: stage "t": stage "t" (decision-table): …`). Separately, a bad
  `condition:` sub-expression is attributed to the stage, not `field "condition"`.
- **L5** — an empty YAML document builds a zero-stage no-op pipeline with no
  error, while empty JSON errors — inconsistent.

## Decision

- **Reject unknown keys (M2).** `ParseYAML` uses a `yaml.Decoder` with
  `KnownFields(true)`; `ParseJSON` uses a `json.Decoder` with
  `DisallowUnknownFields()`. This catches typos on `StageDef`/`PipelineDef`
  fields (`hit_policy`, `depends_on`, stage type selector). Unknown keys inside
  the custom `ExprDef` object form are not covered by the top-level decoder; this
  is a documented residual (the high-value typo classes live on `StageDef`).
- **Attribute precisely (M8).** `Build` does not re-prefix a cause that is
  already a `*stage.StageError`; it passes it through (optionally as
  `ConfigError{Cause: err}` without a redundant `Stage`). The `condition:`
  sub-expression is validated with `Field: "condition"`.
- **Reject empty pipelines (L5).** `PipelineDef.Build()` returns a `ConfigError`
  when it has zero stages, so a truncated/empty file fails consistently across
  formats.

## Consequences

- Configs with stray/misspelled keys now fail loudly at parse time — a behavior
  change that converts silent misconfiguration into a clear error.
- A previously-accepted empty config is now an error; callers that intentionally
  built an empty pipeline via config must add at least one stage (the programmatic
  `stage.NewPipeline()` empty case is unchanged).
