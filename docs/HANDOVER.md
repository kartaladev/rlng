# HANDOVER — rlng (updated 2026-07-12)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then the
> governing artifacts — `docs/specs/010-business-rule-hardening.md`, ADRs `0022`–`0030`, and the
> earlier specs/ADRs they build on. This file points at them; it does not restate them.
> **Never commit/push without explicit user approval** (the increments-3–5 autonomy grant is long spent).

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first: no cgo,
typed wrapping errors). The 5-increment roadmap was complete before this session. **This session
executed a business-rule-system audit and its remediation** (Spec 010) on branch
`claude/business-rule-audit-hw3p61`.

## Exact state (safepoint)

- **Branch:** `claude/business-rule-audit-hw3p61` (off `main`). Tree builds; `go test ./... -race`
  green; `go vet`/`gofmt` clean. Coverage: expr 94.1%, pipe 99.8%, config 100%, root 100% (total 98.7%).
- `git status --short`: clean (all work committed).
- Deps unchanged (`expr-lang/expr`, `yaml.v3`, `mapstructure/v2`; testify test-only). `go.mod` now
  declares `go 1.25` (was `go 1.26.4`).
- **govulncheck:** not installed in this environment — the new `.github/workflows/ci.yml` runs it
  (plus build/vet/fmt/race/lint) across a Go 1.25/1.26 matrix.

## What this session shipped (Spec 010, audit remediation)

Commits (in order) on the branch:
- `chore(build)`: go 1.25 + `ci.yml`.
- `spec`: Spec 010 (standalone).
- `feat(expr)`: host functions (`WithFunction`) + eval panic-safety guarantee (ADR-0022, 0024).
- `feat(expr)`: strict typed env (`WithEnv`) — typos/type errors fail at compile (ADR-0023).
- `feat(pipe)`: decision-table default (`WithDefault`), `HitPolicyUnique`/`HitPolicyAny`,
  collect aggregation (`WithCollectAggregation`: list/sum/min/max/count) (ADR-0025).
- `feat(pipe)`: rule `ID`/`Message` + firing-rule provenance (`Scope.FiringRule(s)`) (ADR-0026).
- `feat(pipe)`: per-stage timing (`Scope.StageDuration`/`StageTimings`) (ADR-0027).
- `feat(config)`: schema for the above + pipeline `constants` + output `mapping` block (ADR-0028).
- `feat(config)`: static `Lint` (`(*PipelineDef).Lint`) — unreachable-rule / missing-default (ADR-0029).
- `feat(pipe)`: clock-backed `NowFunc` for deterministic `now()` (ADR-0024).
- `docs`: runnable examples (`examples/lending_test.go`, `strict_typing_test.go`), README, ADR-0030
  (defer decimal + foreach stage, with interim guidance).
- `fix(rlng)`: **code-review fixes** — `expr.WithGlobals/WithLocals` now MERGE (was overwrite: pipeline
  constants silently clobbered stage globals); collect provenance out-of-range aggregation no longer
  panics (bounds-safe `aggLabel`). Regression-tested.

Gates run: `/code-review` (high) over `main..HEAD` — 2 real bugs found and fixed (above);
`/security-review` — clean for the trusted-ruleset scope (no new untrusted-input/exec/panic paths).

## Deferred (recorded as ADR-0030, not done)

- **Exact decimal money** (float64 today; interim: integer minor units + integer-preserving aggregation).
- **`foreach` stage** (per-line-item iteration; interim: expr `map`/`filter`/`reduce`).

Both warrant their own spec before implementation.

## Next actions

1. Push `claude/business-rule-audit-hw3p61` and open a PR if desired.
2. Let CI run (build/vet/fmt/race/lint/govulncheck, Go matrix); address any lint/vuln findings surfaced
   there (golangci-lint was not run locally — not installed).
3. Future work: decimal + foreach specs (ADR-0030); optionally a strict-env/`now()` config surface if a
   real need appears (currently programmatic by design, ADR-0028).

## Gotchas / environment

- Module `github.com/kartaladev/rlng`.
- `golangci-lint` / `govulncheck` not installed locally — CI covers them.
- expr does **float** division (`1/0 → +Inf`); use `a % b` (b=0) to force a genuine eval error in tests.
- expr's VM recovers evaluation panics (host functions, env methods) into errors — see ADR-0022.
