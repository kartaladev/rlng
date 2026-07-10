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
| 3 | Stage DAG orchestration (`Pipeline`: depends_on topo-sort + cycle detection) | ✅ merged + pushed |
| 4 | Declarative config (`config/`: YAML/JSON loaders) | ✅ **merged + pushed** (this session) |
| 5 | **Result mapper + `Engine[I, R]` facade** | ⏭️ **NEXT — start here** |

Spec 005 does not exist yet. Each increment is a full cycle: **brainstorm/design → spec →
plan → implement → whole-branch review → push**. The wrkflw `map`→`map` adapter is intentionally
**out of rlng** (lives in the wrkflw repo) — not a future increment here.

## Exact state (safepoint)

- **Branch:** `main`. **HEAD:** the Increment-4 handover commit (this file), whose parent is `b6fe25c` (`docs(config): note YAML/JSON scalar-shorthand asymmetry on ExprDef`). Pushed to `origin/main`.
- `git status --short`: **clean**. Increment 4's branch `feat/config-loaders` was merged (fast-forward) and **deleted**.
- **Build/tests green:** `go test ./... -race` passes (`expr`, `stage`, `config`); `go build`/`vet`/`gofmt`/`golangci-lint run ./...` clean; `go mod tidy`/`go mod verify` clean. `govulncheck` **not installed** in this environment — run it when available.
- Deps (consumer-visible): `expr-lang/expr v1.17.8` **and `gopkg.in/yaml.v3 v3.0.1`** (added this increment for config, ADR-0008). `testify` is test-only. Go 1.25+ (toolchain 1.26.x).
- **Delivered previously (Increment 3):** the `stage.Pipeline` orchestrator (`stage/pipeline.go`) — `NewPipeline` validates duplicate names / unknown deps / cycles and computes a deterministic **input-order-preserving topological order**; `Run(ctx, *Scope)` walks it sequentially. Typed `DuplicateStageError`/`UnknownDependencyError`/`CycleError`. Spec 003, plan 003, ADR-0005/0006. Also added the **test-coverage gate** to CLAUDE.md (≥85% target; hard requirement = every hot-path + typed-error branch covered).
- **Delivered this session (Increment 4):** the `config` package — parse declarative YAML/JSON pipeline definitions and build a `*stage.Pipeline`. `ParseYAML`/`ParseJSON`/`LoadFile` (extension dispatch) → `*PipelineDef`; `(*PipelineDef).Build()` maps each `StageDef` (flat union, `type` discriminator, list-ordered) to the matching `stage.New*` constructor + `NewPipeline`. Reusable `ExprDef` with **scalar shorthand** (bare string = expression) decoding in both YAML and JSON, mapping its object form to `expr.Option`. Typed `ConfigError{Stage,Field,Cause}` unwrapping to the underlying stage/expr/pipeline error. **One new consumer dep: `gopkg.in/yaml.v3`** (no mimetype/validator — ADR-0008). `config` coverage **100%**. Spec 004, plan 004, ADR-0007/0008. **Known limitation:** per-decision options in a decision-table are rejected (rule-level only); a bare-string decision is the common case.

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

## What Increments 3–4 shipped (done — for context, not to redo)

- **Increment 3 — `stage.Pipeline`** (`stage/pipeline.go`): `NewPipeline(stages ...Stage) (*Pipeline, error)` (variadic; empty set valid), `Run(ctx, *Scope) error`. Validation order: duplicate names → unknown deps → cycle. Input-order-preserving O(n²) topological sort; `findCycle` returns a concrete loop `["a","b","a"]`. Typed `DuplicateStageError`/`UnknownDependencyError`/`CycleError` (ASCII `->` messages). Sequential deterministic (ADR-0006). ADR-0005/0006.
- **Increment 4 — `config` package** (`config/{errors,expr_def,def,parse,build,doc}.go`): `ParseYAML`/`ParseJSON`/`LoadFile` → `*PipelineDef`; `(*PipelineDef).Build() → *stage.Pipeline`. `ExprDef` scalar shorthand (YAML+JSON) → `expr.Option`. `ConfigError{Stage,Field,Cause}`. Dep: `gopkg.in/yaml.v3` only (ADR-0008). ADR-0007/0008. Per-decision decision-table options are **rejected** (rule-level only) — backlog item to extend `stage.Rule` if ever needed.

## Provisional design leads for Increment 5 — Result mapper + `Engine[I, R]` facade (refine during brainstorming — not binding)

Goal: the top-level, generic **`Engine[I, R]`** facade that ties everything together — flatten a typed input `I` into a `*stage.Scope`, `Run` the `*stage.Pipeline`, then **map** the accumulated `Scope` into a typed result `R`. This is the product's public front door (root `rlng` package, kept empty until now).
- **Package:** likely the root `rlng` package (the facade the README/consumers import). Decide + ADR.
- **`Engine[I, R]`** (`calculator.go` blueprint): generic over input `I` and result `R`. Construct from a `*config.PipelineDef` (or built `*stage.Pipeline`) + a mapping template; compile once; `Evaluate(ctx, I) (R, error)` (or `Calculate`) on the hot path. **Naming decision (ADR):** `Engine`/`Evaluate` vs `Calculator`/`Calculate` — ADR-0001 already leaned rule-oriented; ratify here.
- **Input seeding:** flatten a struct/map `I` into `Scope` via reflection (`structToMap` in the blueprint) — reuse `expr`'s env logic; a `map[string]any` input path too. This is the deferred "struct→map seeding" from spec 002.
- **Result `Mapper[R]`** (`mapper.go`/`mapping.go` blueprint): a `MappingTemplate` (hierarchical `fields`, each a leaf expression, flattenable to dot-paths) projecting the final `Scope` into `R` — struct (mapstructure tags), `map[string]any`, or slice. Compile each field expression once. **New dep?** `go-viper/mapstructure/v2` may be justified here for struct mapping — weigh vs. hand-rolled reflection; record in ADR.
- **VariablePatcher** (config-declared defaults injected as `x ?? <literal>` at compile time) — decide whether it lands here; it was deferred out of Increment 4.
- This is the **last increment** — after merge, the library is feature-complete for a `v0.0.x` tag (then work the release backlog below).

## Carried backlog (triage before the first `v0.0.x` release tag; not merge-blockers)

From increment 2–4 reviews (also in the SDD ledger):
- **Release-gate:** exported `.Error()`/`Unwrap()` methods and interface methods lack godoc across `stage` (`SingleExpr/MultiExpr/DecisionTable` `Name/Type/DependsOn`; `DuplicateStageError/UnknownDependencyError/CycleError`) and `config` (`ConfigError`) — the "every exported symbol documented" gate is release-bound. Add before tagging. (Consistent across the codebase; golangci-lint is clean.)
- **config:** per-decision decision-table options are rejected — extend `stage.Rule` to per-decision `DecisionOptions` if a real need appears.
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

Start **Increment 5 (Result mapper + `Engine[I, R]` facade)** from a fresh branch off `main` (e.g. `feat/engine-facade`), following the per-increment recipe above. Begin with `superpowers:brainstorming` (self-driven) to settle the facade design (see "Provisional design leads for Increment 5"), write ADR(s) + `docs/specs/005-*`, commit the spec standalone, then plan (`docs/plans/005-*`), then implement (TDD + coverage gate), then whole-branch `/code-review` + `/security-review`, then merge → push → delete branch. This is the **final increment**; after it, the library is ready for a first `v0.0.x` tag (work the release backlog first).

The autonomy grant (commit + push for increments 3–5) is **still in force for Increment 5** (the last one).
