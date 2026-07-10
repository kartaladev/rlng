# HANDOVER — rlng (updated 2026-07-10)

> **To the next session:** Read this, then **read before acting** and trust over memory:
> `CLAUDE.md`, `docs/specs/001-expression-core.md`, `docs/plans/001-expression-core.md`,
> `docs/adrs/0001-module-path-and-naming.md`. This points at those artifacts; it does not restate them.

## Status: Increment 1 (Expression core) COMPLETE and merged to `main`.

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first: no cgo,
typed errors). Two tracks on a shared core — rule (decision tables; backs `../wrkflw`'s
`BusinessRuleTask` via its `action.Action` `Do(ctx, map) (map, error)` port) and calculation
(staged pipelines). Delivered as a 5-increment roadmap (see spec 001 §Roadmap).

**Increment 1 shipped** the `expr/` package: `Predicate` (strict / `WithCoerce`), `Function`
(fallback, `WithReturnKind`), the `??` variable-default patcher, typed `CompileError`/`EvalError`,
struct/map `toEnv`, and shared functional options. Fully reviewed (per-task + Opus whole-branch +
`/code-review` + `/security-review`, all findings fixed), `-race`/`vet`/`gofmt`/`golangci-lint`
clean. One consumer-visible dep (`expr-lang/expr`); testify is test-only.

## Exact state
- Merged to `main` and pushed to `origin/main`. Confirm: `git log --oneline -20 origin/main`.
- Deps: `go.mod` has `expr-lang/expr v1.17.8` + `testify` (test-only). Go 1.25+ (`go 1.26.4`).
- `govulncheck` was not installed in the increment-1 environment — re-run it if available.

## Next: Increment 2 — Context + stages
Per `docs/plans` roadmap and spec 001 §Roadmap: build the `Context` accumulator (dot-path
get/set) and the three stage types (`single-expr`, `multi-expr`, `decision-table` with
single/collect hit policy), composing the `expr` evaluators. This is where a usable decision
table (and a thin `map`→`map` adapter for wrkflw's `BusinessRuleTask`) first appears.

**How to start:** follow CLAUDE.md's workflow — `superpowers:brainstorming` → spec
(`docs/specs/002-*`) → `superpowers:writing-plans` (`docs/plans/002-*`) → subagent-driven TDD.
Record any architectural decisions as new `docs/adrs/000N-*`. Keep the table-test/testify
convention and the hard rules (never commit/push without explicit approval; fix review findings
before delivery; hand over from a safepoint at ~60% context).

## Increment 1 backlog (deferred, non-blocking — address if touched)
- Method-bearing data structs *with* exported fields still lose their methods when flattened by
  `toEnv` (only no-exported-field value structs like `time.Time` are now passed through). Revisit
  if consumers need methods on field-bearing structs.
- `WithReturnKind` only applies to the main Function program, not the fallback (by design).
- Coerce truthiness on native typed collections works; broaden tests if the surface grows.

## Gotchas
- Repo `/Users/zakyalvan/Documents/RND/rlng`; macOS/zsh; Go 1.26.4.
- SDD scratch (`.superpowers/`) and `.idea/` are git-ignored.
- Reference blueprint: `bbn-crm-backend` `pkg/calc` (a calculation engine — adapt, don't copy).
