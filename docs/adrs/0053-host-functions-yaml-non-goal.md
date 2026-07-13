# ADR-0053 — Host functions in YAML: a deliberate non-goal (B12 closure)

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Backlog B12 (`docs/BACKLOG.md`); Spec 028 (docs/specs/028-host-functions-yaml-feasibility.md)
- **Supersedes:** the "Deferred within config" bullet of ADR-0028 (that ADR otherwise stands)
- **References:** ADR-0031 (delivered the env-schema half), ADR-0024 (host functions, programmatic),
  ADR-0023 (strict typed env, programmatic)

## Context

ADR-0028 §"Deferred within config" deferred declaring both **strict env (`WithEnv`)** and **host functions
(`WithFunction`)** in a YAML/JSON ruleset, on the grounds that "an env schema needs Go types and functions
are Go values." Backlog item **B12** graduates that deferral for a decision. Two things have since changed
the picture, established in Spec 028 from the current code:

1. **The env-schema half was already delivered.** ADR-0031 (increment 011) added the top-level `schema` block
   to `PipelineDef` (`config/def.go`), threading `expr.WithEnv(schema)` into every compiled-expression site.
   A YAML-authored ruleset already opts into strict typing via representative-value shapes
   (`schema: {score: 0, tier: ""}`) — so that portion of ADR-0028's deferral is obsolete, though ADR-0028's
   note was never updated to say so.
2. **Host functions in YAML remain the only open question**, and it resolves against implementation.

## Decision

**Declaring host functions in YAML is a deliberate, likely-permanent non-goal.** Host functions stay
programmatic (`expr.WithFunction`, applied when the consumer builds the engine — where their inputs naturally
live). The three ways one might express functions in config were each considered and rejected (Spec 028):

1. **Arbitrary Go functions serialized in YAML — infeasible and rejected on principle.** YAML is data; a Go
   function is compiled code. Bridging the gap requires a plugin loader or embedded interpreter, which breaks
   the two constraints that define this library: *pure-Go debuggability* (the raison d'être vs. cgo-bound
   zen-go — a breakpoint must land in plain Go) and a *minimal, safe surface* (running code named by a config
   file is an RCE-shaped capability). This is the crux of ADR-0028's original "functions are Go values."
2. **Expression-bodied functions** (`functions: {discount: {params: [x], expr: "x * 0.9"}}`, a compiled
   sub-expression registered as a callable) — **feasible but YAGNI.** It duplicates `multi-expr` stages, the
   variable patcher (ADR-0028 constants), and `depends_on` composition, adding a mini-DSL (param binding,
   ordering/recursion, strict-env interaction) for no concrete demand.
3. **Named allowlist selection** (YAML names functions from a host-registered Go registry) — **feasible but
   marginal.** Go still supplies every function; YAML only selects, barely improving on calling
   `WithFunction` directly.

This closes B12 with **no runtime code change**. ADR-0028's deferred bullet is annotated to point here (env →
ADR-0031, functions → this ADR).

## Consequences

- The B1–B12 backlog-execution program is complete: B12 moves to `docs/BACKLOG.md` → Resolved.
- The public API is unchanged; no `Hash()`, config-schema, or SemVer impact. Strict typed env stays available
  in YAML (ADR-0031); host functions stay a programmatic `expr.WithFunction` concern.
- If a concrete need for config-declared function logic ever appears, variant 2 (expression-bodied functions)
  is the sanctioned re-entry point — it would supersede this ADR and require its own spec/plan/ADR chain and a
  demonstrated use case, not a speculative build.
- A future reader who revisits "why can't I define a function in the YAML?" finds the reasoning here rather
  than re-deriving it.
