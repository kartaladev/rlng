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
`4b3089e`); B5 (incr 021, `89b1d57`); B6 (incr 022, `56b243d`); B7 (incr 023, `9569ecc`); B8 (incr 024 —
this branch, feat commit; merge SHA in `git log` after merge).** B1–B8 complete. **B9 is next** (a
design-checkpoint item — pause for the user's design approval before implementing).

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

## Next action — B9 (increment 025): DESIGN CHECKPOINT (pause for user approval)
**B9 — Nested `foreach` support** (Spec 015 D7; ADR-0040; `config/build.go` `ErrNestedForEach`;
feature-gap; P3). Nesting is currently **rejected** at build time (`ErrNestedForEach`) — a foreach's inner
`Stages` list may not contain another foreach. Supporting an inner unit that itself iterates needs the
fan-out semantics, scoping, and error model designed (the D7 deferral). **This is a design-checkpoint
item** — brainstorm → write spec `docs/specs/025-*` → **PAUSE for the user's design approval before
implementing** (standing design-gating decision above). Then plan 025 + ADR-0050 ride with the feat commit;
full gate → auto merge+push. Read `pipe/foreach*.go`, `config/build.go` (`buildForEach`, `ErrNestedForEach`),
Spec 015 D7, ADR-0040 first. Note: B9 is a bigger design than B5–B8 (real new semantics, not a provenance
refinement) — expect a longer brainstorm.

Start: `git checkout -b <branch> main` (after this B8 branch is merged), brainstorm, author the spec.

## Exact state
- On branch `claude/foreach-per-element-lineage-024` off `main@9569ecc`. **B8 implementation complete and
  green**; spec 024 committed standalone (`ac52e63`), about to commit the single feat increment (plan 024 +
  ADR-0049 + BACKLOG/HANDOVER — this handover rides in it). After commit: whole-branch gate (`/code-review`
  + `/security-review` over `main..HEAD`) → auto merge+push + delete branch (standing authorization).
- Full gate green: `go build ./...`, `go test ./... -race` (all packages ok), `go vet`, `gofmt -l .`
  (empty), `CGO_ENABLED=0 go build`, `go mod tidy`(no-op)/`verify` all clean. `pipe` coverage 99.1%.
  `benchstat` installed at `$(go env GOPATH)/bin/benchstat`.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch (016's ledger); not used for the 017+ program — track via
  `docs/BACKLOG.md` + git log instead.
- Artifact numbering is continuous: specs/plans at **024 done**, ADRs at **0049 done**. **B9 = spec 025 +
  plan 025 + ADR-0050 next.**
