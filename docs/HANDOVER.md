# HANDOVER — rlng (updated 2026-07-11)

> **To the next session:** Read this, then **read before acting** and trust over memory:
> `CLAUDE.md`, `docs/specs/002-scope-and-stages.md`, `docs/plans/002-scope-and-stages.md`,
> `docs/adrs/0002-stage-execution-model.md`, `docs/adrs/0003-decision-table-hit-policies.md`.
> This points at those artifacts; it does not restate them.

## Status: Increment 2 (Scope + stages) CODE-COMPLETE on branch `feat/scope-and-stages`.

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first: no cgo,
typed errors). Increment 1 (`expr/`) shipped and is merged to `main`. Increment 2 adds the
`stage` package: the `Scope` accumulator and three stage types that compose the `expr`
evaluators into reusable rule/calculation units — `Scope` (`stage/scope.go`), `SingleExpr`
(`stage/single.go`), `MultiExpr` (`stage/multi.go`), `DecisionTable` (`stage/table.go`), plus
`ADR-0002` (stage execution model + the `Context`→`Scope` naming decision) and `ADR-0003`
(decision-table hit policies, `ModeSingle`/`ModeCollect`).

**Not yet done:** the whole-branch pre-merge review gate (`/code-review` + `/security-review`
over `main..HEAD`) and user merge approval. Per CLAUDE.md, do not merge/push until both are
clean and the user has explicitly approved. **This branch is NOT merged to `main` yet** —
confirm with `git log --oneline main..feat/scope-and-stages`.

## Exact state
- Branch: `feat/scope-and-stages`, all 6 plan tasks committed (Task 1–5 stage code, Task 6
  package doc + docs refresh, this commit). Working tree clean after Task 6's commit — confirm
  with `git status --short` and `git log --oneline -10`.
- No new runtime dependency: `stage` imports only in-repo `expr` + stdlib; `go mod tidy` is a
  no-op. `testify` remains test-only.
- Quality gates run in Task 6: `go test ./... -race`, `CGO_ENABLED=0 go build ./...`,
  `go vet ./...`, `gofmt -l .`, `go mod tidy` — see the Task 6 report
  (`.superpowers/sdd/task-6-report.md`) for exact output and whether `golangci-lint`/
  `govulncheck` were available in this environment.

## Next: whole-branch review gate, then Increment 3
1. Run `/code-review` and `/security-review` over `main..HEAD` (the whole `feat/scope-and-stages`
   diff), resolve or explicitly triage every finding, re-run `go test ./... -race` green.
2. Get explicit user approval, then merge to `main` (never push/merge without approval), and
   delete the feature branch per CLAUDE.md's mandatory post-merge cleanup.
3. **Increment 3 — Stage DAG orchestration:** topologically sort stages by `DependsOn()`
   (Kahn's algorithm) with cycle detection, per spec 001's roadmap. Start with
   `superpowers:brainstorming` → `docs/specs/003-*` → `superpowers:writing-plans` →
   `docs/plans/003-*` → subagent-driven TDD, recording any new ADRs as they're made.

## Gotchas
- Repo `/Users/zakyalvan/Documents/RND/rlng`; macOS/zsh; Go 1.26.4.
- SDD scratch (`.superpowers/`) and `.idea/` are git-ignored.
- Reference blueprint: `bbn-crm-backend` `pkg/calc` (a calculation engine — adapt, don't copy).
- The blueprint's "Context" accumulator is implemented as `Scope` (package `stage`) starting
  increment 2 — see ADR-0002 and the one-line clarification added to CLAUDE.md's Architecture
  blueprint section.
