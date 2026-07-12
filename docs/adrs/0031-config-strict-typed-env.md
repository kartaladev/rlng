# ADR-0031 — Strict typed env from a config schema block

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 011 (docs/specs/011-config-path-safety-parity.md) / Plan 011, deeper-audit finding B2.

## Context

ADR-0023 added `expr.WithEnv(schema)`, which compiles an expression strictly
against a declared environment so a field typo (`scoer >= 650`) is a compile-time
error instead of a silent `nil` misfire. The deeper audit found this guard was
reachable only from the programmatic `expr` API: `config.Build` always compiled
with `AllowUndefinedVariables()` (lenient), so a ruleset authored as YAML/JSON —
the deployment shape the library markets — had no way to opt into it. A file
typo shipped a mis-graded decision.

## Decision

Add a top-level `schema` block to `PipelineDef` (`Schema map[string]any`, field
name → representative value giving its type). When a schema is present, `Build`
threads `expr.WithEnv(schema)` into **every** compiled-expression site — the
single-expr value and condition, each multi-expr entry, and a decision table's
conditions, decisions, and default — via a `withStrictEnv` helper composed with
the existing `withConstants`. The error-attribution re-compile in `buildSingle`
(ADR from Plan 011 Task 2) uses the same strict options so attribution stays
faithful under strict mode.

Strict mode is enabled when a schema is present **or** `WithStrict()` is passed;
`WithSchema(env)` injects/overrides the schema for callers who cannot edit the
document; `WithStrict()` with no resolvable schema is a `*ConfigError`. Absent a
schema, compilation stays lenient — existing behavior is unchanged, so this is
additive and non-breaking. Enablement lives on the new variadic
`Build(opts ...BuildOption)` (see ADR-0033), keeping `Build()` backward compatible.

## Consequences

- The config path reaches parity with the programmatic API on the single most
  important production guard; a field typo in a file-authored ruleset now fails
  at `Build`, naming the offending stage/expression.
- Strictness is opt-in (add a schema), so no existing lenient ruleset breaks.
- The schema is a *representative-value* shape, not a validation schema: it
  type-checks expression identifiers, it does not enforce runtime input shape.
- Declared globals/locals and registered functions are merged into the
  type-check env (via `expr`'s existing `strictEnv`) so they remain usable.
