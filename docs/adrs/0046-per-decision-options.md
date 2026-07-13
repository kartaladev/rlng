# ADR-0046 — per-decision options in decision tables

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 021 / Plan 021, graduating backlog item **B5** — the "per-decision options are not
  supported" rejection recorded in ADR-0007 §5 / Spec 004.

## Context

A decision-table `Rule` maps a condition to a set of named decision outputs (`tier`, `limit`, …), each an
expression. In `pipe`, `Rule` carried a **single** `DecisionOptions []expr.Option` applied to **every**
decision, and `WithDefault` likewise took one shared option set for all default decisions. So a decision
could not declare its own `fallback`/`globals`/`coerce` — e.g. "`limit` falls back to `0` on error, but
`tier` has none." The config layer already *parses* each decision as an `ExprDef` (which supports those
options), but `bareDecisions` (`config/build.go`) **rejected** any decision that used them
("per-decision options are not supported; use a bare expression"). The compiled internal form was already
per-decision (`compiledDecision{key, fn}`); only the **input** shape blocked the feature. The user approved
**Option A** ("full options, clean API") at the B5 design checkpoint.

## Decision

**Model a decision as expression + its own options, and thread those options end-to-end — accepting a
breaking change to the exported `pipe.Rule` surface (pre-1.0).**

- **New exported `pipe.Decision` type (Spec 021 D1).** `type Decision struct { Expr string; Options
  []expr.Option }` — one decision output plus the options that apply to it alone. Mirrors the existing
  `Condition`/`ConditionOptions` pairing and the config `ExprDef` shape.
- **`pipe.Rule.Decisions` becomes `map[string]Decision`; `DecisionOptions` is removed (Spec 021 D2,
  BREAKING).** `compileDecisions` drops its shared `opts` parameter and compiles each `Decisions[key].Expr`
  with that decision's own `.Options`. The internal `compiledDecision`/eval path is otherwise unchanged.
- **`WithDefault` takes `map[string]Decision` (Spec 021 D3, BREAKING, symmetric).** Default decisions get
  the same per-decision power as rule decisions; the previous trailing `...expr.Option` shared-option
  parameter is removed.
- **Config threads each `ExprDef`'s options through; the rejection is deleted (Spec 021 D4).**
  `bareDecisions` is replaced by `decisionsFrom(constants, schema, strict, in)` which, per key, builds
  `pipe.Decision{Expr: ed.Expr, Options: withStrictEnv(strict, schema, withConstants(constants,
  ed.options()))}`. The `hasOptions()` check and its `*ConfigError` are gone — a decision's
  `fallback`/`globals`/`coerce` is now honored, in both `rules[i].decisions` and `default`. The
  env/constants/strict wrapping that previously lived in the single `DecisionOptions` is applied per
  decision, so every decision still sees constants + schema, plus its own options.
- **Author (YAML/JSON) surface and `Hash()` are unchanged (Spec 021 D5).** A bare-string decision still
  decodes to `Decision{Expr}` with no options; the object form (`limit: {expr: …, fallback: '0'}`) now
  works instead of erroring. No config schema change and no `Hash()` change — only how `Build` maps the
  (unchanged) parsed `PipelineDef` to `pipe.Rule`, so pre-021 rulesets hash byte-identically (the pinned
  `TestPipelineDefHash` golden test stays green) and replay is unaffected.

## Consequences

- **Feature delivered.** A decision (rule or default) can declare its own `fallback`/`globals`/`coerce`,
  honored end-to-end; a per-decision fallback recovers one output while a sibling without one still errors
  per policy.
- **Breaking exported change (pre-1.0), no deprecation shim.** `pipe.Rule.Decisions` changes type,
  `pipe.Rule.DecisionOptions` is removed, and `pipe.WithDefault`'s signature changes. This is a
  major-affecting API break; because the library is pre-1.0 and the shape is on the debuggability-first hot
  path, a clean break (Option A) was chosen over an additive shim (Option B) or a fallback-only variant
  (Option C). Flag with `apidiff` at release; downstream code constructing `pipe.Rule` directly migrates
  bare decisions to `map[string]Decision{"k": {Expr: "…"}}`.
- **Config strictly more permissive, safer.** `Build` previously *rejected* a decision with options and now
  builds it, honoring the option — a widening, not a regression. The config-schema and parse paths are
  untouched; all existing decision-table config tests stay green.
- **Small surface.** One new exported type, one changed field, one changed option signature, one config
  helper renamed (`bareDecisions` → `decisionsFrom`), one dead method removed (`ExprDef.hasOptions`). No new
  dependency. Every new/changed hot-path branch (per-decision fallback recovers vs sibling errors,
  per-decision globals, default per-decision fallback, config builds-instead-of-rejects, constants reach a
  per-option decision) is covered by table/behavior tests in `pipe` and `config`.

## Traceability

Spec: 021 (docs/specs/021-per-decision-options.md)
Plan: 021 (docs/plans/021-per-decision-options.md)
Backlog: B5 (docs/BACKLOG.md → Resolved)
Supersedes (in part): ADR-0007 §5 (the per-decision-options rejection). Related: ADR-0007 (config
two-phase build & `ExprDef` shorthand), ADR-0037 (why `Hash()` stays stable — the parsed def shape is
unchanged).
