# HANDOVER — rlng (updated 2026-07-11)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then the
> governing artifacts (`docs/specs/*`, `docs/plans/*`, `docs/adrs/*`). This file points at them;
> it does not restate them. **The 5-increment roadmap is COMPLETE and merged** — the next work is
> the first-release backlog (see "Next milestone"). The increments-3–5 autonomy grant is spent;
> re-confirm with the user before committing/pushing, and especially before any release tag.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first: no cgo,
typed wrapping errors that name field+expression). Delivered as a **5-increment roadmap**
(spec 001 §Roadmap):

| # | Increment | Status |
|---|-----------|--------|
| 1 | Expression core (`expr/`) | ✅ merged |
| 2 | Scope + stages (`stage/`) | ✅ merged + pushed |
| 3 | Stage DAG orchestration (`Pipeline`: depends_on topo-sort + cycle detection) | ✅ merged + pushed |
| 4 | Declarative config (`config/`: YAML/JSON loaders) | ✅ merged + pushed |
| 5 | Result mapper + `Engine[I, R]` facade (root `rlng`) | ✅ **merged + pushed** (this session) |

**🎉 The 5-increment roadmap is COMPLETE.** The library is feature-complete; the next milestone is the release backlog (below) then a first `v0.0.1` tag. The wrkflw `map`→`map` adapter is intentionally **out of rlng** (lives in the wrkflw repo).

## Exact state (safepoint)

- **Branch:** `main`. **HEAD:** the Increment-5 handover commit (this file), whose parent is `f325ef8` (`docs(rlng): note map input nested-reference seeding on Evaluate`). Pushed to `origin/main`.
- `git status --short`: **clean**. Increment 5's branch `feat/engine-facade` was merged (fast-forward) and **deleted**.
- **Build/tests green:** `go test ./... -race` passes (`rlng`, `expr`, `stage`, `config`); `go build`/`vet`/`gofmt`/`golangci-lint run ./...` clean; `go mod tidy`/`go mod verify` clean. `govulncheck` **not installed** in this environment — run it when available (first release-backlog item).
- Deps (consumer-visible): `expr-lang/expr v1.17.8`, `gopkg.in/yaml.v3 v3.0.1` (config, ADR-0008), **`github.com/go-viper/mapstructure/v2 v2.5.0`** (root `rlng`, ADR-0010). `testify` is test-only. Go 1.25+ (toolchain 1.26.x).
- **Delivered previously (Increment 3):** the `stage.Pipeline` orchestrator (`stage/pipeline.go`) — `NewPipeline` validates duplicate names / unknown deps / cycles and computes a deterministic **input-order-preserving topological order**; `Run(ctx, *Scope)` walks it sequentially. Typed `DuplicateStageError`/`UnknownDependencyError`/`CycleError`. Spec 003, plan 003, ADR-0005/0006. Also added the **test-coverage gate** to CLAUDE.md (≥85% target; hard requirement = every hot-path + typed-error branch covered).
- **Delivered previously (Increment 4):** the `config` package — parse declarative YAML/JSON pipeline definitions and build a `*stage.Pipeline`. `ParseYAML`/`ParseJSON`/`LoadFile` → `*PipelineDef`; `(*PipelineDef).Build() → *stage.Pipeline`. Reusable `ExprDef` scalar shorthand (YAML+JSON) → `expr.Option`. `ConfigError{Stage,Field,Cause}`. Dep `gopkg.in/yaml.v3` only. Spec 004, plan 004, ADR-0007/0008. Per-decision decision-table options rejected (rule-level only).
- **Delivered this session (Increment 5, FINAL):** the root `rlng` package facade. `Engine[I,R]` — `New[I,R](pipeline *stage.Pipeline, mapper *Mapper[R], opts ...Option)`, `Evaluate(ctx, I) (R, error)`: flatten `I` into a `Scope` (map passthrough, or mapstructure for structs), `Run` the pipeline, map the final `Scope` into `R`. `Mapper[R]` + `MappingTemplate map[string]string` (output dot-path → expression) compiled once; `Map(scope) (R, error)` → nested map → mapstructure-decode into `R`. `WithScopeOptions` threads `stage.ScopeOption`. Typed `MappingError{Field,Cause}`; pipeline/stage errors pass through unwrapped. Realizes the `Engine`/`Evaluate` naming (ADR-0001). **One new consumer dep: `github.com/go-viper/mapstructure/v2`** (ADR-0010). Root `rlng` does **not** import `config` (siblings). Root package coverage **100%**. Spec 005, plan 005, ADR-0009/0010.

## Autonomy grant (USER DIRECTIVE this session — applies to increments 3→5)

The user directed: **execute increments 3→5 autonomously**, then handed off to a fresh session to begin. For that run:

- **Standing `commit` + `push` authorization** for increments 3–5 (overrides the per-action approval rule *for this run only*). Still gate every push on: green `go test ./... -race`, and **all** `/code-review` + `/security-review` findings resolved or explicitly triaged.
- **Do NOT block on design approval.** The user is AFK and reviews code + docs on **GitHub** post-hoc. During each increment's brainstorming, make defensible default decisions yourself and **record each as an ADR** (`docs/adrs/000N-*`), rather than asking. (The brainstorming skill's user-approval gate is satisfied by this delegation + GitHub review.)
- **Maximize parallelism.** Increments are dependency-ordered (4 needs 3, 5 needs 4) so run them in sequence; *within* each increment, parallelize independent implementation tasks (isolated **git worktrees** — `Agent` with `isolation: "worktree"`, or dispatch concurrent implementers only when they touch disjoint files) and run reviews concurrently.
- **Delivery per increment:** fresh branch off `main` → work → merge (fast-forward, keep linear history like increments 1–2) → `git push origin main` → delete the branch (local; branches here are not pushed).

> ⚠️ This standing authorization is **scoped to increments 3–5 of this roadmap**. If you are resuming much later or the roadmap has changed, re-confirm with the user before relying on it.

## Per-increment recipe (roadmap complete — reuse this loop for any future feature/fix)

1. **Fresh branch** off `main` (e.g. `feat/<slug>`).
2. **Brainstorm/design** (`superpowers:brainstorming`, self-driven — no user questions). Settle the open decisions, record them as **ADRs**.
3. **Write the spec** → `docs/specs/00N-<slug>.md`. Commit it **standalone** (`spec:` type, `Spec: 00N` trailer). Specs precede code.
4. **Write the plan** (`superpowers:writing-plans`) → `docs/plans/00N-<slug>.md`. The plan file rides with the **first `feat` commit** (never a standalone plan commit). Follow the `table-test` skill for all test code (assert-closure form, `ctx` modifier + `t.Context()` for context-sensitive components, canceled-context case).
5. **Implement.** For a **transcription-complete** plan with few, dependency-chained tasks, **inline TDD execution** (write failing test → run red → implement → run green → per-task commit) is acceptable and was used for Increment 3. For larger/independent-task plans use `superpowers:subagent-driven-development` (fresh implementer per task — **haiku** for pure transcription, **sonnet** for judgment/multi-file — **sonnet** task reviewer per task, fix-loop on Critical/Important). Either way: per-task commit (pre-authorized), `-race` green per task, and **the test-coverage gate** (≥85% + every hot-path/typed-error branch tested; enumerate branches in the plan). Record progress in the SDD ledger (below).
6. **Whole-branch gate:** `scripts/review-package $(git merge-base main HEAD) HEAD` → dispatch a final reviewer on **opus**; then run `/security-review`. Resolve/triage **every** finding; re-run `-race`.
7. **Merge → push → delete branch.** Update THIS handover at the increment boundary.

**Known gotcha for the plan's test code:** `expr-lang` does **float** division, so `1/0 → +Inf` (no error). To trigger a genuine eval error in a test, use modulo (`a % b`, `b=0` → runtime "integer divide by zero"). Increment 2's decision-table/single-expr error tests use this.

## What the roadmap shipped (done — for context, not to redo)

- **Increment 1 — `expr`**: atomic evaluators `Predicate.Test`/`Function.Apply`, `expr.Option` (globals/locals/coerce/fallback/return-kind), variable-default patcher, typed `CompileError`/`EvalError`. ADR-0001.
- **Increment 2 — `stage`**: `Scope` (dot-path Set/Get/Snapshot, mutex, `WithStrict`), `Stage` interface, `SingleExpr`/`MultiExpr`/`DecisionTable` (single/collect hit policies), shared `Option`, typed `StageError`. ADR-0002/0003/0004.
- **Increment 3 — `stage.Pipeline`** (`stage/pipeline.go`): `NewPipeline(stages ...Stage)` + `Run(ctx, *Scope)`; validate duplicate/unknown-dep/cycle; input-order-preserving topo sort; `findCycle` → concrete loop `["a","b","a"]`; typed `DuplicateStageError`/`UnknownDependencyError`/`CycleError`; sequential deterministic. ADR-0005/0006.
- **Increment 4 — `config`**: `ParseYAML`/`ParseJSON`/`LoadFile` → `*PipelineDef`; `Build() → *stage.Pipeline`; `ExprDef` scalar shorthand; `ConfigError`. Dep `yaml.v3`. ADR-0007/0008.
- **Increment 5 — root `rlng`**: `Engine[I,R]` + `Evaluate`, `Mapper[R]` + `MappingTemplate`, mapstructure input/result decode, `MappingError`. Dep `mapstructure/v2`. ADR-0009/0010.

## Next milestone — first release (`v0.0.1`) backlog

The roadmap is done. A first **release-prep** pass (branch `chore/release-prep`) closed several of these; what's left before tagging `v0.0.1` follows. None are merge-blockers.

**Done in release-prep (2026-07-11):**
- ✅ **`govulncheck ./...`** run — **0 vulnerabilities in `rlng`'s code**. The only advisories (`GO-2026-4970` in `os`, `GO-2026-5856` in `crypto/tls`) are **stdlib**, fixed in **go1.26.5**, and not called by our code. → **Bump the build/CI toolchain to go1.26.5** at release time (no source change needed).
- ✅ **Godoc completeness** — added godoc to all exported `.Error()`/`Unwrap()` and stage `Name/Type/DependsOn` methods across `expr`/`stage`/`config`/root; an audit shows no remaining undocumented exported top-level symbols.
- ✅ **README** refreshed (removed the stale "not written yet"; added a runnable end-to-end `config`→`Build`→`Engine` example).
- ✅ **`StageError.Error()` nil-`Cause` guard** fixed + tested (the `expr` errors were already guarded; `ConfigError`/`MappingError` use nil-safe `%v`).
- ✅ **Missing tests added** — `MultiExpr` stable-tie ordering; `DecisionTable` empty-output-key rejection.

**Still open before `v0.0.1`:**
- **Toolchain:** build/test on **go1.26.5+** (clears the two stdlib advisories) and confirm the CI matrix covers the supported Go versions (1.25+).
- **API surface baseline:** run `gorelease`/`apidiff` conventions to establish the `v0.0.1` baseline (only meaningful at/after the first tag).
- **config:** per-decision decision-table options are rejected — extend `stage.Rule` to per-decision `DecisionOptions` if a real need appears.
- **rlng:** config-declared **output mapping** is not implemented (the `MappingTemplate` is programmatic) — a natural follow-up if declarative result mapping is wanted. `VariablePatcher` config defaults still deferred.
- `Scope.Set` doesn't validate empty path **segments** (`"a.."`, `".a"` create a `""` key) — inert but worth a guard/doc.
- `DecisionTable.executeSingle`/`executeCollect` duplicate iteration scaffolding — `/simplify` candidate.
- Stages are **not transactional** (single/multi persist each write incrementally; collect is atomic) — document the semantic.
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

**The 5-increment roadmap is complete and merged to `main`.** The next milestone is the first release: work the **"Next milestone — first release (`v0.0.1`) backlog"** above (start with `govulncheck` and the godoc-completeness gate), then push an annotated SemVer tag `v0.0.1` (which the `release` workflow turns into a GitHub Release — see CLAUDE.md §License & release). Tagging is a release action: **get explicit user approval before pushing any tag** (the increments-3–5 autonomy grant covered feature commits/pushes, not release tags).

Any further work is ordinary feature/fix work — reuse the per-increment recipe above on a fresh branch, and **re-confirm with the user** before committing/pushing, since the autonomy grant was scoped to increments 3–5 (now done).
