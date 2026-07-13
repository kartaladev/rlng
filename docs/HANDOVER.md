# HANDOVER — rlng (updated 2026-07-14)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then `docs/BACKLOG.md`,
> then `git log` on `main`. Trust the repo (code + git history + `docs/`) over any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). The B1–B12 program (increments 017–028) and the post-audit refactor batch (increment 029)
are complete and on `origin/main`. This session (2026-07-14, user AFK) delivered two increments:

1. **Refactor batch 029** — behavior-preserving quality refactors R1–R9,R11; merged+pushed `origin/main`.
2. **Examples guided tour 030** — the current increment (this handover).

## What's DONE this session

### Increment 029 (refactor batch R1–R9,R11) — merged `origin/main@f225335`
Ten behavior-preserving refactors (numeric kernel `pipe/numeric.go` + ADR-0054, `deriveOrSet`, `p.wide` gate,
prefix-rekey helper, `truthy` fold, decimal table, `copyMap`→`mergeInto`, `knownExprDefFields`/`exprEnvOpts`,
hash memo, `newEngineConfig`/`parseAndBuild`). Whole-branch code-review + security clean, coverage ≥94%,
no exported-API change. `docs/BACKLOG.md` R1–R9,R11 → Resolved (R10 still open); 5 cosmetic "R-cleanup" Minors
deferred there.

### Increment 030 (examples guided tour) — on branch `claude/examples-guided-tour-030`, ready to merge
`examples/` restructured into a **14-file numbered pedagogical tour** (simplest `expr` primitive → typed
engine), 35 runnable `Example` functions across `01_expr_predicates` … `14_engine_typed`, each with teaching
doc comments exposing every capability + its quirks, real-world scenarios (2–3/topic), prettified code, and
verified `// Output:` blocks. `examples/doc.go` is a 14-topic tour index; the README `## Usage` section was
rewritten simplest→complex (11 subsections) cross-referencing the numbered files. Spec 030 / Plan 030; **no
ADR** (docs/test increment). **The only non-`_test.go` change on the whole branch is `examples/doc.go`
(comment-only) — zero library behavior change.** Per-package godoc `*_example_test.go` were left untouched by
design. Executed via SDD (implementer + reviewer per task); whole-branch review = **ready to merge**;
`go test ./... -race` green; gofmt/vet clean. A new **watch item W1** was recorded in `docs/BACKLOG.md` (a
strict-env limitation found while authoring example 11 — not fixed, low urgency).

## Next actions

- **Merge 030 → main, push origin/main, delete branch** (authorized this session) — the final step of this
  handover; once done, `origin/main` carries 029 + 030 for the user's morning review.
- **No open example/doc work.** Remaining backlog: **R10** (additive same-level output-path collision
  detection — needs a design + ADR), the deferred **R-cleanup** cosmetics, and **W1** (strict-env/`decimal()`
  audit). All optional; start any with a fresh `docs/specs/*`→`plans/*`→`adrs/*` chain per CLAUDE.md.
- **Release tags remain gated on explicit user approval** (never auto).

## Exact state

- Branch `claude/examples-guided-tour-030` at HEAD `cd5ab22` (spec + 6 task commits + ErrNilInput follow-up);
  a trailing `docs:` commit (this HANDOVER + BACKLOG W1) precedes the merge. Working tree otherwise clean
  (`.superpowers/sdd/` is gitignored scratch: ledger, briefs, reports, review packages).
- Artifact numbering (continuous): specs/plans at **030**; ADRs at **0054** (030 added no ADR).

## Gotchas / environment

- `govulncheck`/`golangci-lint`/`apidiff` are NOT installed locally — CI runs them on push.
- Examples are `package examples_test` (blackbox); `examples/doc.go` is `package examples` and exports
  nothing. `// Output:` blocks must match real output — `go test ./examples/ -race` is the guard.
- The concurrency read-isolation invariant lives in `pipe/scope.go` + `pipe/pipeline.go` (`p.wide`, now
  suppressed for `WithMaxParallel(1)` after R7); guard `TestPipelineConcurrencyNoSharedNestedMapRace` must
  stay `-race` clean.
