# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then the SDD
> ledger `.superpowers/sdd/progress.md` (the durable per-task record), and the next-increment specs
> `docs/specs/014-value-serde-consistency.md` and `docs/specs/015-foreach-line-item-stage.md` (plans
> NOT yet written). This file points at them; it does not restate them. **Never commit/push without
> explicit user approval** — per-task commits during an approved plan are the only pre-authorized
> exception (CLAUDE.md).

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first: no cgo, typed
wrapping errors). Executing a deeper business-rule production-readiness audit remediation as five
focused specs (011–015).

- **Increment status:** **011 DONE & merged**, **012 DONE & merged** (both on `origin/main`),
  **013 DONE & gated** (on branch, awaiting merge/push approval). **014/015 NOT started.**
- **Active branch:** `claude/ruleset-identity-013` (off `main@7a60d69`), holding the complete 013
  increment. `main` currently = `7a60d69` (010+011+012 merged).

## Exact state (safepoint)

Tree builds; `go test ./... -race` green (all 5 pkgs); `go vet` / `gofmt -l .` clean;
`CGO_ENABLED=0 go build ./...` OK. `git status --short`: only ` M docs/HANDOVER.md` (this file). Last
commit: `1e9ce5e docs(rlng): ADR-0037 + ruleset identity example and docs`.

**Increment 013 — Ruleset identity & decision stamping: COMPLETE, gated, green.**
- Task 1 `c5d379a` — `pipe.RulesetIdentity{Hash, Version}`, chainable `Pipeline.WithRuleset`, `Run`
  stamps the Scope, `Scope.Ruleset()`.
- Task 2 `ccdccf9` — `(*config.PipelineDef).Hash()` (canonical JSON + SHA-256, version excluded),
  `Version` field, `WithRulesetVersion` BuildOption, `Build` wiring, `MatchesRuleset`.
- Task 3 `dd21745` — Scope JSON round-trips ruleset + firing (fully replayable record); `FiringRule`
  json tags.
- Task 4 `1e9ce5e` — ADR-0037 + runnable example + firing/identity docs.
- All 4 tasks reviewed-Approved (Task 1 had one plan-mandated table-test fold, fixed). **Whole-branch
  gate over `main..HEAD` done:** `/code-review` (high) 0 correctness bugs, 2 Minor triaged to backlog;
  `/security-review` clean. See the ledger for the triaged Minors.

**Specs/plans/ADRs:** Specs 011–015 committed. Plans **011/012/013 done**; **014/015 NOT written.**
ADRs 0031–0037 authored & committed. Spec 013 carries resolved decisions D1–D7.

## Traceability pointers (read first, in order)

1. `CLAUDE.md` (workflow, gates, commit discipline, testing rules).
2. `.superpowers/sdd/progress.md` — the SDD ledger: every task's SHA, the 011/012/013 whole-branch gate
   results, and the triaged Minor findings. **Trust the ledger + `git log` over memory.**
3. `docs/specs/014-value-serde-consistency.md`, `docs/specs/015-foreach-line-item-stage.md` — the
   remaining increments to plan & execute.
4. Background: `docs/specs/010`/`011`/`012`/`013` + ADRs 0022–0037.

## Decisions & deviations this session

- **013 shipped** all of G1–G4 plus the expanded scope the user chose (firing serialization in the
  Scope JSON), realized per spec 013's D1–D7. All additive/backward-compatible — no breaking change.
- **013 code-review Minors (triaged, non-blocking — ledger has rationale):** (1) `FiringRule` json tags
  change wire keys for a consumer who directly marshaled the struct pre-013 (library never emitted it
  before). (2) `MatchesRuleset` re-hashes via `Hash()` each call (replay path, not hot path).
- **User process directive (carry forward):** implementers start from `cc-skills-golang:golang-how-to`
  and honor the custom skills (`table-test`, `use-mockgen`, `use-testcontainers`); run `/code-review` +
  `/security-review` before delivery.

## Pending approvals / open questions

- **Nothing is pushed.** Merge/push of `claude/ruleset-identity-013` needs explicit user approval
  (fast-forward onto `main@7a60d69`; 4 commits `c5d379a..1e9ce5e`). Offer to commit this `HANDOVER.md`
  too, then delete the merged branch (CLAUDE.md).
- **014 decimal approach** (decimal-library dependency vs in-house minor-units type) must be
  **brainstormed with the user before Plan 014 is written** — it touches the minimal-deps and
  pure-Go/no-cgo constraints (deferred to ADR-0039).

## Next actions (resume here)

1. **Present increment 013 for the user's merge/push decision** (do not push without approval). On
   approval: commit `HANDOVER.md`, fast-forward merge to `main`, push, delete the branch.
2. **Then plans 014 → 015** — brainstorm 014's decimal decision with the user first, write each plan
   just-in-time (`superpowers:writing-plans` from the committed spec), execute via
   `superpowers:subagent-driven-development`. Each plan must include a Task that authors its ADRs, and
   ensure `scripts/task-brief` is run for EVERY task (013 Task 4's brief was initially missed).

## Gotchas / environment

- **`govulncheck` and `golangci-lint` are NOT installed locally** — `.github/workflows/ci.yml` runs
  them (plus build/vet/fmt/race across a Go 1.25/1.26 matrix). Let CI go green before any merge.
- **`go.mod` declares `go 1.25`.** Supported floor is Go 1.25+.
- **SDD hand-off files** (`.superpowers/sdd/task-N-brief.md`, `task-N-report.md`, `review-*.diff`) are
  git-ignored scratch reused per task/increment (overwritten each time) — a stale brief from a prior
  plan can linger (013 Task 4 hit this); the ledger `progress.md` is the durable record.
