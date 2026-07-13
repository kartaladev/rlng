# rlng — consolidated backlog (tracked)

> Living document. Last swept: 2026-07-13, across all increments through **016** (config source `Provider`).
> Each item cites the ADR/spec/code that records it — **trust those source artifacts over this summary**;
> this file only aggregates and prioritizes. When an item is picked up, it graduates to a `docs/specs/*` →
> `docs/plans/*` → `docs/adrs/*` chain per CLAUDE.md, and moves to the "Resolved" section here (with the
> closing increment/ADR) rather than being deleted.
>
> Nothing below is a bug or a blocker. Every item is a deliberate deferral, YAGNI non-goal, or watch-item;
> all current contracts fail loud (rejected with a typed error) rather than silently misbehaving.

## Open items

Stable IDs are `B<n>`. "Altitude" = how deep a change it is: **additive** (non-breaking new surface),
**contained** (localized change, no contract break), or **new ADR** (contract/behaviour change needing a
spec+plan+ADR chain).

| ID | Title | Source | Category | Altitude | Priority |
|----|-------|--------|----------|----------|----------|
| ~~**B1**~~ | ~~Dot-path roll-up keys~~ | — | — | — | ✅ **Done** (incr 017, ADR-0042) |
| **B2** | `foreach` per-element scope-copy benchmark | ADR-0040 | perf/benchmark | contained | **P1** |
| **B3** | Numeric-coercing Scope getters | Spec 006 non-goal; `pipe/get.go` | feature-gap (ergonomics) | additive | **P2** |
| **B4** | `Hash()` rejects non-marshalable hand-built defs | ADR-0037 | hardening/tech-debt | contained (edge case) | **P2** |
| **B5** | Per-decision options in decision-table config | ADR-0007; Spec 004; `config/build.go:358` | feature-gap | new ADR | **P2** |
| **B6** | Precise member-path references in provenance | ADR-0011; Spec 006 non-goal | feature-gap/debuggability | new ADR | **P2** |
| **B7** | Intra-stage `MultiExpr` local-alias provenance | ADR-0011 ("Known limitations") | tech-debt/debuggability | new ADR | **P3** |
| **B8** | Per-element lineage beyond firing (`foreach`) | ADR-0040; Spec 015 D5 | feature-gap/debuggability | new ADR | **P3** |
| **B9** | Nested `foreach` support | Spec 015 D7; ADR-0040; `config/build.go:20` (`ErrNestedForEach`) | feature-gap | new ADR | **P3** |
| **B10** | Convenience constructors (`NewFromYAML`/nested `Pipeline` as `Stage`) | ADR-0009; ADR-0005 | ergonomics (YAGNI) | additive | **P3** |
| **B11** | Parallel execution of independent DAG stages | ADR-0006; ADR-0005 | perf/feature-gap | new superseding ADR | **P3** |
| **B12** | Strict env / host functions declarable in YAML | ADR-0028 ("Deferred within config") | feature-gap (likely permanent non-goal) | new ADR | **P4** |

### Details

**B1 — Dot-path roll-up keys. ✅ DONE (increment 017, ADR-0042).** `Rollup.Key` is now dot-path-aware —
`applyRollup` resolves it via the shared `lookupPath` helper, so a decision-table output (`<table>.<key>`)
rolls up directly with no companion `single-expr`. Backward-compatible (dot-free key unchanged); no
`Hash()`/schema change.

**B2 — `foreach` scope-copy benchmark.** Each element deep-copies the outer scope's map spine via
`Snapshot()`+`NewScope` — O(elements × outer-scope size). Fine for typical line-item counts; ADR-0040
flags a benchmark before large collections. Measure first (`cc-skills-golang:golang-benchmark`); optimize
only if it regresses.

**B3 — Coercing Scope getters.** Typed getters (`GetInt`/`GetFloat64`/…) are strict: a `float64` at an int
path or a numeric string is a `*ScopeTypeError`. A coercing variant was deferred as additive — can be added
without breaking the strict API.

**B4 — `Hash()` non-marshalable fallback.** A hand-built `PipelineDef` carrying a non-JSON-marshalable
value (`chan`/`func`) falls back to a stable placeholder hash and loses change-detection. Parse paths can
never produce such values, so this only affects defs built by hand in Go. Could validate/reject at `Hash()`.

**B5 — Per-decision decision-table options.** `stage.Rule` carries one rule-level `DecisionOptions` shared
across all decisions; config rejects a decision that declares its own `fallback`/`globals`
(`config/build.go:358`, "per-decision options are not supported; use a bare expression"). Bare-string
decisions are unaffected. Extending to per-decision options is a contract change.

**B6 — Member-path provenance references.** Provenance `Inputs` records top-level identifiers only (`a` for
`a.b.c`). Precise member-path lineage is a recorded future refinement to reference granularity.

**B7 — Intra-stage `MultiExpr` alias provenance.** Within a `MultiExpr`, a later expression reading an
earlier one by bare local name (`b = "a + 1"`) keys `Inputs` by the local name (`a`) while the value lives
at `<stage>.a`; prefix reconciliation links by path, not alias, so `Lineage`/`Explain` silently omit such
intra-stage subtrees. Cross-stage refs reconcile correctly. Fixing it changes the documented "`Inputs` is
keyed by referenced identifier" contract.

**B8 — Per-element lineage.** Per-element firing is recorded under `<stage>[i]`, but each element's full
derivation graph (when the outer scope tracks provenance) is discarded — only the data `Snapshot()` survives
in `items`. "Line i denied by rule X" is answerable; deeper per-element lineage is not yet surfaced.

**B9 — Nested `foreach`.** Nesting is rejected at build time (`ErrNestedForEach`); the D7 deferral is
*enforced*, but supporting an inner unit that itself iterates remains deferred (fan-out semantics, scoping,
error model to design).

**B10 — Convenience constructors.** `rlng.NewFromYAML`/`NewFromProvider` (compose an engine directly from a
config source) and `Pipeline` implementing `Stage` (nested pipelines) were deferred as YAGNI — additive if
desired.

**B11 — Parallel stage execution.** Pipeline execution is sequential & deterministic; parallel execution of
independent DAG stages is deferred ("stage counts are small"). Scope already carries a mutex partly to guard
this future path. A superseding ADR + concurrency design would be needed.

**B12 — YAML-declared env / host functions.** `WithEnv` (typed env schema) and `WithFunction` (host
functions) stay programmatic — an env schema needs Go types and functions are Go values. Recorded as a
deliberate omission; likely a permanent non-goal.

## Recently resolved deferrals (provenance of this sweep's dedup)

Deferrals found in the docs but confirmed already implemented — excluded from the open list:

| Deferral (as originally recorded) | Closed by |
|-----------------------------------|-----------|
| Dot-path roll-up keys (B1; ADR-0040) | Increment 017 / ADR-0042 |
| Exact decimal money (ADR-0030) | Increment 014 / ADR-0038 + ADR-0039 |
| `foreach` stage (ADR-0030) | Increment 015 / ADR-0040 |
| Config-declared output mapping (ADR-0009; Spec 005/008 non-goals) | Increment 010 / ADR-0028 |
| `VariablePatcher` config defaults / pipeline constants (Spec 004/005/008 non-goals) | Increment 010 / ADR-0028 |
| Scope JSON serialization of firing (ADR-0036 known gap) | Increment 013 |
| Serializing the lineage / derivation graph (Spec 006 non-goal) | Increment 007 |

## Notes
- No `TODO`/`FIXME`/`XXX`/`HACK`/`BUG` markers exist in non-test Go source (only `context.TODO()` in
  example tests). "deferred" in `config/*.go` refers to the deferred-provider concept (I/O at parse time),
  not backlog.
- Increment 016's one acknowledged-no-fix item (`urlsource.go` `http.NewRequestWithContext` error branch,
  unreachable via the public API) is not backlog work.
