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
`4b3089e`); B5 (incr 021, `89b1d57`); B6 (incr 022 — this branch, feat commit; merge SHA in `git log`
after merge).** B1–B6 complete. **B7 is next** (a design-checkpoint item — pause for the user's design
approval before implementing).

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
- **B6 / increment 022 (this branch `claude/member-path-provenance-022`):** precise member-path provenance
  references. `expr.References()` now returns the deepest static member path per reference (`grade.tier`,
  not top-level `grade`); `snapshotRefs` resolves via `lookupPath`; `derivationsFor` gained a
  nearest-ancestor fallback (exact → descendants → ancestor) linking a member-path input to the top-level
  seed. `Explain`/`Lineage` no longer fan out to unread sibling outputs. `References()` **signature
  unchanged** (provenance-only consumer, semantics-only change); no `Hash()`/config change. Landed as two
  green commits: `d699892` (pipe path-aware reconciliation, behavior-preserving prep) + the feat commit
  (expr member-path extraction + ADR-0047 + docs). Spec 022 committed standalone (`38cb419`). BACKLOG B6 →
  resolved.

## Next action — B7 (increment 023): DESIGN CHECKPOINT (pause for user approval)
**B7 — Intra-stage `MultiExpr` local-alias provenance** (ADR-0011 "Known limitations"; tech-debt/
debuggability; P3). Within a `MultiExpr`, a later expression reading an earlier one by its **bare local
name** (`b = "a + 1"`) keys `Inputs` by the local name (`a`) while the value lives at `<stage>.a`; prefix
reconciliation links by path, not alias, so `Lineage`/`Explain` silently omit such intra-stage subtrees.
Fixing it means qualifying local aliases to their `<stage>.<name>` path when recording `Inputs`, which
changes the documented "`Inputs` is keyed by referenced identifier" contract. **This is a design-checkpoint
item** — brainstorm → write spec `docs/specs/023-*` → **PAUSE for the user's design approval before
implementing** (standing design-gating decision above). Then plan 023 + ADR-0048 ride with the feat commit;
full gate → auto merge+push. Note: B6's member-path work is adjacent — a cross-stage ref like `stage.field`
already reconciles; B7 is specifically the same-stage local alias.

Start: `git checkout -b <branch> main` (after this B6 branch is merged), brainstorm, author the spec.

## Exact state
- On branch `claude/member-path-provenance-022` off `main@89b1d57`. **B6 implementation complete and
  green**; spec 022 committed standalone (`38cb419`), Task 1 committed (`d699892`), about to commit the
  final feat (Task 2: expr change + ADR-0047 + plan/BACKLOG/HANDOVER — this handover rides in it). After
  commit: whole-branch gate (`/code-review` + `/security-review` over `main..HEAD`) → auto merge+push +
  delete branch (standing authorization).
- Full gate green: `go build ./...`, `go test ./... -race` (all packages ok), `go vet`, `gofmt -l .`
  (empty), `CGO_ENABLED=0 go build`, `go mod tidy`(no-op)/`verify` all clean. `expr` coverage 95.2%,
  `pipe` coverage 99.3%. `benchstat` installed at `$(go env GOPATH)/bin/benchstat`.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch (016's ledger); not used for the 017+ program — track via
  `docs/BACKLOG.md` + git log instead.
- Artifact numbering is continuous: specs/plans at **022 done**, ADRs at **0047 done**. **B7 = spec 023 +
  plan 023 + ADR-0048 next.**
