# HANDOVER — rlng (updated 2026-07-12)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then the
> governing artifacts — the SDD ledger `.superpowers/sdd/progress.md` (the durable per-task record),
> `docs/specs/013-ruleset-identity.md`, `docs/specs/014-value-serde-consistency.md`, and
> `docs/specs/015-foreach-line-item-stage.md` (the next increments, plans NOT yet written). This file
> points at them; it does not restate them. **Never commit/push without explicit user approval** —
> per-task commits during an approved plan are the only pre-authorized exception (CLAUDE.md).

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first: no cgo, typed
wrapping errors). This session is executing a **deeper business-rule production-readiness audit**
remediation as five focused specs (011–015).

- **Active branch:** `claude/business-rule-audit-hw3p61` (carries **spec-010** at `9a366f7`, the
  **011** increment, and the now-complete **012** increment stacked on top).
- **Increment status:** **011 DONE**, **012 DONE** (this session). **013/014/015 NOT started** — plans
  not yet written.

## Exact state (safepoint)

Tree builds; `go test ./... -race` green (all 5 pkgs); `go vet` / `gofmt -l .` clean;
`CGO_ENABLED=0 go build ./...` OK. `git status --short`: only ` M docs/HANDOVER.md` (this file, the
sole uncommitted change — a `docs:` artifact; offer to commit it). Last commit:
`a48cfb4 docs(rlng): ADRs 0034-0036 + firing/fallback/coerce docs`.

**Increment 011 — Config-path safety parity: COMPLETE, gated, green.** Commits `455cf92`→`b676e58`.
ADRs 0031/0032/0033. Whole-branch gate over `9a366f7..b676e58` done (code-review: 1 fixed + 4 Minor
triaged; security clean).

**Increment 012 — Evaluation correctness & explainability: COMPLETE, gated, green.**
- Task 1 `208b0c1` — multi-rule firing model (`map[string][]FiringRule`, `FiringRulesFor`).
- Task 2 `eaf3a68` — collect & any record firing per matched rule; any-conflict records no stray firing.
- Task 3 `2e86621` — `feat(expr)!` fallback fires on error only; nil first-class; `WithFallbackOnNil`,
  `WithFallbackObserver`.
- Task 4 `efd2118` — `fix(expr)!` safe/honest coercion: NaN/±Inf→false, unhandled type→typed EvalError.
- Task 5 `a48cfb4` — ADRs 0034/0035/0036 + runnable firing example + firing/fallback/coerce docs.
- All 5 tasks reviewed-Approved. **Whole-branch gate over `b676e58..a48cfb4` done:** `/code-review`
  (high) found 0 correctness bugs, 2 Minor triaged to backlog (see ledger); `/security-review` clean.

**Specs/plans/ADRs status:** Specs 011–015 committed (`455cf92`). Plans **011 & 012 done**;
**plans 013/014/015 NOT written.** ADRs 0031–0036 authored & committed.

## Traceability pointers (read these first, in order)

1. `CLAUDE.md` (workflow, gates, commit discipline, testing rules).
2. `.superpowers/sdd/progress.md` — the SDD ledger: every task's SHA, the 011 & 012 whole-branch gate
   results, and the triaged/carried Minor findings. **Trust the ledger + `git log` over memory.**
3. `docs/specs/013-ruleset-identity.md`, `docs/specs/014-value-serde-consistency.md`,
   `docs/specs/015-foreach-line-item-stage.md` — the remaining increments to plan & execute.
4. Background: `docs/specs/010`/`011`/`012` + ADRs 0022–0036.

## Decisions & deviations this session

- **012 behavior changes shipped (pre-`v0.1.0`, ADR-bound):** Task 3 — a `nil` main result no longer
  triggers a fallback by default (opt in with `WithFallbackOnNil`); `WithFallbackObserver` surfaces an
  error-triggered fallback's cause (ADR-0034). Task 4 — under `WithCoerce`, `NaN`/`±Inf`→false and an
  unhandled result type → typed `EvalError` (ADR-0035); `truthy` is now `truthy(v any) (bool, error)`.
  Both flagged `!` (breaking) in commit subjects; each updated the pre-existing test asserting the old
  behavior.
- **012 code-review Minors (triaged, non-blocking — ledger has rationale):** (1) a panicking
  `WithFallbackObserver` callback escapes `Apply` — WON'T-FIX (caller code; Go doesn't recover user
  callbacks). (2) collect/any record firing before `writeAgg`/aggregate can error — BACKLOG (cosmetic;
  an errored Run's Scope is discarded).
- **Firing is still NOT serialized** by the Scope JSON codec (`pipe/json.go` has zero firing refs) —
  a real gap for **spec 013**'s replayable decision record. Capture it when writing Plan 013 (persist
  firing + ruleset identity in the JSON round-trip).

## Pending approvals / open questions

- **Nothing is pushed.** Merge/push of `claude/business-rule-audit-hw3p61` needs explicit user
  approval. The branch bundles 010 + 011 + 012; the user may want to split or sequence PRs.
- **014 decimal approach** (decimal-library dependency vs in-house minor-units type) is deferred to
  ADR-0039 and should be **brainstormed with the user before Plan 014 is written** — it touches the
  minimal-deps and pure-Go/no-cgo constraints.

## Next actions (resume here)

1. **Present increments 011 + 012 for the user's merge/push decision** (do not push without approval).
   Offer to commit this `HANDOVER.md` too.
2. **Then plans 013 → 014 → 015** — write each plan just-in-time (`superpowers:writing-plans` from the
   committed spec), execute via `superpowers:subagent-driven-development` (the SDD loop is established;
   scripts under `~/.claude/plugins/cache/claude-plugins-official/superpowers/6.1.1/skills/subagent-driven-development/scripts/`).
   Brainstorm 014's decimal decision with the user first. Each plan must include a Task that authors its
   ADRs (lesson from 011).

## Gotchas / environment

- **`govulncheck` and `golangci-lint` are NOT installed locally** — `.github/workflows/ci.yml` runs
  them (plus build/vet/fmt/race across a Go 1.25/1.26 matrix). Let CI go green before any merge.
- **`go.mod` declares `go 1.25`.** Supported floor is Go 1.25+.
- **Scratchpad** for temp files: `/private/tmp/claude-501/.../scratchpad` (not `/tmp`).
- **SDD hand-off files** (`.superpowers/sdd/task-N-brief.md`, `task-N-report.md`, `review-*.diff`) are
  git-ignored scratch reused per task/increment; the ledger `progress.md` is the durable record.
  `git clean -fdx` would destroy them; recover from `git log`.
