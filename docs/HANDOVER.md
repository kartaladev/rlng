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
| 2 | Scope + stages (`stage/`) | ✅ **merged + pushed** (this session) |
| 3 | **Stage DAG orchestration** (depends_on topo-sort + cycle detection) | ⏭️ **NEXT — start here** |
| 4 | Declarative config (YAML/JSON loaders) | pending |
| 5 | Result mapper + `Engine[I, R]` facade | pending |

None of specs 003–005 exist yet. Each increment is a full cycle: **brainstorm/design → spec →
plan → implement → whole-branch review → push**. The wrkflw `map`→`map` adapter is intentionally
**out of rlng** (lives in the wrkflw repo) — not a future increment here.

## Exact state (safepoint)

- **Branch:** `main`. **HEAD:** `cbd9049` (`refactor(stage): unify stage options and align HitPolicy naming`). Pushed to `origin/main`.
- `git status --short`: **clean**. Increment 2's branch `feat/scope-and-stages` was merged (fast-forward) and **deleted**.
- **Build/tests green:** `go test ./... -race` passes (`expr`, `stage`); `go build`/`vet`/`gofmt`/`golangci-lint` clean; `go mod tidy` a no-op. `govulncheck` **not installed** in this environment — run it when available.
- Deps (consumer-visible): only `expr-lang/expr v1.17.8`. `testify` is test-only. Go 1.25+ (toolchain 1.26.x).
- **Delivered this session (Increment 2):** the `stage` package — `Scope` (dot-path Set/Get/Snapshot, mutex-guarded, `WithStrict`), the `Stage` interface (`Name/Type/DependsOn/Execute(ctx, *Scope)`), and `SingleExpr` / `MultiExpr` / `DecisionTable` (single/collect hit policies), a single shared `Option` type, typed `StageError`. ADR-0002/0003/0004.

## Autonomy grant (USER DIRECTIVE this session — applies to increments 3→5)

The user directed: **execute increments 3→5 autonomously**, then handed off to a fresh session to begin. For that run:

- **Standing `commit` + `push` authorization** for increments 3–5 (overrides the per-action approval rule *for this run only*). Still gate every push on: green `go test ./... -race`, and **all** `/code-review` + `/security-review` findings resolved or explicitly triaged.
- **Do NOT block on design approval.** The user is AFK and reviews code + docs on **GitHub** post-hoc. During each increment's brainstorming, make defensible default decisions yourself and **record each as an ADR** (`docs/adrs/000N-*`), rather than asking. (The brainstorming skill's user-approval gate is satisfied by this delegation + GitHub review.)
- **Maximize parallelism.** Increments are dependency-ordered (4 needs 3, 5 needs 4) so run them in sequence; *within* each increment, parallelize independent implementation tasks (isolated **git worktrees** — `Agent` with `isolation: "worktree"`, or dispatch concurrent implementers only when they touch disjoint files) and run reviews concurrently.
- **Delivery per increment:** fresh branch off `main` → work → merge (fast-forward, keep linear history like increments 1–2) → `git push origin main` → delete the branch (local; branches here are not pushed).

> ⚠️ This standing authorization is **scoped to increments 3–5 of this roadmap**. If you are resuming much later or the roadmap has changed, re-confirm with the user before relying on it.

## Per-increment recipe (follow for 3, then 4, then 5)

1. **Fresh branch** off `main` (e.g. `feat/dag-orchestration` for inc 3).
2. **Brainstorm/design** (`superpowers:brainstorming`, self-driven — no user questions). Settle the open decisions, record them as **ADRs**.
3. **Write the spec** → `docs/specs/00N-<slug>.md`. Commit it **standalone** (`spec:` type, `Spec: 00N` trailer). Specs precede code.
4. **Write the plan** (`superpowers:writing-plans`) → `docs/plans/00N-<slug>.md`. The plan file rides with the **first `feat` commit** (never a standalone plan commit). Follow the `table-test` skill for all test code (assert-closure form, `ctx` modifier + `t.Context()` for context-sensitive components, canceled-context case).
5. **Implement** with `superpowers:subagent-driven-development`: fresh implementer per task (**haiku** for plans whose task text contains complete code = transcription; **sonnet** for judgment/multi-file), **sonnet** task reviewer per task, fix-loop on Critical/Important, per-task commit (pre-authorized), `-race` green per task. Record progress in the SDD ledger (below).
6. **Whole-branch gate:** `scripts/review-package $(git merge-base main HEAD) HEAD` → dispatch a final reviewer on **opus**; then run `/security-review`. Resolve/triage **every** finding; re-run `-race`.
7. **Merge → push → delete branch.** Update THIS handover at the increment boundary.

**Known gotcha for the plan's test code:** `expr-lang` does **float** division, so `1/0 → +Inf` (no error). To trigger a genuine eval error in a test, use modulo (`a % b`, `b=0` → runtime "integer divide by zero"). Increment 2's decision-table/single-expr error tests use this.

## Provisional design leads for Increment 3 (refine during brainstorming — not binding)

- A `Pipeline` (name TBD — `Pipeline`/`Graph`/`Flow`) that holds `[]Stage`, validates at construction (unique names, every `DependsOn` target exists, **no cycles via Kahn's algorithm**), computes execution order once, and `Run(ctx, *Scope) error` executes stages in dependency order.
- **Package:** likely in `stage` (it operates on `Stage`+`Scope`) — decide + ADR. Root stays clean for the Increment-5 `Engine` facade.
- **Execution model decision (ADR-worthy):** sequential topo-order (deterministic, debuggable — the project's core criterion) **vs** concurrent execution of independent stages. Spec 002's documented concurrency invariant (stages write disjoint name-namespaces; `Snapshot` shares nested maps safely) was written to *enable* parallelism. Recommendation: ship **sequential ordered execution** as the default for debuggability; consider a `WithConcurrency` opt-in or defer parallelism — record the choice in an ADR.
- **Typed errors** at construction: cycle, unknown-dependency, duplicate-stage (keep the debuggability discipline). Thread `context.Context`; stop on first stage error, naming the failing stage.

## Carried backlog (triage before the first `v0.0.x` release tag; not merge-blockers)

From increment-2 reviews (also in the SDD ledger):
- **Release-gate:** `SingleExpr/MultiExpr/DecisionTable` `Name()/Type()/DependsOn()` lack godoc — the "every exported symbol documented" gate is release-bound. Add before tagging.
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

Start **Increment 3** from a fresh branch off `main`, following the per-increment recipe above. Begin with `superpowers:brainstorming` (self-driven) to settle the `Pipeline` design, write ADR(s) + `docs/specs/003-*`, then plan, then implement.
