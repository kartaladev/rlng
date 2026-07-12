# ADR-0033 — Lint enforcement at Build

- **Status:** Accepted
- **Date:** 2026-07-12
- **Prompted by:** Spec 011 (docs/specs/011-config-path-safety-parity.md) / Plan 011, deeper-audit finding (config MEDIUM/LOW).
- **Relates to:** ADR-0029 (ruleset lint — the analysis this promotes).

## Context

ADR-0029 added `(*PipelineDef).Lint`, static analysis that reports authoring
smells a compile does not catch — an unreachable rule shadowed by a catch-all, or
a first-match table with no default and no catch-all (a silent no-match gap). The
deeper audit found `Build` never calls `Lint`, so those findings surfaced only if
the consumer independently remembered to. A config with a first-match table and
no default built clean. Separately, lint's catch-all detection matched only the
literal string `true`, so a semantic catch-all such as `1 == 1` produced a false
`missing-default` and hid genuinely unreachable later rules.

## Decision

Add a backward-compatible variadic `Build(opts ...BuildOption)` and a
`WithLintErrors()` option that runs `Lint` during `Build` and promotes findings
to a `*LintError` (an `errors.As`-able type carrying the `[]Finding`). The
`ErrNoStages` check runs first, so an empty definition still returns
`ErrNoStages`, not a lint error. Default `Build()` stays **advisory** — it never
consults lint — so nothing existing breaks; enforcement is opt-in.

Broaden `isCatchAll` to a small documented best-effort set (`true`, `(true)`,
`1 == 1`) and state in godoc that detection is syntactic and best-effort: a
semantic always-true condition it does not recognize may still be flagged, and it
does not claim protection against malformed conditions. This narrows the false
positives without pretending to be an evaluator.

`BuildOption`/`buildConfig` also carry the strict-env toggles (ADR-0031), so the
two config-safety features share one build-option mechanism.

## Consequences

- A ruleset author can make coverage gaps fail construction (`WithLintErrors`)
  instead of shipping a silent no-match table; the default stays advisory for
  compatibility.
- The catch-all heuristic is honest about being best-effort, so its findings can
  be trusted as guidance rather than authority.
- `Build` gains a variadic option surface — the extension point for future
  build-time toggles — without breaking the no-arg call form.
