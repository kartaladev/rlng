# HANDOVER — rlng (updated 2026-07-11)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then the
> governing artifacts for the increment you're on. This file points at them; it does not restate
> them. You are being handed an **autonomous multi-increment run** — read the "Autonomy grant"
> and "Per-increment recipe" sections below before doing anything.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first: no cgo,
typed wrapping errors that name field+expression). Delivered as a **5-increment roadmap**
(spec 001 §Roadmap):

| # | Increment | Status |
|---|-----------|--------|
| 1 | Expression core (`expr/`) | ✅ merged |
| 2 | Scope + stages (`stage/`) | ✅ merged + pushed |
| 3 | Stage DAG orchestration (`Pipeline`: depends_on topo-sort + cycle detection) | ✅ **merged + pushed** (this session) |
| 4 | **Declarative config (YAML/JSON loaders)** | ⏭️ **NEXT — start here** |
| 5 | Result mapper + `Engine[I, R]` facade | pending |

Specs 004–005 do not exist yet. Each increment is a full cycle: **brainstorm/design → spec →
plan → implement → whole-branch review → push**. The wrkflw `map`→`map` adapter is intentionally
**out of rlng** (lives in the wrkflw repo) — not a future increment here.

## Exact state (safepoint)

- **Branch:** `main`. **HEAD:** the Increment-3 handover commit (this file), whose parent is `699750c` (`test(stage): drop dead execErr scaffolding from recordStage`). Pushed to `origin/main`.
- `git status --short`: **clean**. Increment 3's branch `feat/dag-orchestration` was merged (fast-forward) and **deleted**.
- **Build/tests green:** `go test ./... -race` passes (`expr`, `stage`); `go build`/`vet`/`gofmt`/`golangci-lint run ./...` clean; `go mod tidy` a no-op. `govulncheck` **not installed** in this environment — run it when available.
- Deps (consumer-visible): only `expr-lang/expr v1.17.8`. `testify` is test-only. Go 1.25+ (toolchain 1.26.x).
- **Delivered this session (Increment 3):** the `stage.Pipeline` orchestrator (`stage/pipeline.go`) — `NewPipeline(stages ...Stage)` validates duplicate names / unknown deps / cycles and computes a deterministic **input-order-preserving topological order** once; `Run(ctx, *Scope)` walks it sequentially, honoring ctx cancellation and stopping at the first stage error. Typed construction errors `DuplicateStageError` / `UnknownDependencyError` / `CycleError` (concrete `a -> b -> a` loop path). Sequential deterministic execution (concurrency deferred). `stage` package coverage 91.4%, `pipeline.go` 100%/func. Spec 003, plan 003, ADR-0005/0006. Also this session: **test-coverage gate** added to CLAUDE.md (≥85% target; hard requirement = every hot-path + typed-error branch has a covering test).

## Autonomy grant (USER DIRECTIVE this session — applies to increments 3→5)

The user directed: **execute increments 3→5 autonomously**, then handed off to a fresh session to begin. For that run:

- **Standing `commit` + `push` authorization** for increments 3–5 (overrides the per-action approval rule *for this run only*). Still gate every push on: green `go test ./... -race`, and **all** `/code-review` + `/security-review` findings resolved or explicitly triaged.
- **Do NOT block on design approval.** The user is AFK and reviews code + docs on **GitHub** post-hoc. During each increment's brainstorming, make defensible default decisions yourself and **record each as an ADR** (`docs/adrs/000N-*`), rather than asking. (The brainstorming skill's user-approval gate is satisfied by this delegation + GitHub review.)
- **Maximize parallelism.** Increments are dependency-ordered (4 needs 3, 5 needs 4) so run them in sequence; *within* each increment, parallelize independent implementation tasks (isolated **git worktrees** — `Agent` with `isolation: "worktree"`, or dispatch concurrent implementers only when they touch disjoint files) and run reviews concurrently.
- **Delivery per increment:** fresh branch off `main` → work → merge (fast-forward, keep linear history like increments 1–2) → `git push origin main` → delete the branch (local; branches here are not pushed).

> ⚠️ This standing authorization is **scoped to increments 3–5 of this roadmap**. If you are resuming much later or the roadmap has changed, re-confirm with the user before relying on it.

## Per-increment recipe (follow for 4, then 5)

1. **Fresh branch** off `main` (e.g. `feat/config-loaders` for inc 4).
2. **Brainstorm/design** (`superpowers:brainstorming`, self-driven — no user questions). Settle the open decisions, record them as **ADRs**.
3. **Write the spec** → `docs/specs/00N-<slug>.md`. Commit it **standalone** (`spec:` type, `Spec: 00N` trailer). Specs precede code.
4. **Write the plan** (`superpowers:writing-plans`) → `docs/plans/00N-<slug>.md`. The plan file rides with the **first `feat` commit** (never a standalone plan commit). Follow the `table-test` skill for all test code (assert-closure form, `ctx` modifier + `t.Context()` for context-sensitive components, canceled-context case).
5. **Implement.** For a **transcription-complete** plan with few, dependency-chained tasks, **inline TDD execution** (write failing test → run red → implement → run green → per-task commit) is acceptable and was used for Increment 3. For larger/independent-task plans use `superpowers:subagent-driven-development` (fresh implementer per task — **haiku** for pure transcription, **sonnet** for judgment/multi-file — **sonnet** task reviewer per task, fix-loop on Critical/Important). Either way: per-task commit (pre-authorized), `-race` green per task, and **the test-coverage gate** (≥85% + every hot-path/typed-error branch tested; enumerate branches in the plan). Record progress in the SDD ledger (below).
6. **Whole-branch gate:** `scripts/review-package $(git merge-base main HEAD) HEAD` → dispatch a final reviewer on **opus**; then run `/security-review`. Resolve/triage **every** finding; re-run `-race`.
7. **Merge → push → delete branch.** Update THIS handover at the increment boundary.

**Known gotcha for the plan's test code:** `expr-lang` does **float** division, so `1/0 → +Inf` (no error). To trigger a genuine eval error in a test, use modulo (`a % b`, `b=0` → runtime "integer divide by zero"). Increment 2's decision-table/single-expr error tests use this.

## What Increment 3 shipped (done — for context, not to redo)

- `stage.Pipeline` in `stage/pipeline.go`: `NewPipeline(stages ...Stage) (*Pipeline, error)` (variadic; empty set valid), `Run(ctx, *Scope) error`. Validation order: duplicate names → unknown deps → cycle. **Input-order-preserving O(n²) topological sort** (`topoSort`); on stall, `findCycle` walks non-emitted deps until a node repeats and returns the concrete loop `["a","b","a"]`. Typed errors `DuplicateStageError`/`UnknownDependencyError`/`CycleError` (ASCII `->` in messages). `Run` checks `ctx.Err()` before each stage (returns it unwrapped), stops at first stage error without re-wrapping (built-in stages self-name via `*StageError`). Sequential deterministic (ADR-0006); concurrency is an additive future change.
- **Decisions:** ADR-0005 (placement in `stage`, name `Pipeline`, variadic ctor, distinct typed errors, input-order Kahn), ADR-0006 (sequential execution; supersede when concurrency is added).

## Provisional design leads for Increment 4 — Declarative config (refine during brainstorming — not binding)

Goal: load stage/pipeline **definitions from YAML/JSON** and build `Stage` values (and a `Pipeline`) from them. Mirror the `pkg/calc` blueprint (CLAUDE.md §Architecture blueprint), adapted to rules:
- **`ConfigLoader[T]`** abstraction with `StaticConfigLoader`, `ParseYamlConfigLoader`, `ParseJsonConfigLoader`, `FilesystemConfigLoader` (content-type sniffed via `gabriel-vasile/mimetype`). Definitions are declarative YAML/JSON with custom `UnmarshalYAML`/`UnmarshalJSON` supporting a **scalar shorthand** (bare string = the expression) or full object form.
- **Package:** likely a new `config` package (keep `stage` free of yaml/json deps) — decide + ADR. Definitions map onto the existing `stage` constructors (`NewSingleExpr`/`NewMultiExpr`/`NewDecisionTable`) + `NewPipeline`.
- **Validation** via `go-playground/validator` on decoded structs; keep typed, field-naming errors (debuggability). Reuse the increment-1/2 `expr.Option`/`stage.Option` plumbing.
- **New consumer-visible deps** (justify each; keep set minimal): `gopkg.in/yaml.v3`, `gabriel-vasile/mimetype`, `go-playground/validator/v10`, possibly `go-viper/mapstructure/v2`. This is the increment that grows the dependency set — weigh each against the "minimal deps" gate.
- **VariablePatcher** (config-declared defaults injected at compile time via `x ?? <literal>`) may land here or in inc 5 — decide during brainstorming.

## Carried backlog (triage before the first `v0.0.x` release tag; not merge-blockers)

From increment-2 and increment-3 reviews (also in the SDD ledger):
- **Release-gate:** `SingleExpr/MultiExpr/DecisionTable` `Name()/Type()/DependsOn()` and the new `DuplicateStageError/UnknownDependencyError/CycleError` `.Error()` methods lack godoc — the "every exported symbol documented" gate is release-bound. Add before tagging. (Consistent with existing `StageError.Error()`; golangci-lint is clean.)
- `Scope.Set` doesn't validate empty path **segments** (`"a.."`, `".a"` create a `""` key) — inert but worth a guard/doc.
- `StageError.Error()` (and `expr.CompileError/EvalError`) deref `Cause` unconditionally → panic on a hand-built nil-`Cause` literal. One-line nil guard across all three restores parity.
- Missing tests: `MultiExpr` stable-tie ordering; `DecisionTable` empty-output-key rejection (a live untested branch).
- `DecisionTable.executeSingle`/`executeCollect` duplicate iteration scaffolding — `/simplify` candidate.
- Stages are **not transactional** (single/multi persist each write incrementally; collect is atomic) — document the semantic.
- Run `govulncheck ./...` once it's installed.

## Environment / gotchas

- Repo `/Users/zakyalvan/Documents/RND/rlng`; macOS/zsh; Go 1.26.x; module `github.com/kartaladev/rlng`; remote `git@github.com:kartaladev/rlng.git`.
- **SDD ledger:** `.superpowers/sdd/progress.md` (git-**ignored** scratch — the durable recovery map for subagent-driven runs, but it does **not** survive a fresh clone; this handover is the authoritative, committed handoff). The autonomy grant + increment-2 review history are recorded there.
- SDD helper scripts: `superpowers/.../skills/subagent-driven-development/scripts/{task-brief,review-package}` (paths in the ledger).
- `.superpowers/` and `.idea/` are git-ignored.
- Reference blueprint (adapt, don't copy): `bbn-crm-backend` `pkg/calc`.

## Next action

Start **Increment 4 (Declarative config — YAML/JSON loaders)** from a fresh branch off `main` (e.g. `feat/config-loaders`), following the per-increment recipe above. Begin with `superpowers:brainstorming` (self-driven) to settle the loader design (see "Provisional design leads for Increment 4"), write ADR(s) + `docs/specs/004-*`, commit the spec standalone, then plan (`docs/plans/004-*`), then implement (TDD + coverage gate), then whole-branch `/code-review` + `/security-review`, then merge → push → delete branch.

The autonomy grant (commit + push for increments 3–5) is **still in force for Increments 4 and 5**.
