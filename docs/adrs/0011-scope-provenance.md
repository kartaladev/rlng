# ADR-0011 — Opt-in Scope value provenance and recursive lineage

- **Status:** Accepted
- **Date:** 2026-07-11
- **Prompted by:** Spec 006 (docs/specs/006-scope-provenance-and-getters.md)

## Context

Debuggability is `rlng`'s first-class criterion, but the `Scope` records values
without their derivation. We want, for any value, to answer: which stage
produced it, by what operation, from which expression, reading which inputs —
and to trace that recursively to the seed inputs. This must not tax the
production hot path: most evaluations don't need a trace, and the engine is
"compile once, evaluate fast."

## Decision

1. **Opt-in via `WithProvenance()`.** Provenance is off by default. When off, no
   derivation map is allocated and no recording work runs anywhere — the write
   path is exactly today's `Set`. Benchmarks assert the off path adds zero
   allocations versus baseline. Callers (or the `Engine`, via
   `WithScopeOptions(stage.WithProvenance())`) enable it for debugging/audit.

2. **`Derivation` record per value**, keyed by scope dot-path:
   `{Path, Stage, StageType, Operation, Expression, Inputs, Value}`. Seed inputs
   are recorded in `NewScope` as `Operation: "seed"`. Stages record their writes
   via a new `Derive(path, v, Derivation)` method; `Derive` with provenance off
   is exactly `Set`.

3. **`TracksProvenance()` is lock-free.** The `provenance bool` is set once at
   construction and never mutated, so stages branch on it without the mutex —
   the guard that keeps the off path free of any provenance cost (no `Operation`
   string formatting, no input snapshot).

4. **Referenced-identifier inputs, extracted at compile time.** `Inputs` is the
   set of identifiers the expression actually reads, snapshotted from the eval
   env. `expr.Function/Predicate.References()` computes these **once at compile**
   (parse + `ast.Walk` over `expr-lang`'s AST) and caches them, so recording adds
   no per-eval parsing. Identifiers are **top-level** (`a` for `a.b.c`).

5. **Recursive lineage with prefix reconciliation.** `Lineage(path)` returns the
   value's derivation plus, transitively, its inputs' derivations back to seeds;
   `Explain(path)` renders an ASCII indented tree. Because references are
   top-level but stages write namespaced paths (`tiers.tag`), an input `id`
   links to `Derivation(id)` if present, else to every derivation whose path is
   `id` or starts with `id + "."`. A `visited` set prevents re-walking shared
   inputs (the pipeline is a DAG).

## Consequences

- Zero-cost when off (the common/production case); full causal trace when on.
- The three stages gain a provenance branch in `Execute` and must retain their
  expression source strings (previously discarded) for the `Expression` field.
- Top-level-identifier granularity keeps AST handling simple and robust; the
  prefix reconciliation makes namespaced (decision-table / multi-expr) outputs
  traceable. Precise member-path references (`a.b.c` as a single input) are a
  future refinement, recorded as a non-goal. **Refined by ADR-0047 (B6):**
  `References()` now returns deepest static member paths and reconciliation gains
  a nearest-ancestor fallback.
- `expr` takes internal imports of `expr-lang/expr`'s `parser`/`ast` subpackages
  — no new module dependency.
- If always-on provenance is ever wanted, it is additive (a default flip) and
  would supersede point 1 here.

## Known limitations (deferred)

- **Intra-stage `MultiExpr` references are not traced.** Within a `MultiExpr`,
  a later expression may read an earlier one by its **bare local name** (e.g.
  `b = "a + 1"`), which is only present in the transient stage env, not the
  Scope. Its `Derivation.Inputs` is therefore keyed by that local name (`"a"`),
  while the value is written to the Scope at the namespaced path
  (`"<stage>.a"`). Prefix reconciliation (Decision point 5) links by path, not
  by local alias, so `Lineage`/`Explain` silently omit such intra-stage inputs'
  subtrees. Cross-stage references (which appear as member paths like
  `stage.field` → top-level `stage`) reconcile correctly; only same-stage local
  aliases are affected. A fix means qualifying local aliases to their
  `<stage>.<name>` path when recording `Inputs`, which changes the documented
  "`Inputs` is keyed by referenced identifier" contract — deferred as a
  follow-up so it can be decided deliberately rather than bundled here.
