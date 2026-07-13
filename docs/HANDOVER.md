# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then `docs/BACKLOG.md`
> (the tracked backlog B1–B12), then `git log` on `main`. For the item you pick up, read its
> `docs/specs/*` + `docs/plans/*` + related ADRs first. Trust the repo (code + git history + `docs/`) over
> any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Increments 011–016 merged & pushed. **Increment 016 (config `Provider`) complete.**

**Active program: execute ALL 12 backlog items (B1–B12) from `docs/BACKLOG.md`,** each as its own
increment. **B1 (incr 017, `f10a8be`); B2 (incr 018, `934f1b5`); B3 (incr 019, `2af21f2`); B4 (incr 020,
`4b3089e`); B5 (incr 021, `89b1d57`); B6 (incr 022, `56b243d`); B7 (incr 023, `9569ecc`); B8 (incr 024,
`e6ed96e`); B9 (incr 025, `612b20b` + docs close-out); B10 (incr 026 — this branch, feat commit next in
`git log`; constructors done, Pipeline-as-Stage re-deferred, ADR-0051).** B1–B10 complete. **B11 is next.**
Per the standing design-gating decision, B11 **is a design-checkpoint item** — it reverses a non-goal
(ADR-0006) and needs a superseding ADR **plus a live approval pause** before implementation.

## Standing decisions for this program (do NOT re-ask)
- **Scope = all 12**, including the two deliberate non-goals **B11** (parallel exec; write a superseding
  ADR for ADR-0006 first) and **B12** (YAML env/host functions; superseding ADR for ADR-0028 first).
- **Cadence = AUTO merge+push each green increment** (standing authorization). After each increment's full
  gate passes, merge to `main`, push, delete branch, start the next — no per-increment merge/push approval.
  Does NOT extend to release tags (still need explicit approval).
- **Design-gating = checkpoint only the risky ones.** Design + implement **B2, B3, B4, B10** autonomously
  (surface design via the committed spec, no live approval pause). **PAUSE for the user's design approval**
  before implementing **B5, B6, B7, B8, B9, B11, B12**. Code-review + security-review gates run on every
  increment regardless.
- Per-increment workflow: brainstorm → spec `docs/specs/NNN` → plan `docs/plans/NNN` + ADR `docs/adrs/NNNN`
  → TDD (red/green) → `/code-review high main..HEAD` + `/security-review` → resolve findings → merge+push.
  (See memory `rlng-backlog-execution-program` for the full authorization record.)

## What's DONE this session
- **B5 / increment 021 (`main@89b1d57`, merged+pushed):** per-decision options in decision tables (Option A).
  `pipe.Rule.Decisions` → `map[string]pipe.Decision` (new exported `Decision{Expr, Options}`); shared
  `Rule.DecisionOptions` removed; `WithDefault(map[string]pipe.Decision)`; config `decisionsFrom` threads
  each `ExprDef`'s options. Breaking pre-1.0 `pipe` API; no config-schema/`Hash()` change. Spec/plan 021,
  ADR-0046. Code-review: 1 finding (stale README) fixed; security-review: clean.
- **B6 / increment 022 (`main@56b243d`):** precise member-path provenance references. `expr.References()`
  returns the deepest static member path per reference (`grade.tier`, not top-level `grade`); `snapshotRefs`
  resolves via `lookupPath`; `derivationsFor` gained a nearest-ancestor fallback (exact → descendants →
  ancestor). `References()` signature unchanged (provenance-only consumer). Spec/plan 022, ADR-0047.
- **B7 / increment 023 (`main@9569ecc`):** intra-stage `MultiExpr` local-alias provenance. A local-alias
  reference (first path segment names an earlier expr in the stage) is keyed by its `<stage>.<name>` scope
  path when recording `Inputs`, so B6's exact/ancestor reconciliation traces the intra-stage subtree; D2
  shadowing honored. Localized to `pipe/multi.go` via a keyed `snapshotRefs`. Spec/plan 023, ADR-0048.
- **B8 / increment 024 (this branch `claude/foreach-per-element-lineage-024`):** per-element `foreach`
  lineage. When the outer scope tracks provenance, each element's full derivation graph is merged onto the
  outer scope under the `<name>[i].` path prefix (paths + `Inputs` keys rewritten), so
  `Lineage`/`Explain`/`Derivations` answer per-element lineage (`<name>[i].<inner output>` → element seed)
  via B6's exact/ancestor reconciliation, alongside the existing per-element firing. Always-on when
  provenance is on; zero cost off. New unexported `Scope.recordElementDerivations`; `ForEach.Execute` calls
  it per element. No data/`Hash()`/config/eval change. Spec 024 committed standalone (`ac52e63`); plan 024 +
  ADR-0049 ride in the feat commit. BACKLOG B8 → resolved.
- **B9 / increment 025 (this branch `claude/nested-foreach-025`):** nested `foreach` support. A `foreach`
  stage's inner `Stages` may now contain another `foreach` (no depth cap); per-element firing keys compose
  hierarchically (`<outer>[i].<inner>[j].<table>`), and lineage composes for free via B6's exact/ancestor
  reconciliation (no new provenance code). Each nesting level must bind a distinct `as` name — a collision
  is rejected at build time via the new `config.ErrForEachAsCollision` sentinel. The prior `ErrNestedForEach`
  build-time gate is removed. Breaking pre-1.0 changes: the per-element firing-key shape (`!` on the `pipe`
  commit) and the `ErrNestedForEach` removal (`!` on the `config` commit). Spec 025 committed standalone
  (`47e77df`); plan 025 + ADR-0050 ride in the feat commits (`d63a05c`, `612b20b`); acceptance example +
  BACKLOG/HANDOVER close-out in this docs commit. BACKLOG B9 → resolved.
- **B10 / increment 026 (this branch `claude/convenience-constructors-026`):** convenience constructors.
  `rlng.NewFromProvider`/`NewFromYAML` and typed `NewTypedFromProvider[I,R]`/`NewTypedFromYAML[I,R]`
  (`fromconfig.go`) compose `config.Parse -> PipelineDef.Build -> New/NewTypedEngine` in one call, ctx-first,
  default `Build()`, engine `Option`s threaded, errors passed through unwrapped (no new sentinel).
  Introduces an additive in-module `rlng -> config` import — no new external dependency (`go mod tidy`
  stays a no-op), acyclic. Pipeline-as-Stage (the other half of B10) is **re-deferred** — reversing
  ADR-0005 has marginal value now that `foreach` covers per-element sub-pipelines; needs a superseding ADR
  + concrete use case to revisit. Spec 026 committed standalone (`400b27d`); plan 026 + ADR-0051 ride in
  the feat commit. BACKLOG B10 → resolved (constructors half).

## Next action — B11 (increment 027): design-checkpoint — PAUSE for approval
**B11 — Parallel execution of independent DAG stages** (`docs/BACKLOG.md`; ADR-0006; ADR-0005;
perf/feature-gap; new superseding ADR; P3). Pipeline execution is currently sequential & deterministic
(ADR-0006); `Scope` already carries a mutex partly to guard this future path. This is **both** a
design-checkpoint item **and** a non-goal reversal: it requires authoring a **superseding ADR to ADR-0006**
(concurrency design — worker pool vs errgroup fan-out, error/cancellation semantics across parallel
branches, whether `Scope` mutation stays safe as-is or needs restructuring) **and**, per the standing
design-gating decision, a **live approval pause** before implementation — do not implement without the
user signing off on the design. Read `pipe/pipeline.go` (DAG/topo-sort execution), ADR-0006, ADR-0005, and
the B11 backlog entry first; brainstorm and author the spec + draft ADR, then stop and ask for approval
before writing code.

Start: `git checkout -b <branch> main` (after this B10 branch is merged), brainstorm, author the spec and
draft superseding ADR, then PAUSE for approval.

## Exact state
- On branch `claude/convenience-constructors-026` off `main@e6ed96e`. **B10 implementation complete and
  green** (this commit). After commit: whole-branch gate (`/code-review` + `/security-review` over
  `main..HEAD`) → auto merge+push + delete branch (standing authorization).
- Full gate green: `go build ./...`, `go test ./... -race` (all packages ok), `go vet`, `gofmt -l .`
  (empty), `CGO_ENABLED=0 go build`, `go mod tidy`(no-op)/`verify` all clean.
  `benchstat` installed at `$(go env GOPATH)/bin/benchstat`.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch (016's ledger); not used for the 017+ program — track via
  `docs/BACKLOG.md` + git log instead.
- Artifact numbering is continuous: specs/plans at **026 done**, ADRs at **0051 done**. **B11 needs a new
  spec/plan + a superseding ADR to ADR-0006 — and a design-approval pause before implementation.**
