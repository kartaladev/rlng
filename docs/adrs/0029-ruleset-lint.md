# ADR-0029 — Ruleset lint

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 010 (docs/specs/010-business-rule-hardening.md), audit finding P1-#10.

## Context

A compile (`Build`) catches malformed expressions and structural errors, but not
authoring *smells* that produce wrong-but-valid rulesets: a catch-all rule placed
before later rules (shadowing them), or a first-match table with neither a
catch-all nor a default (a silent no-match gap). These are exactly the mistakes a
BRMS authoring tool would flag; as an engine library we can still offer the static
check without owning a UI.

## Decision

Add `(*PipelineDef).Lint() []Finding` in the `config` package — it lints the
authored declarative artifact, the natural target. Findings are advisory
(`SeverityWarning`), machine-coded, and produced by **static analysis only** — no
expression is evaluated. Initial checks:

- `LintUnreachableRule` — under a first-match policy (single/unique), rules after
  an unconditional `true` catch-all can never fire.
- `LintMissingDefault` — a first-match table with no catch-all and no default
  block leaves unmatched input with no output.

`collect` (accumulates all matches; empty is valid) and `any` (overlap by design)
are excluded from these checks. Catch-all detection is deliberately literal
(`strings.TrimSpace(cond) == "true"`) — cheap, predictable, and false-negative-
safe (it never wrongly flags a real condition); richer tautology detection can be
added later without an API change.

## Consequences

- Consumers get a pre-flight safety net for common authoring bugs, callable in CI
  over their rule files.
- Advisory by design: `Lint` returns findings; enforcing them (fail the build) is
  the consumer's policy choice.
- Extensible: new `Lint*` codes can be added; the `Finding` shape is stable.
