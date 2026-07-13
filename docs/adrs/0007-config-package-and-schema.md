# ADR-0007 — Config package, two-phase parse/Build, list schema, and `ExprDef` shorthand

- **Status:** Accepted
- **Date:** 2026-07-11
- **Prompted by:** Spec 004 (docs/specs/004-declarative-config.md)

## Context

Increment 4 adds the declarative front door: parse a pipeline definition from
YAML/JSON and build the `stage`/`Pipeline` types delivered in Increments 2–3.
Several shape decisions needed recording: where the code lives, how loading is
structured, what the definition schema looks like, and how expression fields are
authored ergonomically. (The dependency choices are consequential enough to
isolate in ADR-0008.)

## Decision

1. **A new `config` package**, importing `stage`, `expr`, and `gopkg.in/yaml.v3`.
   It is kept separate from `stage`/`expr` so those packages retain their
   minimal footprint (YAML is not pulled into a consumer that only builds stages
   programmatically). The root `rlng` package stays empty for the Increment-5
   `Engine` facade, which will wrap `config` + `Pipeline`.

2. **Two-phase API: parse then build.** `ParseYAML`/`ParseJSON`/`LoadFile` return
   a `*PipelineDef` (pure data, no compilation); `(*PipelineDef).Build()` maps it
   to `stage.New*` constructors and `stage.NewPipeline`, returning a
   `*stage.Pipeline`. Separating decode from compile lets callers inspect or
   mutate a definition before building, keeps each phase's errors distinct
   (decode vs compile/validate), and means `Build` reuses all existing
   constructor validation rather than duplicating it.

3. **List-ordered stage schema.** `PipelineDef.Stages` is an ordered **list** of
   a flat `StageDef` union (a `type` discriminator selects which fields apply),
   not a map keyed by stage name. A list preserves authoring order, which `Build`
   feeds to `NewPipeline` — and the pipeline's tie-break ordering among
   independent stages is defined as input order (ADR-0005), so a map (whose
   iteration order is randomized) would make execution order non-deterministic.
   The flat union keeps decoding to a single struct; `Build` reads only the
   fields relevant to each `Type` and validates the required ones.

4. **`ExprDef` scalar shorthand.** Every expression-bearing field decodes from a
   reusable `ExprDef` that accepts **either** a bare scalar string (the string
   *is* the expression — the overwhelmingly common case) **or** a full mapping
   (`expr`, `fallback`, `globals`, `coerce`) via custom `UnmarshalYAML`/
   `UnmarshalJSON`. `ExprDef.options()` maps the object form to the existing
   `expr.Option` values. This one type serves single-expr values/conditions,
   multi-expr entries, and decision-table conditions/decisions, so the ergonomic
   shorthand and the option-passing are defined once.

5. **Delegate validation; add only config-shape checks.** `Build` does not
   re-validate expressions or names — the `stage`/`expr` constructors already do,
   with field-naming errors. `Build` adds only what is config-specific: unknown
   `type`, missing required field for a type, invalid `hit_policy`, and (to avoid
   silently dropping data) **rejecting per-decision options** on a decision-table
   decision, since `stage.Rule` carries only rule-level `DecisionOptions`.
   **Superseded in part by ADR-0046 (B5):** decisions now carry per-decision
   options (`pipe.Decision{Expr, Options}`) and the rejection is removed.

## Consequences

- YAML lives only in `config`; `stage`/`expr` consumers pay nothing for it.
- The two-phase split makes the Increment-5 `Engine` straightforward: accept a
  `*PipelineDef` (or bytes+format), `Build`, then wrap input seeding/result
  mapping around `Pipeline.Run`.
- The flat `StageDef` union permits nonsensical field combinations in the struct
  (e.g. `rules` set on a `single-expr`); `Build` ignores fields irrelevant to
  the `Type` rather than rejecting them, matching how `stage.Option` ignores
  inapplicable options. Required-field presence *is* checked.
- Per-decision decision-table options are unsupported this increment (rejected
  with a `ConfigError`, not silently dropped); a backlog item covers extending
  `stage.Rule` if the need arises.
- `ExprDef` is the single place shorthand/option semantics are defined, so a new
  `expr.Option` (e.g. a future `return_kind`) is one addition here.
