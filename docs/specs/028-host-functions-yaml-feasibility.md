# Spec 028 — Host functions in YAML: feasibility & closure of B12

- **Status:** Accepted (closure spec — no implementation)
- **Date:** 2026-07-13
- **Backlog item:** B12 (`docs/BACKLOG.md`) — "Strict env / host functions declarable in YAML"
- **Governing ADRs:** ADR-0028 (config surface; recorded the deferral), ADR-0031 (delivered the env-schema
  half), ADR-0024 (host functions, programmatic), ADR-0023 (strict typed env, programmatic).
- **Resulting ADR:** ADR-0053 (host functions in YAML — deliberate non-goal).

> **Deliberate deviation from the usual spec → plan → ADR → code chain:** this increment produces **no
> runtime code**. Its deliverable is a decision (ADR-0053) closing a backlog non-goal. There is therefore no
> `docs/plans/028` — this spec links directly to ADR-0053. Recorded explicitly so the traceability gap is
> intentional, not an omission.

## Problem

B12 asks whether the two programmatic hardening features — a **strict typed env** (`expr.WithEnv`) and
**host functions** (`expr.WithFunction`) — can be declared in a YAML/JSON ruleset instead of being supplied
in Go at engine-build time. ADR-0028 §"Deferred within config" recorded both as deferred ("an env schema
needs Go types and functions are Go values"), flagged in the backlog as a *likely permanent non-goal*. This
spec settles the feasibility question with the code as it stands today, rather than from the original
deferral note.

## Finding 1 — the env-schema half is already delivered

ADR-0031 (increment 011) added a top-level `schema` block to `PipelineDef`
(`config/def.go`: `Schema map[string]any` with `yaml:"schema" json:"schema"`) and threads
`expr.WithEnv(schema)` into **every** compiled-expression site when a schema is present. A field typo
(`scoer >= 650`) in a file-authored ruleset now fails at `Build`, naming the offending stage. So "declare a
strict typed env in YAML" — the first half of B12 — **already works**; the `schema` block expresses each
field's type via a representative value (`{"score": 0, "tier": ""}`), which YAML/JSON carry natively. The
BACKLOG/ADR-0028 deferral note is stale on this point and is corrected by ADR-0053.

## Finding 2 — host functions in YAML, across three variants

A host function is `func(...any) (any, error)` — arbitrary Go. Three ways one might "declare functions in
YAML" were considered:

1. **Arbitrary Go functions serialized in YAML.** **Infeasible and undesirable.** YAML is data; Go functions
   are compiled code. Bridging that needs a plugin loader or an embedded interpreter — which would break the
   project's two load-bearing constraints: *pure-Go debuggability* (the entire reason `rlng` exists rather
   than the cgo-bound zen-go — a breakpoint must land in plain Go, not a sandbox) and a *minimal, safe
   surface* (executing code named by a config file is an RCE-shaped capability). Rejected on principle.

2. **Expression-bodied functions** — a named, parameterized `expr` sub-expression in YAML, e.g.
   `functions: {discount: {params: [x], expr: "x * 0.9"}}`, compiled at `Build` and registered as a callable
   via `expr.Function`. **Technically feasible** (wrap a compiled `*vm.Program`). **Rejected as YAGNI:** it
   is a new mini-DSL that largely duplicates existing mechanisms — `multi-expr` stages (named intra-stage
   sub-expressions), the variable patcher (named constants/thresholds, ADR-0028), and `depends_on`
   composition — for the cost of parameter binding, declaration ordering/recursion rules, and strict-env
   interaction. No concrete demand justifies the added surface.

3. **Named allowlist selection** — YAML references functions by name from a registry the host pre-registers
   in Go and passes to `Build`. **Feasible and safe, but marginal:** the Go program still supplies every
   function; YAML only *selects* from what Go already provides, which is barely more than calling
   `WithFunction` directly. No meaningful capability gained.

## Decision

**Close B12 as a documented non-goal (ADR-0053).** Record that the env-schema half was delivered by ADR-0031,
and that host functions in YAML stay programmatic — variant 1 infeasible/unsafe, variants 2 and 3 feasible
but not worth their surface — so a future reader does not re-litigate it. No runtime code changes. This
resolves the last open backlog item and completes the B1–B12 program.

## Traceability

Realized by **ADR-0053** (supersedes the "Deferred within config" bullet of ADR-0028; references ADR-0031,
ADR-0024, ADR-0023). Closes **B12** in `docs/BACKLOG.md` (→ Resolved). No plan (docs-only closure, see the
deviation note above). No `Hash()`/config-schema/API change.
