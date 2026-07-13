# Spec 021 — per-decision options in decision tables

- **Status:** Draft
- **Backlog item:** B5 (`docs/BACKLOG.md`) — graduates the "per-decision options are not supported"
  rejection recorded in ADR-0007 §5 / Spec 004.
- **Design approval:** Option A ("full options, clean API") approved by the user at the B5 design checkpoint
  (2026-07-13).
- **Realized by:** Plan 021; ADR-0046.

## Problem

A decision-table `Rule` maps a condition to a set of named decision outputs (`tier`, `limit`, …), each an
expression. In `pipe` a `Rule` carries a **single** `DecisionOptions []expr.Option` applied to **every**
decision, and `WithDefault` likewise takes one shared option set for all default decisions. So a decision
cannot declare its own `fallback` / `globals` / `coerce` — e.g. "`limit` falls back to `0` on error, but
`tier` has no fallback." The config layer already *parses* each decision as an `ExprDef` (which supports
those options), but `bareDecisions` (`config/build.go`) **rejects** any decision that uses them
("per-decision options are not supported; use a bare expression"). The compiled form is already
per-decision (`compiledDecision{key, fn}`), so only the **input** shape blocks the feature.

## Goal

Let each decision (and each default decision) carry its own `fallback`/`globals`/`coerce`, honored
end-to-end, by making `pipe.Rule` model decisions as expression **plus options** instead of a bare string
plus one shared option set. Delete the config rejection so the already-parsed per-decision `ExprDef`
options flow straight through. This is **Option A** (approved): the clean-API shape, accepting a breaking
change to the exported `pipe.Rule` surface (pre-1.0, ADR-0046).

## Decisions

- **D1 — New exported `pipe.Decision` type.** `type Decision struct { Expr string; Options []expr.Option }`
  — one decision output: its expression and its own compile options. Mirrors the existing
  `Condition`/`ConditionOptions` pairing and the config `ExprDef` shape.
- **D2 — `pipe.Rule.Decisions` becomes `map[string]Decision`; `DecisionOptions` is removed (BREAKING).**
  ```go
  type Rule struct {
      ID, Message, Condition string
      ConditionOptions []expr.Option
      Decisions        map[string]Decision   // was map[string]string
      // DecisionOptions []expr.Option        // REMOVED — options now live per-decision
  }
  ```
  `compileDecisions` compiles each `Decisions[key].Expr` with that decision's own `.Options` (its shared
  `opts` parameter is dropped). The internal `compiledDecision`/eval path is otherwise unchanged (it was
  already per-decision).
- **D3 — `WithDefault` takes per-decision decisions too (BREAKING, symmetric).**
  `WithDefault(decisions map[string]Decision) Option` (was `(map[string]string, ...expr.Option)`). Default
  decisions get the same per-decision power as rule decisions; the option is symmetric with D2.
- **D4 — Config threads each `ExprDef`'s options through; the rejection is deleted.** `bareDecisions` is
  replaced by a converter `decisionsFrom(...) map[string]pipe.Decision` that, for each key, builds
  `pipe.Decision{Expr: ed.Expr, Options: withStrictEnv(strict, schema, withConstants(constants,
  ed.options()))}`. The `hasOptions()` check and its `*ConfigError` disappear — a decision's
  `fallback`/`globals`/`coerce` is now honored instead of rejected, in both `rules[i].decisions` and
  `default`. The env/constants/strict wrapping that previously lived in the single `DecisionOptions` is
  applied per decision, so every decision still sees constants + schema, plus its own options.
- **D5 — Author (YAML/JSON) surface is unchanged.** A bare-string decision (`tier: '"prime"'`) still
  decodes to `Decision{Expr: …}` with no options; the object form (`limit: {expr: …, fallback: '0'}`) now
  works instead of erroring. No config schema change, no `Hash()` change (the parsed `PipelineDef` shape is
  untouched — only how `Build` maps it to `pipe.Rule`), so pre-021 rulesets hash identically and replay is
  unaffected.
- **D6 — Migrate all in-repo callers/tests.** Every `pipe.Rule{Decisions: map[string]string{…}}` and
  `WithDefault(map[string]string{…}, …)` in the pipe/config test suites is migrated to the new shapes.
  Bare decisions become `map[string]Decision{"k": {Expr: "…"}}`.

## Non-goals

- Per-decision options anywhere other than decision tables (single-expr/multi-expr already have their own
  per-expression options).
- Deprecation shims for the old `pipe.Rule` shape — pre-1.0, a clean break with an ADR + `apidiff` note is
  the recorded choice (Option A over the additive Option B / fallback-only Option C).
- New option *kinds* — `fallback`/`globals`/`coerce` are the existing `ExprDef`/`expr.Option` set; B5 only
  makes them per-decision.

## Success criteria / hot-path branches to cover

1. A rule with a per-decision `fallback` on one output (not another): the output with the fallback recovers
   on a value error while a sibling output without one still errors/behaves per policy.
2. Per-decision `globals` and `coerce` on a decision are honored (previously rejected).
3. A `default` decision with its own `fallback`/`globals` is honored (D3 symmetry).
4. Bare-string decisions (no options) behave exactly as before (regression).
5. Config: a decision using options **builds** (no `*ConfigError`) — the deleted rejection is gone; existing
   decision-table config tests stay green; `Hash()` of an unchanged ruleset is byte-identical (golden test).
6. Constants + schema/strict env still reach every decision (per-decision options compose with the shared
   env, not replace it).

## Traceability

Backlog: B5. Plan: 021. ADR: 0046 (records Option A + the breaking `pipe.Rule`/`WithDefault` change;
supersedes ADR-0007 §5's per-decision-options rejection). Related: ADR-0007 (config two-phase build &
`ExprDef` shorthand), ADR-0037 (why `Hash()` stays stable — the parsed def shape is unchanged).
