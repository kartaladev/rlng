# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then `docs/BACKLOG.md`
> (now fully resolved), then `git log` on `main`. Trust the repo (code + git history + `docs/`) over any
> recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Increments 011–016 merged & pushed.

**✅ The B1–B12 backlog-execution program is COMPLETE.** All 12 tracked backlog items from
`docs/BACKLOG.md` have been executed as their own increments (or closed as a documented non-goal), each
merged & pushed to `main`:

- **B1** incr 017 `f10a8be` ADR-0042 · **B2** incr 018 `934f1b5` ADR-0043 · **B3** incr 019 `2af21f2` ADR-0044
- **B4** incr 020 `4b3089e` ADR-0045 · **B5** incr 021 `89b1d57` ADR-0046 · **B6** incr 022 `56b243d` ADR-0047
- **B7** incr 023 `9569ecc` ADR-0048 · **B8** incr 024 `e6ed96e` ADR-0049 · **B9** incr 025 `1a50653` ADR-0050
- **B10** incr 026 `b05ce72` ADR-0051 (constructors; Pipeline-as-Stage re-deferred) · **B11** incr 027
  `007ea7a` ADR-0052 (parallel stage execution + delivery-gate race fix) · **B12** incr 028 ADR-0053
  (closed as non-goal; env half already shipped by ADR-0031)

## What's DONE this session
- **B11 / increment 027 (`main@007ea7a`, merged+pushed):** opt-in parallel execution of independent DAG
  stages (`pipe.WithConcurrency()` / `WithMaxParallel(n)` + config + convenience-constructor options).
  Wave-based level-barrier scheduling; deterministic on the success path. **A data race the original design
  missed was caught by the whole-branch `/code-review` and fixed before merge** — stages evaluated against
  `sc.Snapshot()`'s live nested references while an independent same-level stage mutated a shared nested map
  in place (production `fatal: concurrent map read and map write`). Fixed with per-stage read isolation:
  under real parallelism (`p.wide`), `Snapshot` deep-copies the map spine. ADR-0052 §5 amended; regression
  guard `TestPipelineConcurrencyNoSharedNestedMapRace`.
- **B12 / increment 028 (docs-only closure):** feasibility-brainstormed and closed. The strict-typed-env
  half of B12 was **already delivered** by ADR-0031 (the YAML `schema` block). Host functions in YAML are
  **closed as a deliberate non-goal**: arbitrary Go can't be serialized to YAML without a plugin/interpreter
  that breaks pure-Go debuggability + the minimal-safe-surface constraint; the two feasible variants
  (expression-bodied functions; host-registered allowlist) are YAGNI/marginal, recorded in ADR-0053 as
  considered-and-rejected re-entry points. No runtime code. Spec 028, ADR-0053 (supersedes the "Deferred
  within config" bullet of ADR-0028, which is annotated in place); BACKLOG B12 → Resolved, program banner
  added.

## Next actions
- **No open backlog items.** The program is complete. New work should start with a **fresh backlog sweep**
  (re-audit the code + docs for deferrals/watch-items) and, if anything surfaces, a new `docs/specs/*` →
  `docs/plans/*` → `docs/adrs/*` chain per CLAUDE.md.
- **Optional, still gated on explicit user approval:** cut a release tag. Nothing has been tagged; per
  CLAUDE.md the standing auto-merge authorization does **not** extend to release tags — ask first. Consider
  whether the accumulated pre-1.0 breaking changes (e.g. `NewPipeline` signature, `WithRuleset` option,
  decision-table `map[string]pipe.Decision`, nested-foreach firing keys) warrant a version-planning ADR
  before a `v0.x` tag.

## Exact state
- **On branch `claude/close-b12-nongoal-028`** at handover time, about to merge B12 (docs-only) to `main`.
  Once merged: `main` carries all of B1–B12.
- Docs-only increment: `go build ./...` + `go test ./... -race` unchanged and green (no code touched);
  `/code-review` + `/security-review` trivially clean (no code delta).
- Standing program cadence (AUTO merge+push per green increment) applies; release tags still need explicit
  approval.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch; track program history via `docs/BACKLOG.md` + git log.
- Artifact numbering (continuous): specs/plans at **028** (028 is a spec-only closure, no plan by design —
  see the spec's deviation note); ADRs at **0053** done. Next work continues from 029 / 0054.
- The concurrency read-isolation invariant lives in `pipe/scope.go` (`Snapshot` deep-copy when `concurrent`)
  + `pipe/pipeline.go` (`p.wide` gate + `markConcurrent`); regression guard
  `TestPipelineConcurrencyNoSharedNestedMapRace` must stay `-race` clean.
