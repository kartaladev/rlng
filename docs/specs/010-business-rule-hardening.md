# Spec 010 — Business-rule production hardening

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-12
- **Post-roadmap hardening**, driven by a business-rule-system audit of the merged
  library toward `v0.0.1`. Complements Spec 008 (defect-focused) with the
  *feature/semantic* gaps a real-world rule engine needs.
- **Builds on:** Spec 001 (`expr`), 002 (`pipe` Scope/stages), 003 (`Pipeline`),
  004 (`config`), 005 (`Engine`/`Mapper`), 006/007 (provenance, JSON, timing),
  008 (production-hardening).
- **Realized by:** Plan 010.
- **Related ADRs:** ADR-0022 (evaluation safety: panic recovery),
  ADR-0023 (strict typed env), ADR-0024 (host functions + `now()`),
  ADR-0025 (decision-table hit policies + default + collect aggregation),
  ADR-0026 (rule identity & explainability), ADR-0027 (per-stage timing),
  ADR-0028 (config surface: mapping + constants), ADR-0029 (ruleset lint),
  ADR-0030 (decimal & foreach deferred).

## Context

The engine is a correct expression evaluator with staged DAG execution, decision
tables, provenance, and typed errors. A business-rule audit found that several
*defaults* silently produce wrong decisions, and that several features required by
real-world rulesets (financial eligibility, pricing, adjudication) are missing.
This spec captures the fixes and additions. It does **not** change the project's
scope boundary: `rlng` stays an embeddable engine, not a BRMS (no authoring UI,
governance, or persistence).

## Goals

### P0 — correctness & safety (silent wrong decisions / host risk)

1. **G1 — Strict typed environment.** Every expression today compiles with
   `AllowUndefinedVariables()`, so a field typo (`scoer >= 650`) evaluates to
   `nil` and the rule silently misfires. Add an opt-in `expr.WithEnv(schema)`
   that compiles against a declared environment *without* undefined-variable
   tolerance, turning typos and type errors into compile-time errors. Declared
   globals/locals and registered functions are merged into the type-check env so
   they remain usable. See ADR-0023.
2. **G2 — Panic recovery.** A panic in the expr VM or a host function must not
   crash the consumer's process. `Function.Apply` and `Predicate.Test` recover
   panics into a typed `EvalError`. See ADR-0022.
3. **G3 — Decision-table semantics.** `executeSingle` writes nothing on no-match
   (a silent zero downstream), and `Collect` only accumulates a slice. Add:
   - a per-table **default** (`else`) decision set applied when no rule matches;
   - hit policies **Unique** (error if >1 rule matches) and **Any** (>1 may match
     but their outputs must agree, else error);
   - **collect aggregation** (`sum`, `min`, `max`, `count`, plus the existing
     list) so "sum of applicable fees" / "max discount" are first-class.
   See ADR-0025.

### P1 — real-world rule features

4. **G4 — Host functions + `now()`.** Add `expr.WithFunction(name, fn)` to register
   host functions, and expose the injected `Clock` as `now()` so temporal rules
   are deterministic and testable. See ADR-0024.
5. **G5 — Rule identity & explainability.** A decision-table `Rule` gains optional
   `ID` and `Message`; the firing rule is recorded so a decision can be explained
   ("Denied by rule CREDIT_MIN_650"). Exposed via a new `Scope` accessor and in
   provenance. See ADR-0026.

### P2 — operational & library

6. **G6 — Per-stage timing.** Record each stage's duration on the Scope (not only
   the total), readable for observability. See ADR-0027.
7. **G7 — Config surface.** Expose the new hit policies, rule metadata, functions,
   strict env, plus config-declared **result mapping** and a pipeline-level
   **constants** block, so a complete decision service is authorable as one
   YAML/JSON document. See ADR-0028.
8. **G8 — Ruleset lint.** A static `Lint` API reports unreachable decision-table
   rules (a catch-all before later rules), tables with no default and no
   catch-all, and other authoring smells — the "manager" safety net. See ADR-0029.
9. **G9 — Release blockers.** `go.mod` declares `go 1.25` (was `go 1.26.4`, which
   broke the stated 1.25+ support); a `ci.yml` runs build/vet/fmt/race-test/lint/
   govulncheck across a Go matrix.

### Deferred (recorded, not implemented this pass) — ADR-0030

- **Decimal money type.** Exact decimal arithmetic inside expressions is a
  cross-cutting design (custom expr type + operator handling) warranting its own
  spec; `float64` semantics are documented as a caveat meanwhile.
  *Realized by Spec 014 (generalized to value serde consistency).*
- **`foreach` stage.** Applying a decision table per element of a collection is a
  new stage type warranting its own spec; the `map/filter/reduce` expr builtins
  cover the scalar-in-one-expression case today. *Realized by Spec 015.*

### Follow-on remediation (deeper post-010 audit)

A second, deeper business-rule production-readiness audit of the merged library
produced specs **011** (config-path safety parity), **012** (evaluation
correctness & explainability), **013** (ruleset identity), **014** (value serde
consistency — subsumes the decimal deferral above), and **015** (`foreach`).

## Non-goals

Authoring UI, persistence, rule versioning/effective-dating workflow, hot-reload
registry — these are BRMS concerns the consumer builds around `rlng`.

## Hot-path branches (test targets)

- `WithEnv`: strict compile rejects unknown ident; accepts declared ident;
  merges globals/locals/functions; lenient path unchanged.
- Panic recovery: panicking function/predicate → `EvalError` (not a crash).
- Hit policies: single (first match), unique (0/1/≥2 matches), any (agree /
  conflict), collect (list) and each aggregation (sum/min/max/count); default
  applied only when no rule matched; empty-agg and type-mismatch error branches.
- `now()`: returns the injected clock's time; deterministic under a fake clock.
- Rule identity: firing rule recorded (single) / not recorded (no match);
  accessor present/absent branches.
- Per-stage timing: duration recorded per stage; empty pipeline.
- Config: each new field parsed + built; mapping/constants; unknown hit policy;
  strict-env round-trip.
- Lint: unreachable rule detected; missing-default detected; clean table → no
  findings.
