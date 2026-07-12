# Spec 008 — Production-hardening pass

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-12
- **Post-roadmap hardening** (the 5-increment roadmap is complete; this pass fixes defects found in a whole-library production-readiness audit, toward `v0.0.1`).
- **Builds on:** Spec 001 (`expr`), Spec 002 (`stage.Scope`, stages), Spec 003 (`Pipeline`), Spec 004 (`config`), Spec 005 (`Engine`/`Mapper`), Spec 006/007 (provenance, JSON, `BareEngine`).
- **Related ADRs:** ADR-0015 (Scope input isolation + path-segment validation), ADR-0016 (expr recursion bound + fallback semantics), ADR-0017 (strict config decoding + attribution), ADR-0018 (engine input validation + mapper collision). Realized by Plan 008.

## Context

A four-package code + security audit of the merged library surfaced defects that
contradict two of its own guarantees — *"safe for concurrent use after
construction"* and *"never panic/crash on caller input"* — plus a cluster of
silent-failure issues that undercut the library's first-class goal of
**debuggability**. This spec captures the fixes.

## Goals

### Blockers (guarantee violations)

1. **B1 — Scope input isolation.** Seeding a Scope from a `map[string]any` input
   must not alias the caller's nested maps. Today `flatten` passes the map by
   reference and `NewScope` copies only the top level, so a stage write mutates
   caller-owned nested maps and concurrent `Evaluate` calls **data-race**
   (verified at `scope.go:94`). Fix: deep-copy the nested `map[string]any` spine
   when seeding, so the Scope owns every map it may write. This makes the
   documented concurrency guarantee true and stops the caller-mutation side
   effect. See ADR-0015.
2. **B2 — Bounded env reflection.** `expr.toEnv`'s `convertValue`/`structToMap`
   recurse through struct/pointer/map/slice fields with no depth bound; a cyclic
   or very deep struct env crashes the process with an unrecoverable
   `fatal error: stack overflow`. Fix: bound the recursion and return a typed
   `EvalError` when exceeded. See ADR-0016.

### Medium (correctness & debuggability)

3. **M1 — Fallback honors `WithReturnKind`.** A `Function` fallback program is
   compiled without the declared return-kind coercion, so the fallback path can
   return the wrong Go type. Compile the fallback with the same options.
4. **M2 — Strict config decoding.** `ParseYAML`/`ParseJSON` silently drop unknown
   keys, so a `hit_policy` typo changes semantics with no error. Reject unknown
   keys. See ADR-0017.
5. **M3 — Mapper reports colliding output paths.** `setNested` silently overwrites
   a mapped field when output dot-paths collide (`{"a", "a.b"}`); return a
   `*MappingError` instead. See ADR-0018.
6. **M4 — Reject empty path segments.** `Scope.Set` accepts `"a..b"`, `".x"`,
   `"a."` and silently corrupts the namespace; return an error. See ADR-0015.
7. **M5 — Dotted seed keys (declined).** Nesting dotted seed keys was considered
   but **rejected**: it introduces a non-deterministic prefix-collision
   data-loss hazard (`{"a", "a.b"}`). Seed keys are stored verbatim (lossless);
   see ADR-0015.
8. **M6 — Reject nil input.** `Engine.Evaluate(ctx, nil)` (nil pointer) silently
   runs over an empty scope and returns a bogus zero result; return a typed
   error. See ADR-0018.
9. **M7 — Zero-value Scope is usable or safe.** `Set` on a `&Scope{}` panics with
   `assignment to entry in nil map`; lazy-init the map. See ADR-0015.
10. **M8 — Precise config error attribution.** `config.Build` re-prefixes a
    `*stage.StageError` (doubled stage name) and attributes a bad `condition:` to
    the stage rather than the field. See ADR-0017.
11. **M9 — Fallback preserves the triggering error.** When the main expression
    errors and a fallback runs, the original cause is discarded; join it so the
    root reason survives. See ADR-0016.

### Low (robustness / observability / efficiency)

12. **L1** — `Lineage`/`Explain` signal truncation instead of silently dropping
    derivations past the depth bound.
13. **L2** — index derivations by namespace so `derivationsFor` is not an O(N)
    scan per recursion step.
14. **L3** — collect-mode provenance records per-rule `Inputs` faithfully rather
    than last-writer-wins.
15. **L4** — `References()` excludes call-callee identifiers (function names).
16. **L5** — an empty/zero-stage `PipelineDef.Build()` is a `ConfigError`
    (consistent YAML/JSON behavior).
17. **Simplify** — dedup `DecisionTable.executeSingle`/`executeCollect`
    scaffolding; drop the double `ExprDef.options()` allocation in `buildTable`.
18. **Docs** — document the `LoadFile` trust boundary; correct the top-level
    `stage` doc's unqualified "concurrency-safe" wording.

## Non-goals

- Deep-copying nested **slices** on seed (Set never writes into a slice, so the
  writable spine is maps only; slice values stay shared, as already documented).
- Declarative output mapping, `VariablePatcher` config defaults (unchanged
  backlog).
- A base-dir/size-cap sandbox for `LoadFile` (documented as trusted-config;
  hardening deferred unless untrusted use is in scope).

## Verification

Every changed package keeps ≥85% coverage and a covering test for each new
typed-error/hot-path branch (empty-segment, nil-input, depth-exceeded,
unknown-key, collision, fallback-coercion). A `-race` test reproduces B1 (shared
nested-map input across concurrent `Evaluate`) and confirms the fix. Full gate:
`go test ./... -race`, `vet`, `gofmt`, `golangci-lint`, then `/code-review` +
`/security-review` over `main..HEAD`.
