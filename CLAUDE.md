# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

**Greenfield.** The repo is bootstrapped (git, `README.md`, `.claude/` skills) but has **no Go module or source yet**. `rlng` ("rule engine") is a Go **rule engine library** to be built. This file is a design brief so future work starts from the intended architecture rather than reverse-engineering it.

When you scaffold the module, do not blindly copy the reference package below — it is a *calculation* engine. Adapt its patterns to rule evaluation. Update this file (remove this status section, add real build commands) once the module exists.

**Open decisions to record first (as ADRs):** the rule-vs-`calc` naming (e.g. `Engine`/`Evaluate` vs `Calculator`/`Calculate`). The module path is `github.com/kartaladev/rlng` (matches the git remote and README). Ratify both in `docs/adrs/0001-*` before or with the first `feat` commit.

## Development workflow (mandatory)

Follow this loop for every feature or bugfix, not just large ones. The skills named here are required steps, not suggestions.

1. **Brainstorm first.** Before any creative/implementation work, run `superpowers:brainstorming` to explore intent, requirements, and design. Don't jump to code. For a multi-step task, follow up with `superpowers:writing-plans`.
2. **TDD — red → green → refactor.** Use `superpowers:test-driven-development`. Write a failing test first (red), make it pass with the simplest code (green), then refactor. Never write implementation ahead of a failing test.
3. **Consult the Go skills while coding** (see below) — start from `cc-skills-golang:golang-how-to`, which routes you to the specific `golang-*` skills for the task.
4. **On big or complex features, refactor with `/simplify`** once green, to clean up reuse/simplification/efficiency/altitude before review.
5. **Gate before delivering (committing):** in order — run **`/code-review`** on the diff and address findings, run **`/security-review`** on the pending changes and resolve anything it flags, then **re-run the project-wide test suite** (`go test ./... -race`) and confirm it passes. Use `superpowers:verification-before-completion` — evidence before claiming done. Only commit after all three pass. For release-bound changes, the full **Library quality gates** (see below) also apply.

**Fix review findings before delivering — mandatory.** The gate's `/code-review` and `/security-review` are not advisory: every finding they surface must be **fixed**, or **explicitly triaged to a backlog with a written rationale**, before the work is delivered. Re-run the affected review and the `-race` suite after fixing. **When completing a feature branch, run `/code-review` and `/security-review` over the whole-branch diff** (`main..HEAD`, not just the last commit) as the final pre-merge gate, resolve/triage every finding, and confirm the suite is green — this is the same path used per-increment, applied once more before any merge/push. Never merge or push a branch that still has unaddressed review findings.

**Delete the feature branch after it merges to `main`.** Once the branch is merged, remove it — `git branch -d <branch>` locally, and delete the remote branch too if it was pushed (`git push origin --delete <branch>`). Don't leave merged branches lingering; each increment starts from a fresh branch off `main`.

**Never `git commit` or `git push` without explicit user approval — this is a hard rule, with one scoped exception (below).** Ask first every time, even for trivial or "obvious" changes, and even when the user previously approved a similar action; approval is per-action, never standing. When work is ready, stage it, show what would be committed/pushed, and wait for the go-ahead. When the user does approve, the **pre-commit gate** (Development workflow §5: `/code-review` → `/security-review` → full `-race` suite) is an additional hard precondition before committing.

**Exception — per-task commits during plan execution.** Once the user has approved a written plan (`docs/plans/*`) *and* chosen a task-by-task execution mode (`superpowers:subagent-driven-development` or `superpowers:executing-plans`), the per-task commits enumerated in that plan are **pre-authorized**: commit each completed, green task without pausing for per-commit approval. This standing authorization is narrowly scoped and does **not** relax anything else:
- It covers `git commit` **only** — `git push`, merges, tags, and any branch deletion still require explicit per-action approval.
- Each task must be a **green unit** — its `go test ./... -race` passes — before its commit; no WIP/broken-build commits.
- The **whole-branch delivery gate** (`/code-review` + `/security-review` over `main..HEAD`, findings resolved/triaged, `-race` green) still runs before the final increment commit, exactly as §5 requires.
- It applies only to commits the approved plan spells out. Any commit *not* in the plan (or made outside an active plan-execution workflow) falls back to the default: ask first.

**Proactively recommend alternatives.** Whenever a decision has to be made — design, library, API shape, trade-off — don't silently pick one. Surface the viable options with their pros/cons and state a recommended default, so the user can steer before you proceed.

## Documentation artifacts

Persist the workflow's written outputs under `docs/`, each **prefixed with an incrementing version number**:

- **Specs** (from `superpowers:brainstorming` / spec work) → `docs/specs/` — e.g. `docs/specs/001-<slug>.md`.
- **Plans** (from `superpowers:writing-plans`) → `docs/plans/` — e.g. `docs/plans/001-<slug>.md`. Pair a plan's number with its originating spec where practical.
- **Architecture Decision Records** → `docs/adrs/`, one file per decision, following **Michael Nygard's ADR convention** (Title, Status, Context, Decision, Consequences), numbered incrementally — e.g. `docs/adrs/0001-<slug>.md`. Record *every* architectural decision as it is made; supersede rather than rewrite old ADRs (set the old one's Status to `Superseded by ADR-NNNN`).

**Traceability is a hard requirement.** Every artifact must be cross-linked so any decision can be traced end to end — spec → plan → ADR(s) → code/commit — and back. Concretely:

- A **plan** must reference the **spec** it implements; a **spec** should list the plans that realize it.
- An **ADR** must cite the spec/plan that prompted it; the plan (and any relevant spec) must link back to the ADRs it depends on.
- **Code and commits** must reference the driving artifact (e.g. `Implements spec 003 / plan 003; see ADR-0007`) so reviewers can follow the chain.
- Do not merge or commit work whose governing spec/plan/ADR link is missing. A new artifact with no traceable parent (or a decision with no ADR) is incomplete.

## Session handover

To survive context limits without hallucinating, hand off through a written document rather than relying on a cluttered context.

**When to hand over:**
- **On request** — whenever the user asks to "hand over"/"handover"/"hand off".
- **Proactively at ~60% context usage** — stop starting new work, write the handover, and then **ask the user to continue in a fresh session**. Do not silently push past 60%.

**Hand over only from a safepoint — this is mandatory.** The next session must start well-grounded, so never write a handover mid-edit, with a broken build, or with failing tests. A safepoint means: the tree builds, the relevant tests are green, and you are at a clean task/step boundary. To reach one, either finish the in-flight step to green, or revert the incomplete edits back to the last green state — then capture that state in the handover. If you cannot reach a safepoint, say so explicitly and record the exact partial state (files touched, what's half-done, how to revert) so the next session can recover deterministically rather than guess.

**Where:** a single living file **`docs/HANDOVER.md`**, overwritten each time so it always reflects the latest state. It is a `docs:` artifact — offer to commit it (subject to the never-commit-without-approval rule); a committed handover survives a fresh clone.

**What it MUST contain** (enough for a brand-new session to resume with zero prior context — reference the governing artifacts, don't restate them, to avoid drift):
1. **Objective & roadmap position** — what we're building and which spec/plan/increment/**task+step** is active (e.g. "Plan 001, Task 4, Step 3").
2. **Exact state** — done (with commit SHAs), in progress, and the verbatim `git status --short` + last commit line; call out anything uncommitted in the working tree.
3. **Traceability pointers** — the active `docs/specs/*`, `docs/plans/*`, and `docs/adrs/*` files, plus CLAUDE.md, that the fresh session must read *first*.
4. **Decisions & deviations** this session, and any **pending approvals** or open questions blocking progress.
5. **Next actions** — the precise next steps and commands to run to resume.
6. **Gotchas/environment** — anything non-obvious needed to continue (tools, paths, credentials-by-reference).

Begin the document with an explicit instruction to the next session: *read CLAUDE.md and the referenced spec/plan/ADR before acting, and trust those files over any memory.*

## Commit discipline

Commit **completed, green units of work** — a coherent increment whose tests pass (and that has cleared the **pre-commit gate**, Development workflow §5). No WIP or broken-build commits.

Use **Conventional Commits**: `type(scope): summary`, where `type` names the activity —

- `feat` — new behavior/capability
- `fix` — bug fix
- `refactor` — behavior-preserving restructure (e.g. after `/simplify`)
- `spec` — a new/updated spec (from brainstorming), committed **standalone** since specs precede code
- also allowed: `test`, `docs`, `chore`, `perf`, `build`, `ci`

**Couple plans and ADRs with the code that realizes them.** Do *not* make separate plan/ADR commits. Because plans and ADRs are routinely revised during implementation, the plan/ADR changes ride in the **same commit** as the `feat`/`fix`/`refactor` code — keeping the artifact and its implementation atomic and never out of sync. (Specs are the exception: they're authored before code, so a `spec` commit stands alone.)

**Recommended refinement — traceability trailers.** To make the hard traceability requirement machine-checkable rather than prose-only, put the links in Conventional-Commit **footer trailers** instead of freeform text:

```
feat(engine): add decision-table collect mode

Implements the collect-mode evaluation path and updates the plan/ADR
to reflect the ordered-rule semantics settled during implementation.

Spec: 003
Plan: 003
ADR: 0007
```

This keeps the spec→plan→ADR→commit chain greppable (`git log --grep`), survives rebases, and lets CI enforce that every `feat`/`fix`/`refactor` commit carries at least a `Plan:` (and, for architectural changes, an `ADR:`) trailer. Prefer this over embedding references in the subject line.

## Go conventions & skills

This is a Go project — start every Go coding, review, or debug task from **`cc-skills-golang:golang-how-to`** (the always-on orchestrator from `samber/cc-skills-golang`). It reads the task and pulls in the relevant `golang-*` skills (error handling, naming, design patterns, structs/interfaces, concurrency, testing, lint, …). Consult those skills rather than working from memory.

Three **project-local skills override samber's testing guidance** where they conflict — prefer these:

- **`table-test`** — table-driven tests use the `assert` closure form (not `want`/`wantErr` fields), a `ctx` modifier for context-sensitive components, and `t.Context()` over `context.Background()`. Fold ≥2 cases exercising the same call into a table. Overrides `cc-skills-golang:golang-testing`.
- **`use-mockgen`** — generate test doubles with uber-go/mock (`mockgen`, `--typed`), placed alongside the interface in the producer package via `//go:generate`. Overrides mock-generation in `golang-testing` / `golang-stretchr-testify`.
- **`use-testcontainers`** — provision heavy external resources (Postgres, Redis, Kafka, MinIO, …) via testcontainers-go, never mocks/in-memory fakes; expose each through a single `RunTestX(t, opts...)` helper. Overrides integration-test scaffolding in `golang-testing`.

## What this is — and is not

- **Is:** a rule *engine* — an importable Go **library** that compiles rule/expression definitions (from YAML/JSON/config) and evaluates them against runtime input. Consumed via `go get` + import; the exported API is the product.
- **Is not:** a Business Rules Management System (BRMS). No rule authoring UI, versioning workflow, governance, or persistence layer. Just the engine.
- **No binary deliverable.** There is **no `main` package, no `cmd/`, no CLI or server** to ship. Any `main` is throwaway (examples/manual repro only) and must not become a deliverable. Library code must not call `os.Exit`, `log.Fatal`, or `panic` on caller input (return errors instead), and must not log to a global logger by default — accept an injected logger/handler if logging is needed.

## License & release

- **License:** Apache-2.0. The verbatim text lives in `LICENSE`. New source files may carry the standard short Apache header; keep third-party attributions in a `NOTICE` file if any are added.
- **Releases are tag-driven.** Push an annotated **SemVer** tag `vX.Y.Z` (e.g. `v0.0.1`) and the `release` GitHub Action (`.github/workflows/release.yml`) publishes a **GitHub Release** with auto-generated notes via the `gh` CLI. Consumers get the version via `go get module@vX.Y.Z`; **the tag itself is the distribution** — nothing is compiled or uploaded (no GoReleaser, no binaries, this is a library). Tags with a pre-release suffix (`v0.0.1-rc.1`) are marked as pre-releases automatically. Release-note categories are configured in `.github/release.yml` (keyed to PR labels).
- Tag versioning must obey the SemVer/API-compatibility gate below (breaking exported API ⇒ major bump ⇒ ADR).

## Library quality gates

Because the deliverable is a package other code imports, the exported surface *is* the contract. In addition to the **pre-commit gate** (Development workflow §5), the following must hold before any release-bound change is considered done:

- **Everything builds & tests green, race-clean:** `go build ./...` and `go test ./... -race` pass; `go vet ./...` and `golangci-lint run ./...` are clean; `gofmt`/`gofumpt` report nothing.
- **Module hygiene:** `go mod tidy` leaves `go.mod`/`go.sum` unchanged and `go mod verify` passes. Keep the **dependency set minimal** — every direct dep becomes a transitive dep for every consumer; justify additions.
- **Public API is documented & deliberate:** every exported symbol has a godoc comment; keep internals in `internal/` so they can't be imported. Prefer a small, stable surface — "accept interfaces, return structs."
- **API compatibility (SemVer):** no breaking change to an exported symbol without a major-version bump. Check with `gorelease` / `go run golang.org/x/exp/cmd/apidiff` against the last tag; **deprecate** (doc comment + keep working) before removing. Any intended break is an architectural decision → record an ADR.
- **Pure Go, no cgo:** `CGO_ENABLED=0 go build ./...` must succeed — keeps the library cross-compilable and debuggable (see debuggability criterion below).
- **Runnable examples & coverage:** exported behavior is covered by `Example…` tests (they double as godoc) and table tests; watch coverage on the public packages, don't just chase a number.
- **Vulnerability scan:** `govulncheck ./...` is clean.
- **Multi-version support:** builds/tests on the supported Go versions (Go 1.25+), not just the local toolchain.

## Core quality criterion: debuggability

The primary reason this exists rather than using [gorules/zen-go](https://github.com/gorules/zen-go): zen-go binds to a Rust engine via **cgo**, which makes it hard to debug. `rlng` must be **pure Go** — no cgo — so a developer can set a breakpoint, step through evaluation, and read a plain Go stack trace. Treat "can I debug this with a normal Go debugger and readable errors?" as a first-class design constraint on every decision. Prefer typed, wrapping errors that name the offending field and expression (see the error types in the reference) over opaque failures.

## Foundations

- Built **on top of [expr-lang/expr](https://github.com/expr-lang/expr)** (`v1.17.x`) — the expression language and its compile/VM (`expr.Compile` → `*vm.Program`, `expr.Run`). This is the evaluation core; do not reinvent it.
- Design and patterns are seeded by the `pkg/calc` package of `bbn-crm-backend` (GitLab: `teknikal1/bbn-crm-backend`, `development` branch). That package is a *calculation* engine but its structure is the blueprint for this rule engine. Clone it for reference:
  `git clone -b development git@gitlab.com:teknikal1/bbn-crm-backend.git` then read `pkg/calc/`.
- Inspiration (not dependency) from `gorules/zen-go` for the *decision-table* / rule-evaluation model — reimplemented in pure Go.

## Architecture blueprint (from the `calc` reference)

The reference wires config → compiled programs → staged evaluation → mapped result. Expect `rlng` to mirror this shape (renamed for rules):

- **ConfigLoader[T]** (`config.go`): pluggable source abstraction — `StaticConfigLoader`, `ParseYamlConfigLoader`, `ParseJsonConfigLoader`, `FilesystemConfigLoader` (content-type sniffed via mimetype). Definitions are declarative YAML or JSON with custom `UnmarshalYAML`/`UnmarshalJSON` supporting a **scalar shorthand** (a bare string = the expression) or a full object form.

- **Engine/Calculator[I, R]** (`calculator.go`): generic over input type `I` and result type `R`. Loads config, validates (`go-playground/validator`), compiles everything up front, then `Calculate(input)` runs stages in order and maps the accumulated context into `R`. Compilation happens once at construction; evaluation is the hot path.

- **Context** (`context.go`): a `map[string]any` accumulator threaded through evaluation. Input struct/map is flattened into it via reflection (`structToMap`); each stage writes its output back under a dot-separated path (`Set("stage.field", v)`). Supports nested paths and an override/strict-mode guard. This map *is* the `expr` evaluation environment. (The increment-2 implementation names this accumulator `Scope`, package `stage` — see `stage/scope.go`.)

- **Stages** (`stage.go`): a stage is `Name() / Type() / DependsOn() / Execute(ctx)`. Three types today:
  - `single-expr` — one expression, optional `condition` gate and `fallback_expr`.
  - `multi-expr` — several named expressions with priority ordering.
  - `decision-table` — ordered rules, each a **predicate condition** + a set of **decisions** (functions); `mode: single` (first match wins) or `mode: collect` (accumulate all matches).
  Stages declare `depends_on` and are **topologically sorted** (Kahn's algorithm) with cycle detection before execution — this is the DAG that orders rule evaluation.

- **Predicates & Functions** (`expr.go`): `CompiledPredicate.Test(env) (bool, error)` with lenient truthiness coercion (numbers, strings, slices → bool) unless strict; `CompiledFunction.Apply(env)` with fallback-on-error. Both built via a functional-options pattern (`lestrrat-go/option`, `WithGlobalVariables`, `WithStrictMode`, `WithReturnType`, …).

- **VariablePatcher** (`expr.go`): an `ast.Visitor` that injects config-declared variables (globals + per-stage locals) into expressions at **compile time** by rewriting each matching identifier `x` into `x ?? <literal>`. So variables act as defaults overridable by runtime input. Only scalar kinds (bool/string/int/uint/float) are patched. This is the mechanism to reuse for named constants/thresholds in rules.

- **Mapper[R]** (`mapper.go`, `mapping.go`): a `MappingTemplate` (hierarchical `fields`, each a leaf expression, flattenable to dot-paths) that projects the final context map into the typed result `R` — struct (via `mapstructure` tags), `map[string]any`, or slice. Compiles each field expression once at construction.

- **Errors** (`errors.go`): typed, `Unwrap`-able errors carrying the field path + expression (`ExprCompilationError`, `MappingError`, `TypeValidationError`). Keep this discipline — it is central to the debuggability goal.

### Key data-flow invariant

Everything flows through a `map[string]any` context, and every compiled `*vm.Program` reads/writes that map by string path. `expr.AllowUndefinedVariables()` is used widely so referencing a not-yet-computed field yields nil rather than a compile error — depend on stage ordering (the DAG), not on the compiler, to guarantee a value exists when read.

## Likely dependencies (from the reference)

`github.com/expr-lang/expr`, `github.com/go-viper/mapstructure/v2`, `github.com/go-playground/validator/v10`, `github.com/lestrrat-go/option`, `github.com/gabriel-vasile/mimetype`, `gopkg.in/yaml.v3`. Target Go 1.25+.

## Commands

Standard Go tooling once the module is scaffolded (`go mod init github.com/kartaladev/rlng`):

```bash
go build ./...
go test ./...
go test ./... -race                       # concurrency: Context and MapAccess use mutexes
go test -run TestName ./path/to/pkg       # single test
go test -run TestName/subtest ./path      # single subtest
go test -run '^Example' ./...             # runnable example tests (reference uses many)
go vet ./...
gofmt -l .                                # or: gofumpt -l .
golangci-lint run ./...                   # once .golangci.yml is added (see cc-skills-golang:golang-lint)
govulncheck ./...                         # vulnerability scan (library quality gate)

# Release (library — the tag is the distribution; the workflow publishes a GitHub Release)
git tag -a v0.0.1 -m "v0.0.1" && git push origin v0.0.1   # triggers .github/workflows/release.yml
```

Prefer runnable **Example tests** (`func Example...() { ... // Output: ... }`) for documenting engine behavior — the reference `calc` package leans on them heavily and they double as compilable docs.
