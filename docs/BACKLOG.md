# rlng — consolidated backlog (tracked)

> Living document. Last swept: 2026-07-13, across all increments through **016** (config source `Provider`).
> Each item cites the ADR/spec/code that records it — **trust those source artifacts over this summary**;
> this file only aggregates and prioritizes. When an item is picked up, it graduates to a `docs/specs/*` →
> `docs/plans/*` → `docs/adrs/*` chain per CLAUDE.md, and moves to the "Resolved" section here (with the
> closing increment/ADR) rather than being deleted.
>
> Nothing below is a bug or a blocker. Every item is a deliberate deferral, YAGNI non-goal, or watch-item;
> all current contracts fail loud (rejected with a typed error) rather than silently misbehaving.
>
> **✅ PROGRAM COMPLETE (2026-07-13): B1–B12 all resolved.** Every tracked backlog item has been executed
> (or closed as a documented non-goal). The B1–B12 table below is retained as the historical record.
>
> **🔧 POST-AUDIT REFACTOR BACKLOG (2026-07-14, R-items).** A whole-codebase audit (increment 029) fixed a
> batch of correctness/safety bugs and surfaced a set of **refactor / quality** opportunities. These are
> quality-only (no behavior change) and are deferred to be revisited after the bug fixes land. See the
> "Post-audit refactor items" section below.

## Post-audit refactor items (R1–R10)

Quality-only; each is a behavior-preserving refactor (or a small additive guard). Stable IDs `R<n>`. Priority
is a rough ordering, not a commitment.

| ID | Title | Source (audit finding) | Where | Priority |
|----|-------|------------------------|-------|----------|
| **R1** | Unify numeric coercion into one overflow-checked core | pipe #3 / #2 + review C2 | `pipe/table.go` (`toInt64`/`toFloat64`/`asDecimal`/`classify`) vs `pipe/get.go` (`coerceToInt64`/`coerceToFloat64`) | P2 |
| **R2** | `Scope.DeriveOrSet(path, v, buildDerivation)` helper | review C1 | duplicated `if TracksProvenance(){Derive}else{Set}` + `snapshotRefs` across `single.go`/`multi.go`/`table.go` (5 sites) | P2 |
| **R3** | Fold `truthy`'s exact `bool`/`string` head into the reflect switch | expr #4 / review C1 | `expr/predicate.go` — the exact-type head is now redundant with the added `reflect.Bool`/`reflect.String` cases | P3 |
| **R4** | Table-drive `decimalExprOptions` add/sub/mul/div | expr review C2 | `expr/decimal.go` | P3 |
| **R5** | Extract `newEngineConfig(opts…)` + shared parse→build helper | root review C1/C2 | `engine.go`/`typed_engine.go`/`fromconfig.go` (4 sites) | P3 |
| **R6** | Hoist known-field map to a package var; collapse `withStrictEnv(withConstants(…))` wrapper | config review C2 | `config/expr_def.go`, `config/build.go` (5 sites) | P3 |
| **R7** | Gate `p.wide` on `maxParallel != 1` (a size-1 cap never overlaps, yet pays deep-copy cost) | pipe review C3 | `pipe/pipeline.go` | P3 |
| **R8** | Shared locked prefix-rekey merge helper | pipe review C5 | `pipe/provenance.go` `recordElementDerivations` + `firing.go` `recordElementFirings` | P4 |
| **R9** | Collapse near-duplicate map-copy helpers (`mergeInto`/`copyMap`) | expr review C4 | `expr/options.go`, `expr/variables.go` | P4 |
| **R10** | Static same-level output-path collision detection at `NewPipeline` (the deterministic guard for the write-write caveat) | pipe #5 | `pipe/pipeline.go` + a `Stage` output-path surface | P3 (additive feature; needs a small design) |
| **R11** | Memoize the content hash on the built pipeline (canonicalJSON deep-walks the def on every `Hash()`/`MatchesRuleset`) | incr-029 code-review | `config/hash.go` / `config/build.go` | P4 |

> R1 and R2 are the highest-value: R1 removes the duplicated numeric-reflection paths whose divergence caused
> pipe bug #3 (a unified overflow-checked core prevents recurrence); R2 removes the most-copied hot-path block.
> R10 is the only non-trivial one (a small feature + likely an ADR) — the rest are contained refactors.

## Historical: B1–B12 (all resolved)

Stable IDs are `B<n>`. "Altitude" = how deep a change it is: **additive** (non-breaking new surface),
**contained** (localized change, no contract break), or **new ADR** (contract/behaviour change needing a
spec+plan+ADR chain).

| ID | Title | Source | Category | Altitude | Priority |
|----|-------|--------|----------|----------|----------|
| ~~**B1**~~ | ~~Dot-path roll-up keys~~ | — | — | — | ✅ **Done** (incr 017, ADR-0042) |
| ~~**B2**~~ | ~~`foreach` per-element scope-copy benchmark~~ | — | — | — | ✅ **Done** (incr 018, ADR-0043) |
| ~~**B3**~~ | ~~Numeric-coercing Scope getters~~ | — | — | — | ✅ **Done** (incr 019, ADR-0044) |
| ~~**B4**~~ | ~~`Hash()` rejects non-marshalable hand-built defs~~ | — | — | — | ✅ **Done** (incr 020, ADR-0045) |
| ~~**B5**~~ | ~~Per-decision options in decision-table config~~ | — | — | — | ✅ **Done** (incr 021, ADR-0046) |
| ~~**B6**~~ | ~~Precise member-path references in provenance~~ | — | — | — | ✅ **Done** (incr 022, ADR-0047) |
| ~~**B7**~~ | ~~Intra-stage `MultiExpr` local-alias provenance~~ | — | — | — | ✅ **Done** (incr 023, ADR-0048) |
| ~~**B8**~~ | ~~Per-element lineage beyond firing (`foreach`)~~ | — | — | — | ✅ **Done** (incr 024, ADR-0049) |
| ~~**B9**~~ | ~~Nested `foreach` support~~ | — | — | — | ✅ **Done** (incr 025, ADR-0050) |
| ~~**B10**~~ | ~~Convenience constructors~~ (constructors ✅; Pipeline-as-Stage re-deferred) | ADR-0009; ADR-0005 | ergonomics | additive | ✅ **Done** (incr 026, ADR-0051) — constructors; Pipeline-as-Stage still deferred |
| ~~**B11**~~ | ~~Parallel execution of independent DAG stages~~ | ADR-0006; ADR-0005 | perf/feature-gap | new superseding ADR | ✅ **Done** (incr 027, ADR-0052) |
| ~~**B12**~~ | ~~Strict env / host functions declarable in YAML~~ | ADR-0028 | — | — | ✅ **Done** (incr 028, ADR-0053) — env half already shipped (ADR-0031); host functions closed as non-goal |

### Details

**B1 — Dot-path roll-up keys. ✅ DONE (increment 017, ADR-0042).** `Rollup.Key` is now dot-path-aware —
`applyRollup` resolves it via the shared `lookupPath` helper, so a decision-table output (`<table>.<key>`)
rolls up directly with no companion `single-expr`. Backward-compatible (dot-free key unchanged); no
`Hash()`/schema change.

**B2 — `foreach` scope-copy benchmark. ✅ DONE (increment 018, ADR-0043).** `BenchmarkForEachScopeCopy`
(`pipe/foreach_bench_test.go`) measured the per-element `Snapshot()`+`NewScope` cost across collection size
× outer-scope shape: linear in both axes, sub-millisecond for typical line-item counts (~5 ms only at a
1000-element × 64-key extreme). Accepted as the price of the per-element isolation invariant; no
optimization now (ADR-0043 records the direction if a very-large-collection need ever arises).

**B3 — Coercing Scope getters. ✅ DONE (increment 019, ADR-0044).** Added `GetIntCoerce`/`GetInt64Coerce`/
`GetFloat64Coerce` (`pipe/get.go`): opt-in coercing variants accepting a wider type set (integer kinds
overflow-checked, integral finite floats, `json.Number`, numeric strings) converted safely/honestly per
ADR-0035 (no silent truncation, never manufacture `NaN`/`±Inf`, fail loud with `*ScopeTypeError`). Strict
getters unchanged (additive, no SemVer break).

**B4 — `Hash()` non-marshalable fallback. ✅ DONE (increment 020, ADR-0045).** `Build` now rejects a
hand-built def carrying a non-JSON-marshalable value (in any `any`-typed field) with a `*ConfigError`
wrapping the new `ErrUnhashableDef` sentinel, instead of silently stamping the `{}` placeholder identity.
`Hash()`/`MatchesRuleset` signatures unchanged (the direct-`Hash()` fallback is retained + documented);
parse paths unaffected; existing hashes byte-identical.

**B5 — Per-decision decision-table options. ✅ DONE (increment 021, ADR-0046).** `pipe.Rule.Decisions` is
now `map[string]pipe.Decision` (`Decision{Expr, Options}`) and the shared `Rule.DecisionOptions` field is
removed; `WithDefault` takes `map[string]pipe.Decision`. Each decision (rule or default) carries its own
`fallback`/`globals`/`coerce`, honored end-to-end. Config's `decisionsFrom` threads each `ExprDef`'s
options through (composing with constants + strict env); the old "per-decision options are not supported"
rejection is deleted. Breaking pre-1.0 `pipe` API change (Option A); no config-schema or `Hash()` change
(parsed `PipelineDef` shape untouched — pre-021 rulesets hash byte-identically).

**B6 — Member-path provenance references. ✅ DONE (increment 022, ADR-0047).** `expr.References()` now
returns the deepest statically-known member path per reference (`grade.tier`, not top-level `grade`);
`snapshotRefs` resolves each via `lookupPath` (precise `Inputs` values); `derivationsFor` gained a
nearest-ancestor fallback (exact → descendants → ancestor) so a member-path input links to the top-level
seed. `Explain`/`Lineage` no longer fan out to unread sibling outputs. `References()` signature unchanged
(provenance-only consumer, semantics-only change); no `Hash()`/config change.

**B7 — Intra-stage `MultiExpr` alias provenance. ✅ DONE (increment 023, ADR-0048).** A `MultiExpr`
local-alias reference (first path segment names an earlier expr in the stage) is now keyed by its
`<stage>.<name>` scope path when recording `Inputs`, so B6's exact/ancestor reconciliation traces the
intra-stage subtree (`calc.taxed` → `calc.base` → seeds). Localized to `pipe/multi.go` via a keyed
`snapshotRefs`; `single`/`table` and seed/cross-stage keys unchanged. Contract change (MultiExpr local
`Inputs` keyed by path, not bare name); no signature/`Hash()`/config change.

**B8 — Per-element lineage. ✅ DONE (increment 024, ADR-0049).** Each element's full derivation graph is now
merged onto the outer scope under the `<stage>[i].` path prefix when the outer scope tracks provenance
(paths + `Inputs` keys rewritten), so `Lineage`/`Explain`/`Derivations` answer per-element lineage
(`<stage>[i].<inner output>` → element seed) via B6's exact/ancestor reconciliation, alongside the existing
per-element firing. Always-on when provenance is on; zero cost off; no data/`Hash()`/config change.

**B9 — Nested `foreach`. ✅ DONE (increment 025, ADR-0050).** A `foreach` stage's inner `Stages` list may now
contain another `foreach`, iterating without a depth cap. Per-element firing keys compose hierarchically
(`<outer>[i].<inner>[j].<table>`), so a decision on the innermost element stays explainable down to the
exact (outer, inner) pair; lineage composes the same way with no new provenance code (B6's exact/ancestor
reconciliation already handles the deeper paths). Each nesting level must bind a distinct `as` name — the
`as`-chain guard rejects a collision (`config.ErrForEachAsCollision`) at build time rather than allowing a
silent shadow. The prior D7 deferral is resolved; there is no remaining nesting-depth gate.

**B10 — Convenience constructors. ✅ PARTIALLY DONE (increment 026, ADR-0051) — constructors shipped;
Pipeline-as-Stage remains deferred.** `rlng.NewFromProvider`/`NewFromYAML` and typed
`NewTypedFromProvider[I,R]`/`NewTypedFromYAML[I,R]` now compose `config.Parse -> PipelineDef.Build ->
New/NewTypedEngine` in one call (`fromconfig.go`), introducing an additive in-module `rlng -> config`
import (no new external dependency) — the convenience ADR-0009 anticipated. `Pipeline` implementing `Stage`
(nested pipelines) **remains deferred**: it would reverse ADR-0005, and has marginal value now that
`foreach` already owns per-element sub-pipelines and a flat nested pipeline is ≈ inlining its stages
(shared scope + DAG) — it would add naming/shared-scope-bookkeeping/collision semantics for no concrete
demand. Revisit with a superseding ADR to ADR-0005 if a real composition need appears.

**B11 — Parallel stage execution. ✅ DONE (increment 027, ADR-0052).** `Pipeline` gained opt-in parallel
execution of independent DAG stages via `WithConcurrency()` / `WithMaxParallel(n)` (and the config-path
`config.WithConcurrency`/`WithMaxParallel` BuildOptions + convenience-constructor `rlng.WithConcurrency`/
`WithMaxParallel` Options). Wave-based (level-barrier) scheduling runs each dependency level concurrently;
execution stays fully deterministic — final `Scope`, surfaced error (topo-min selection), and reported stage
order are identical to sequential. `Scope` needed no structural change (existing mutex + Spec-002 namespace
isolation; `-race` clean). Sequential remains the default. `NewPipeline` was consolidated onto
`(stages []Stage, opts ...PipelineOption)` and `WithRuleset` moved from a fluent method to an option (pre-1.0
breaking). ADR-0052 supersedes ADR-0006; the deferral is resolved.

**B12 — YAML-declared env / host functions. ✅ DONE (increment 028, ADR-0053).** Split into its two halves:
the **strict-typed-env half was already delivered** by ADR-0031 (increment 011) — the top-level `schema`
block makes strict env declarable in YAML, so a field typo fails at `Build`. The **host-function half is
closed as a deliberate non-goal**: arbitrary Go functions cannot (and should not) be serialized into YAML
without a plugin/interpreter that would break pure-Go debuggability and the minimal-safe-surface constraint;
the two feasible variants (expression-bodied functions; host-registered allowlist selection) are YAGNI /
marginal and are recorded in ADR-0053 as considered-and-rejected re-entry points. `WithFunction` stays
programmatic. No runtime code change; no `Hash()`/schema/API impact. Spec 028, ADR-0053 (supersedes the
"Deferred within config" bullet of ADR-0028). **This was the last open item — the B1–B12 program is
complete.**

## Recently resolved deferrals (provenance of this sweep's dedup)

Deferrals found in the docs but confirmed already implemented — excluded from the open list:

| Deferral (as originally recorded) | Closed by |
|-----------------------------------|-----------|
| Dot-path roll-up keys (B1; ADR-0040) | Increment 017 / ADR-0042 |
| `foreach` per-element scope-copy benchmark (B2; ADR-0040) | Increment 018 / ADR-0043 |
| Numeric-coercing Scope getters (B3; Spec 006 non-goal) | Increment 019 / ADR-0044 |
| `Hash()` rejects non-marshalable hand-built defs (B4; ADR-0037) | Increment 020 / ADR-0045 |
| Per-decision decision-table options (B5; ADR-0007 §5 / Spec 004) | Increment 021 / ADR-0046 |
| Member-path provenance references (B6; ADR-0011 point 4) | Increment 022 / ADR-0047 |
| Intra-stage MultiExpr local-alias provenance (B7; ADR-0011 known limitation) | Increment 023 / ADR-0048 |
| Per-element foreach lineage (B8; ADR-0040 D5 / Spec 015 D5) | Increment 024 / ADR-0049 |
| Nested foreach (B9; ADR-0040 D7 / Spec 015 D7) | Increment 025 / ADR-0050 |
| rlng.NewFromYAML convenience (B10; ADR-0009 deferral) | Increment 026 / ADR-0051 |
| Parallel stage execution (B11; ADR-0006 deferral) | Increment 027 / ADR-0052 |
| YAML-declared env / host functions (B12; ADR-0028 deferral) | env half: incr 011 / ADR-0031; functions: non-goal, incr 028 / ADR-0053 |
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
