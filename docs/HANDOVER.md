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
`4b3089e`); B5 (incr 021, `89b1d57`); B6 (incr 022, `56b243d`); B7 (incr 023 — this branch, feat commit;
merge SHA in `git log` after merge).** B1–B7 complete. **B8 is next** (a design-checkpoint item — pause for
the user's design approval before implementing).

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
- **B7 / increment 023 (this branch `claude/multiexpr-local-alias-provenance-023`):** intra-stage
  `MultiExpr` local-alias provenance. A `MultiExpr` local-alias reference (first path segment names an
  earlier expr in the stage) is now keyed by its `<stage>.<name>` scope path when recording `Inputs`, so
  B6's exact/ancestor reconciliation traces the intra-stage subtree (`calc.taxed` → `calc.base` → seeds).
  D2 shadowing honored: the first expr named `x` reading seed `x` stays unqualified; a later expr reading
  the local `x` is qualified. Localized to `pipe/multi.go` via a keyed `snapshotRefs`; `single`/`table` and
  seed/cross-stage keys unchanged. Contract change (MultiExpr local `Inputs` keyed by path); no
  signature/`Hash()`/config change. Spec 023 committed standalone (`e804c28`); plan 023 + ADR-0048 ride in
  the feat commit. BACKLOG B7 → resolved.

## Next action — B8 (increment 024): DESIGN CHECKPOINT (pause for user approval)
**B8 — Per-element lineage beyond firing (`foreach`)** (ADR-0040; Spec 015 D5; feature-gap/debuggability;
P3). Per-element firing is recorded under `<stage>[i]`, but each element's full derivation graph (when the
outer scope tracks provenance) is discarded — only the data `Snapshot()` survives in `items`. "Line i denied
by rule X" is answerable; deeper per-element lineage is not yet surfaced. **This is a design-checkpoint
item** — brainstorm → write spec `docs/specs/024-*` → **PAUSE for the user's design approval before
implementing** (standing design-gating decision above). Then plan 024 + ADR-0049 ride with the feat commit;
full gate → auto merge+push.

Start: `git checkout -b <branch> main` (after this B7 branch is merged), brainstorm, author the spec.
Read `docs/specs/015-*`, ADR-0040, and the `foreach` code (`pipe/foreach*.go`) first.

## Exact state
- On branch `claude/multiexpr-local-alias-provenance-023` off `main@56b243d`. **B7 implementation complete
  and green**; spec 023 committed standalone (`e804c28`), about to commit the single feat increment (plan
  023 + ADR-0048 + BACKLOG/HANDOVER — this handover rides in it). After commit: whole-branch gate
  (`/code-review` + `/security-review` over `main..HEAD`) → auto merge+push + delete branch (standing
  authorization).
- Full gate green: `go build ./...`, `go test ./... -race` (all packages ok), `go vet`, `gofmt -l .`
  (empty), `CGO_ENABLED=0 go build`, `go mod tidy`(no-op)/`verify` all clean. `pipe` coverage 99.2%.
  `benchstat` installed at `$(go env GOPATH)/bin/benchstat`.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch (016's ledger); not used for the 017+ program — track via
  `docs/BACKLOG.md` + git log instead.
- Artifact numbering is continuous: specs/plans at **023 done**, ADRs at **0048 done**. **B8 = spec 024 +
  plan 024 + ADR-0049 next.**
