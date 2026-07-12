# ADR-0023 — Strict typed environment (WithEnv)

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 010 (docs/specs/010-business-rule-hardening.md), audit finding P0-#1.

## Context

Every expression compiled through `expr` uses `exprlang.AllowUndefinedVariables()`
unconditionally. That makes referencing an unknown identifier legal: a field typo
such as `scoer >= 650` compiles and evaluates `scoer` to nil, so the rule
silently misfires with no error. For eligibility/pricing rules this is the single
highest-impact correctness risk in the library — it defeats the debuggability
goal precisely where it matters.

expr offers the opposite mode: compiling with `expr.Env(schema)` and **without**
`AllowUndefinedVariables` type-checks every identifier against the schema and
rejects unknown names at compile time (verified: `scoer` → `unknown name scoer`).

## Decision

Add `expr.WithEnv(env map[string]any) Option`. When set, `buildExprOpts` compiles
with `exprlang.Env(strictEnv(cfg))` and omits `AllowUndefinedVariables`, so
unknown identifiers and type-invalid operations fail at construction. `env` is a
schema: field name → a representative value whose Go type is used for checking.

`strictEnv` overlays the declared env with declared globals then locals so a
`??`-patched default (ADR: variable patcher) still resolves; registered functions
(ADR-0024) are supplied via `exprlang.Function` and type-check independently.

Strict mode is **opt-in**. The default stays lenient (undefined allowed) to avoid
a breaking behavior change and to keep the dynamic-map ergonomics for callers who
want them. The `config` and engine layers thread it through (ADR-0028) so a
declarative ruleset can require strict typing.

## Consequences

- Opt-in production hardening: typos and type mistakes surface at `Build`/compile,
  not as silent nil at runtime.
- The caller supplies a schema; keeping it in sync with the real input is their
  responsibility. A mismatch (declared type ≠ runtime type) is caught by strict
  checking — that is the point.
- Lenient default preserved, so existing callers are unaffected until they opt in.
