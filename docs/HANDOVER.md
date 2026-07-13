# HANDOVER — rlng (updated 2026-07-14)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then `docs/BACKLOG.md`,
> then `git log` on `main`. For the autonomous program in flight, also read `.superpowers/sdd/progress.md`
> (the SDD ledger) and the project memory `rlng-refactor-and-examples-030`. Trust the repo (code + git
> history + `docs/`) over any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). The B1–B12 backlog program is complete (increments 017–028). The whole-codebase audit
(increment 029 label, merged `main@a333841`) fixed 8 bugs and recorded the **post-audit refactor backlog
R1–R11** in `docs/BACKLOG.md`.

**Active autonomous session (user AFK, reviews `origin/main` in the morning).** Explicit user authorization
this session for **merge to main + push to origin/main + branch delete** (NOT release tags).

## What's DONE this session — refactor batch 029 (R1–R9, R11)

Ten behavior-preserving, quality-only refactors, executed via subagent-driven development (fresh implementer
+ per-task spec/quality review each), on branch `claude/refactor-batch-029`:

- **R1** `fb5bd0a` (ADR-0054) unify numeric coercion onto one overflow-checked kernel `pipe/numeric.go` ·
  **R2** `6e4c516` `Scope.deriveOrSet` · **R7** `31c0e43` gate `p.wide` on `maxParallel!=1` ·
  **R8** `cc95f9f` shared `prefixKey`/`mergePrefixed` · **R3** `6842a55` fold `truthy` heads ·
  **R4** `c365f08` table-drive decimal ops · **R9** `4e077bc` collapse `copyMap`→`mergeInto` ·
  **R6** `e36ffd6` `knownExprDefFields` var + `exprEnvOpts` · **R11** `acf9624` memoize Build hash ·
  **R5** `908eeab` `newEngineConfig`+`parseAndBuild`. Spec/plan `docs/{specs,plans}/029-*`.

**Whole-branch gate PASSED:** `go build/vet ./...` clean, `gofmt -l .` empty, `go test ./... -race` green
(all 5 packages); coverage rlng 97.6 / config 99.6 / expr 94.9 / pipe 99.2%; whole-branch `/code-review`
(opus) = ready-to-merge (no Critical/Important); `/security-review` = clean; no exported-API change. Five
cosmetic/doc Minors deferred to `docs/BACKLOG.md` ("R-cleanup"). **R10 remains open** (additive feature).

## Next actions (this autonomous session, in order)

1. **Merge 029 → main, push origin/main, delete branch** (authorized this session).
2. **Follow-on increment — examples restructure** (own branch off main): renumber/reorder `examples/*_test.go`
   and per-package `*_example_test.go` from **simplest → most complex, starting with the `expr` package**.
   Expose ALL library capabilities; each component's Example must include its **variances/quirks**.
   **Pedagogical**: elaborate doc comments that teach each feature in detail. **Real-world** scenarios
   (deliberately designed, not toy), **2–3 samples per topic**, with **prettified, readable** sample code.
   Runnable `Example…` with `// Output:`.
3. **README.md `Usage` section**: explain the examples — not too verbose, gradually increasing usage
   complexity, cross-referencing related examples.
4. Run the same delivery gate on the follow-on, then merge + push to origin/main.

## Exact state

- Branch `claude/refactor-batch-029`, at HEAD `908eeab` (all R-tasks committed + gate passed); a trailing
  `docs:` commit for BACKLOG/HANDOVER precedes the merge. Working tree otherwise clean (SDD scratch under
  `.superpowers/sdd/` is gitignored).
- Artifact numbering (continuous): specs/plans at **029**; ADRs at **0054**. The examples/README follow-on
  is a docs/test increment — author a spec/plan/ADR only if a real decision arises.

## Gotchas / environment

- `govulncheck`/`golangci-lint`/`apidiff`/`gorelease` are NOT installed locally — CI runs them on push; the
  no-exported-API-change claim was verified by whole-branch review (all new symbols unexported), not apidiff.
- `.superpowers/sdd/*` are gitignored scratch (SDD ledger, briefs, reports, review packages).
- The concurrency read-isolation invariant lives in `pipe/scope.go` (`Snapshot` deep-copy when `concurrent`)
  + `pipe/pipeline.go` (`p.wide` gate — R7 now suppresses it for `maxParallel==1`); regression guard
  `TestPipelineConcurrencyNoSharedNestedMapRace` must stay `-race` clean.
