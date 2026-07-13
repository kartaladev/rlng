# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then `docs/BACKLOG.md`
> (the tracked backlog B1–B12), then `git log` on `main`. For the item you pick up, read its
> `docs/specs/*` + `docs/plans/*` + related ADRs first. Trust the repo (code + git history + `docs/`) over
> any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Increments 011–016 merged & pushed.

**Active program: execute ALL 12 backlog items (B1–B12) from `docs/BACKLOG.md`,** each as its own
increment. **B1 (incr 017, `f10a8be`); B2 (incr 018, `934f1b5`); B3 (incr 019, `2af21f2`); B4 (incr 020,
`4b3089e`); B5 (incr 021, `89b1d57`); B6 (incr 022, `56b243d`); B7 (incr 023, `9569ecc`); B8 (incr 024,
`e6ed96e`); B9 (incr 025, `1a50653`); B10 (incr 026, `b05ce72`); B11 (incr 027, on branch
`claude/parallel-stage-execution-027`, awaiting merge).** B1–B10 complete & pushed to `main`.
**B11 is complete on its branch and about to merge; B12 is the last remaining item.**

## Standing decisions for this program (do NOT re-ask)
- **Scope = all 12**, including the two deliberate non-goals **B11** (parallel exec; done) and **B12**
  (YAML env/host functions; superseding ADR for ADR-0028 first).
- **Cadence = AUTO merge+push each green increment** (standing authorization). After each increment's full
  gate passes, merge to `main`, push, delete branch, start the next — no per-increment merge/push approval.
  Does NOT extend to release tags (still need explicit approval).
- **Design-gating = checkpoint only the risky ones.** Design + implement **B2, B3, B4, B10** autonomously.
  **PAUSE for the user's design approval** before implementing **B5, B6, B7, B8, B9, B11, B12**. Code-review +
  security-review gates run on every increment regardless.
- Per-increment workflow: brainstorm → spec `docs/specs/NNN` → plan `docs/plans/NNN` + ADR `docs/adrs/NNNN`
  → TDD (red/green) → `/code-review high main..HEAD` + `/security-review` → resolve findings → merge+push.
  (See memory `rlng-backlog-execution-program` for the full authorization record.)

## What's DONE this session
- **B11 / increment 027 (branch `claude/parallel-stage-execution-027`, awaiting merge):** opt-in parallel
  execution of independent DAG stages via `pipe.WithConcurrency()` / `pipe.WithMaxParallel(n)` (+ config-path
  `config.WithConcurrency`/`WithMaxParallel` BuildOptions and convenience-constructor `rlng.WithConcurrency`/
  `WithMaxParallel` Options). Wave-based (level-barrier) scheduling; deterministic on the success path
  (final `Scope`, surfaced error via topo-min selection, and reported stage order identical to sequential).
  `NewPipeline` consolidated onto `(stages []Stage, opts ...PipelineOption)`; `WithRuleset` moved from a
  fluent method to an option (pre-1.0 breaking). ADR-0052 supersedes ADR-0006. Spec/plan 027.
- **Delivery-gate race fix (this session, folded into B11 before merge):** the whole-branch `/code-review`
  found — and a `-race` regression test (`TestPipelineConcurrencyNoSharedNestedMapRace`) confirmed — a
  **data race** the original B11 missed: a stage evaluates against `sc.Snapshot()`, whose nested maps are
  **live references**; an independent same-level stage whose output path descends into a pre-existing nested
  map (`Set("customer.score", …)` via `WithOutput`) mutates it in place, racing the reader's `expr` VM
  (production `fatal error: concurrent map read and map write`). **Fix:** when a level actually runs >1 stage,
  `Pipeline.Run` marks the `Scope` concurrent and `Snapshot` deep-copies the map spine (reusing `cloneValue`)
  for per-stage read isolation; slices (and maps nested in them) stay shared, since `setPath` can never reach
  into a slice element. Sequential path and linear-chain-configured-for-concurrency unchanged (`p.wide`
  gate). ADR-0052 §5 **amended in place** (its "no Scope change" claim was wrong) + Correction note; spec 027
  "Concurrency safety (per-stage read isolation)" and plan 027 corrected. Review findings #2 (determinism
  needs declared `DependsOn`), #3 (post-error `Scope` not contractual), #4 (foreach inner not parallelized)
  handled as doc caveats in ADR-0052 + `WithConcurrency` godoc; #5 (duplicated concurrency-mode enum in
  `engine.go`) refactored to a captured `[]config.BuildOption`. Re-run `/code-review` + `/security-review`:
  both clean.

## Next action — B12 (increment 028): design-checkpoint — PAUSE for approval
**B12 — Strict env / host functions declarable in YAML** (`docs/BACKLOG.md`; ADR-0028; new superseding ADR;
**P4**; flagged a *likely permanent non-goal*). `WithEnv` (typed env schema) and `WithFunction` (host
functions) are intentionally programmatic — an env schema needs Go types, functions are Go values. Per the
standing design-gating decision this is a **live approval-pause** item and a non-goal reversal needing a
**superseding ADR to ADR-0028** first. The honest brainstorming outcome may be to **formally close B12 as a
documented non-goal via the superseding ADR rather than implement it** — which would complete the whole
B1–B12 program. Read `config/*` (env/function wiring), ADR-0028, and the B12 backlog entry first; brainstorm,
author the spec + draft ADR, then **PAUSE for the user's decision** before writing any code.

Start (after B11 merges): `git checkout main && git pull` (confirm B11 merged), then
`git checkout -b <branch> main`, brainstorm, author `docs/specs/028-*` + a draft superseding ADR to ADR-0028
(`docs/adrs/0053-*`), then PAUSE.

## Exact state
- **On branch `claude/parallel-stage-execution-027`.** B11 implementation (7 commits) + the uncommitted
  delivery-gate race fix are green. The fix is **staged/uncommitted** at handover time — see `git status`.
- Full gate green: `go build ./...`, `go vet`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build`,
  `go test ./... -race` (all 5 packages ok), `go mod tidy`(no-op)/`verify`, coverage `pipe` 99.2% / root
  98.2%. `/code-review high main..HEAD` + `/security-review` both clean after the fix.
- **To land B11:** commit the fix (`fix(pipe): isolate per-stage eval env under concurrency (B11)`), then
  per the standing AUTO merge+push authorization: merge to `main`, push, delete the branch, then proceed to
  B12.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch; not used for the 017+ program — track via `docs/BACKLOG.md` +
  git log instead.
- Artifact numbering is continuous: specs/plans at **027 done**, ADRs at **0052 done**. **B12 needs a new
  spec/plan + a superseding ADR to ADR-0028 — and a design-approval pause before implementation.**
- The concurrency read-isolation invariant lives in `pipe/scope.go` (`Snapshot` deep-copy when `concurrent`)
  and `pipe/pipeline.go` (`p.wide` gate + `markConcurrent`). Regression guard:
  `TestPipelineConcurrencyNoSharedNestedMapRace` — must stay `-race` clean.
