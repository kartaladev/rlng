# Spec 029 — post-audit refactor batch (R1–R9, R11)

- **Status:** Accepted
- **Backlog items:** R1–R9 and R11 (`docs/BACKLOG.md`, "Post-audit refactor items"). These are the
  **quality-only, behavior-preserving** refactors surfaced by the increment-029 whole-codebase audit and
  deliberately deferred until the audit's bug fixes had landed. **R10 is out of scope** (it is an additive
  feature — static same-level output-path collision detection — that needs its own small design + ADR).
- **Design approval:** the scope (all R-items except R10), the R1 kernel boundary, the unexported
  `deriveOrSet` visibility, and the single-ADR strategy (ADR-0054 for R1 only) were put to the user and
  approved during brainstorming (2026-07-14).
- **Realized by:** Plan 029; ADR-0054 (unify numeric coercion core — R1 only).

## Problem

The increment-029 audit fixed eight correctness/safety bugs and, in doing so, exposed a set of **duplication
and clarity** liabilities that are not bugs today but are the *soil* the bugs grew in. The headline example:
two independent reflect-based numeric-conversion paths — `pipe/get.go`
(`coerceToInt64`/`coerceToFloat64`) and `pipe/table.go` (`toInt64`/`toFloat64`/`asDecimal`/`classify`) —
diverged, and that divergence *was* pipe bug #3 (`int64(rv.Uint())` silently wrapping a `uint64 >
math.MaxInt64` in the aggregation path while the getter path overflow-checked the same conversion). The
remaining items are smaller repeated blocks (a 5-site provenance branch, near-duplicate map-copy helpers,
repeated engine-construction wiring) that raise the cost of every future change and invite the same
copy-diverge failure mode.

This increment pays that debt down as **one batch of behavior-preserving refactors**. Nothing here changes a
public contract, a typed error's identity, `Hash()`, or the config schema; the existing test suite is the
primary regression guard, and every refactor keeps `go test ./... -race` green.

## Goal

Remove the audit-identified duplication and improve altitude **without any observable behavior change**. For
every item: same inputs → same outputs, same errors (same sentinels, same wrapping), same `Hash()`, same
race-cleanliness. The deliverable is a smaller, harder-to-diverge internal surface — measured by the existing
tests staying byte-green and by new table-test cases pinning any *newly reachable* branch the refactor
introduces.

## Decisions

### R1 — one overflow-checked numeric core (the substantive item) — **ADR-0054**

**Constraint that shapes the design:** the two numeric paths have **different accepted type-sets and different
failure contracts**, so "unify" must not mean "merge behavior":

- `get.go` coercion is **fail-loud and text-accepting**: it converts `string` and `json.Number` in addition
  to the numeric kinds, and returns an error on overflow / non-integral / non-finite (wrapped by the caller
  into `*ScopeTypeError`).
- `table.go` folding is **trusting and text-rejecting**: `classify` first rejects `string`/`json.Number` as
  `kindNonNumeric` (→ `ErrNonNumericAggregate`) and promotes `decimal` (and `uint64 > MaxInt64`) to
  `kindDecimal`; `toInt64`/`toFloat64` then convert a value the caller *guarantees* is already the right kind,
  returning `0` on the (unreachable) unexpected input.

**Decision.** Extract only the **shared reflect kernel** — the overflow-checked int/uint→`int64`,
int/uint/float→`float64`, and int/uint/float/`decimal`→`decimal.Decimal` reflect conversions — into one
internal file, `pipe/numeric.go`. Both callers delegate to the kernel and keep their own **outer policy**:

- `get.go`'s `coerceToInt64`/`coerceToFloat64` keep their `string`/`json.Number` heads, then fall through to
  the kernel for the reflect cases (overflow-checked, exactly as today).
- `table.go`'s `classify` (kind ranking, decimal/uint64 promotion) is unchanged; `toInt64`/`toFloat64`/
  `asDecimal` become thin adapters over the kernel. Because `classify` has already excluded the failing
  cases, the fold path observes no new error — the kernel's overflow-checked `int64` conversion is only ever
  reached for a value `classify` ranked `kindInt` (i.e. `uint64 ≤ MaxInt64`), so it cannot fail there.

This deletes the duplicated reflect switch (the literal divergence surface behind bug #3) while every accepted
type-set and every error stays exactly as it is. The kernel is **unexported** (`internal to package pipe`) —
no public-API change.

### R2 — unexported `deriveOrSet` helper (DRY, no ADR)

Five sites (`single.go` ×1, `multi.go` ×1, `table.go` ×3: `writeDecision`, `writeCollected`, `writeAgg`)
repeat the shape `if sc.TracksProvenance() { build a Derivation (incl. snapshotRefs); Derive } else { Set }`.
Collapse them into one **unexported** method:

```go
func (s *Scope) deriveOrSet(path string, v any, build func() Derivation) error
```

`build` is a **lazy closure**: it is invoked only when provenance is on, so the expensive
`snapshotRefs(...)` (and, for `multi`, `snapshotRefsKeyed`) still never runs when provenance is off —
preserving today's zero-cost-when-off property exactly. `deriveOrSet` internally reuses the existing `Derive`
(which already no-ops its recording branch when provenance is off) and `Set`. **Unexported** per the approved
visibility decision: all current callers live in package `pipe`, and keeping the public surface minimal is
preferred; an external `Stage` implementation continues to use the exported `Derive`/`Set`. No public-API,
`Hash()`, or behavior change.

### The mechanical items (no ADR; documented here, detailed in Plan 029)

Each is a localized behavior-preserving cleanup covered by existing tests:

- **R7 — gate `p.wide` on `maxParallel != 1`** (`pipe/pipeline.go`). A size-1 parallel cap can never overlap
  two stages, yet today it still marks the `Scope` concurrent and pays `Snapshot`'s deep-copy cost. Gate the
  `wide`/`markConcurrent` path so a `WithMaxParallel(1)` pipeline behaves like — and costs like — sequential.
  Behavior-preserving: output is already identical; only the wasted deep-copy is removed. Covered by a test
  asserting a `WithMaxParallel(1)` run produces the same `Scope` as sequential (and does not trip the
  concurrent deep-copy path).
- **R8 — shared locked prefix-rekey merge helper** (`pipe/provenance.go` `recordElementDerivations` +
  `pipe/firing.go` `recordElementFirings`). Both take `s.mu`, iterate `src`, and re-key each entry under
  `prefix + "." + key`. Extract the shared locked-merge skeleton; the per-entry rewrite (derivations also
  rewrite `Path` and `Inputs` keys; firings only re-key) stays caller-specific. If the shared surface proves
  too thin to be worth a helper, this item may be **closed as not-worth-it** in the plan with a one-line
  rationale (it is P4).
- **R3 — fold `truthy`'s exact `bool`/`string` head into the reflect switch** (`expr/predicate.go`). The
  exact-type head became redundant once the audit added the `reflect.Bool`/`reflect.String` cases; remove the
  redundant head so one switch handles both. Covered by the existing lenient-truthiness table.
- **R4 — table-drive `decimalExprOptions` add/sub/mul/div** (`expr/decimal.go`). Replace the four
  near-identical option constructions with one table. Covered by existing decimal-arithmetic tests.
- **R9 — collapse near-duplicate map-copy helpers** (`expr/options.go` `mergeInto` / `expr/variables.go`
  `copyMap`). Unify into one helper (or express one in terms of the other). P4; covered by existing option/
  variable-patching tests.
- **R6 — hoist the known-field map to a package var; collapse the
  `withStrictEnv(withConstants(…))` wrapper** (`config/expr_def.go`, `config/build.go`, 5 sites). The
  known-field set is rebuilt per call; hoist it to a package-level `var`. Collapse the nested option-wrapper
  into a single composed helper. Covered by existing config parse/build tests.
- **R11 — memoize the content hash on the built pipeline** (`config/hash.go` / `config/build.go`).
  `canonicalJSON` deep-walks the def on every `Hash()` / `MatchesRuleset` call. Compute the hash once at
  build time and memoize it on the built value; `Hash()` returns the cached bytes. Behavior-preserving: the
  hash value is byte-identical (same `canonicalJSON`), only recomputation is removed. The **direct-`Hash()`
  fallback** for hand-built defs (ADR-0045) is preserved. Covered by a test asserting repeated `Hash()` calls
  return equal bytes and that a memo hit does not re-walk (e.g. equal to the pre-memo value).
- **R5 — extract `newEngineConfig(opts…)` + a shared parse→build helper** (`engine.go`, `typed_engine.go`,
  `fromconfig.go`, 4 sites). The engine constructors repeat option-config assembly and the
  `config.Parse → PipelineDef.Build → New*Engine` wiring. Extract the shared config assembly and the shared
  parse→build step. Done **last** because it touches the most files. Covered by existing engine-construction
  and convenience-constructor tests (including the typed variants).

## Non-goals

- **R10 (static same-level output-path collision detection)** — an *additive feature* (needs a `Stage`
  output-path surface + likely an ADR), not a behavior-preserving refactor; excluded from this batch and left
  in the backlog.
- **Any behavior, contract, error-identity, `Hash()`, or config-schema change** — this batch is quality-only.
  If any item is found to *require* a behavior change to proceed, it is dropped from the batch and re-scoped as
  its own spec, not smuggled in here.
- **New public API** — R1's kernel and R2's `deriveOrSet` are both unexported. No exported symbol is added,
  removed, or changed; `gorelease`/`apidiff` against the last tag must report no change.
- **Performance tuning beyond removing the identified waste** — R7 and R11 remove obviously-wasted work
  (a needless deep-copy; a needless re-walk); no further micro-optimization is in scope.

## Success criteria / hot-path branches to cover

All tests: `table-test` assert-closure form, blackbox `pipe_test` / `expr_test` / `config_test` / `rlng_test`
packages, `t.Context()` where a context is involved, and `go test ./... -race` green throughout.

1. **R1 — kernel parity through the public surface.** The coercing getters (`GetIntCoerce`,
   `GetInt64Coerce`, `GetFloat64Coerce`) and the collect aggregations (`AggregateSum`/`Min`/`Max` over
   int/uint/float/decimal mixes) produce identical results and identical errors (same sentinels:
   `*ScopeTypeError`, `ErrNonNumericAggregate`, `ErrAggregateOverflow`, `ErrNonFiniteAggregate`) before and
   after the refactor. Each kernel branch — `int64` overflow (`uint64 > MaxInt64`), float non-finite /
   non-integral / overflow, `decimal` from full-`uint64` via `big.Int` — has a covering case exercised
   through a public entry point.
2. **R1 — bug-#3 regression stays fixed.** A `uint64 > math.MaxInt64` value: overflow-errors through
   `GetInt64Coerce`, and promotes to exact `decimal` (not a wrapped `int64`) through `AggregateSum` — the
   exact behaviors the audit established.
3. **R2 — provenance on/off both green.** With `WithProvenance` on, `single`/`multi`/`decision-table`
   derivations (paths, `Inputs`, `Operation` labels) are unchanged; with provenance off, `snapshotRefs` is
   never invoked (the lazy `build` closure is not called) and outputs match `Set` behavior. Existing
   provenance/lineage tests stay byte-green.
4. **R7 — `WithMaxParallel(1)` equals sequential** in final `Scope` data and does not trip the concurrent
   deep-copy path; `WithMaxParallel(2)`+ still races clean (existing concurrency regression guard
   `TestPipelineConcurrencyNoSharedNestedMapRace` stays green).
5. **R11 — memoized hash is stable and equal.** Repeated `Hash()` / `MatchesRuleset` on a built pipeline
   return byte-equal results equal to the pre-memo `canonicalJSON` value; the hand-built direct-`Hash()`
   fallback (ADR-0045) is unaffected.
6. **R3/R4/R6/R9/R5 — existing suites stay green.** Each mechanical item is validated by its package's
   existing tests remaining byte-green; where a refactor introduces a newly-reachable branch, a table case is
   added to the relevant existing table.
7. **Whole-tree gates.** `go build ./...`, `go vet ./...`, `gofmt`/`gofumpt` clean, `go test ./... -race`
   green, and (per the delivery gate) `/code-review` + `/security-review` over `main..HEAD` resolved/triaged
   before the final increment commit. `apidiff` reports no exported-surface change.

## Traceability

Backlog: R1–R9, R11 (`docs/BACKLOG.md`). Plan: 029. ADR: **0054** (unify numeric coercion core — R1 only;
records the kernel-boundary decision and why the two callers keep distinct outer type-set policies). R2–R9 and
R11 are non-architectural cleanups recorded in this spec and Plan 029, with no dedicated ADR. Related source
artifacts the items derive from: the increment-029 audit (pipe #2/#3, expr #4, review C1–C5) captured in
`docs/BACKLOG.md`; ADR-0044 (coercing getters — R1's `get.go` side), ADR-0038/0039 (exact decimal — R1's
`asDecimal` side), ADR-0052 (parallel execution — R7's `p.wide`/`markConcurrent` gate), ADR-0045 (`Hash()`
non-marshalable fallback — R11 preserves it), ADR-0011/0047/0048/0049 (provenance — R2's write sites).
