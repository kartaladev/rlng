# Plan 010 — Business-rule production hardening

- **Implements:** Spec `docs/specs/010-business-rule-hardening.md`.
- **ADRs:** 0022 (panic safety), 0023 (strict env), 0024 (host functions + now),
  0025 (hit policies + default + aggregation), 0026 (rule identity), 0027 (per-stage
  timing), 0028 (config surface), 0029 (lint), 0030 (decimal/foreach deferred).
- **Discipline:** strict TDD per symbol (visible red → green), black-box tests,
  assert-closure tables; per-package coverage ≥ 85% with every hot-path/typed-error
  branch covered; `go test ./... -race` green before each commit.

## Task graph (each a green, committed unit)

1. **Release blockers** — `go.mod` → `go 1.25`; add `.github/workflows/ci.yml`
   (build/vet/gofmt/race/lint/govulncheck, Go 1.25/1.26 matrix). *(chore, no TDD.)*
2. **Spec 010** — authored standalone (`spec` commit). *(this + the plan file.)*
3. **expr: host functions + panic safety** — `WithFunction`; regression test locking
   that an eval panic surfaces as `EvalError` (VM already recovers). ADR-0022/0024.
4. **expr: strict typed env** — `WithEnv(schema)` compiles against a declared env
   without undefined-variable tolerance; merge globals/locals/functions. ADR-0023.
5. **pipe: decision-table semantics** — `WithDefault`, `HitPolicyUnique`/`HitPolicyAny`,
   `WithCollectAggregation` (list/sum/min/max/count); typed `ErrMultipleMatches`,
   `ErrConflictingMatches`, `ErrNonNumericAggregate`. ADR-0025.
6. **pipe: rule identity** — `Rule.ID`/`Message`; `Scope.FiringRule(s)` recording the
   firing rule for single/unique/any and the default. ADR-0026.
7. **pipe: per-stage timing** — `Pipeline.Run` times each stage via the injected clock;
   `Scope.StageDuration`/`StageTimings`. ADR-0027.
8. **config: surface** — `hit_policy` unique/any, `aggregation`, `default`, rule
   `id`/`message`; pipeline `constants` (globals) and `mapping` block. ADR-0028.
9. **config: lint** — `(*PipelineDef).Lint()` → unreachable-rule / missing-default. ADR-0029.
10. **pipe: `NowFunc`** — clock-backed `now()` host function. ADR-0024.
11. **docs** — runnable examples, README capabilities, ADR-0030 (defer decimal/foreach).
12. **Whole-branch gate** — `/code-review` + `/security-review` over `main..HEAD`;
    resolve/triage findings; re-run `-race`.

## Hot-path branches to cover (fold into `table-test` tables)

- Strict env: reject unknown ident / accept declared / merge globals+functions / lenient default.
- Panic: panicking function & predicate → `EvalError`.
- Hit policies: single first-match; unique 0/1/≥2 matches; any agree/conflict; collect
  list + each aggregation; default only on no-match; empty-key & compile errors; numeric
  vs non-numeric aggregate; int/uint/float folds; out-of-range aggregation must not panic.
- Firing rule: recorded (single/unique/any) / default flagged / absent on no-match; all-stages.
- Timing: per-stage recorded in order; missing stage → not found.
- Config: each new field parsed + built; unknown hit-policy/aggregation error; constants
  reach conditions+decisions+default; constants do not clobber stage globals; mapping parsed.
- Lint: unreachable detected; missing-default detected; exhaustive/collect/non-table → clean.

## Verification checklist

- [x] Each task: red observed, green, per-package coverage checked, committed.
- [x] `go build ./...`, `go vet ./...`, `gofmt -l .` clean.
- [x] `go test ./... -race` green (expr 94.1% · pipe 99.8% · config 100% · root 100%).
- [x] `/code-review` (high) over `main..HEAD` — 2 bugs fixed (globals-merge clobber;
      aggregation index panic), regression-tested.
- [x] `/security-review` — clean for the trusted-ruleset scope.
- [ ] `golangci-lint` / `govulncheck` — run in CI (not installed locally).
