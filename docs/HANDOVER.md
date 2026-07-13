# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then `docs/BACKLOG.md`
> (the tracked backlog B1–B12), then `git log` on `main`. For the item you pick up, read its
> `docs/specs/*` + `docs/plans/*` + related ADRs first. Trust the repo (code + git history + `docs/`) over
> any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Increments 011–016 merged & pushed. **Increment 016 (config `Provider`) complete.**

**Active program: execute ALL 12 backlog items (B1–B12) from `docs/BACKLOG.md`,** each as its own
increment. **B1 (incr 017, `f10a8be`); B2 (incr 018, `934f1b5`); B3 (incr 019, `2af21f2`); B4 (incr 020,
`4b3089e`).** The autonomous set (B1–B4) is complete. **B5 IN PROGRESS (incr 021): design checkpoint CLEARED
— user approved Option A; spec 021 committed to `main` (`32978f7`); implementation is the next action.**

## Standing decisions for this program (do NOT re-ask)
- **Scope = all 12**, including the two deliberate non-goals **B11** (parallel exec; write a superseding
  ADR for ADR-0006 first) and **B12** (YAML env/host functions; superseding ADR for ADR-0028 first).
- **Cadence = AUTO merge+push each green increment** (standing authorization). After each increment's full
  gate passes, merge to `main`, push, delete branch, start the next — no per-increment merge/push approval.
  Does NOT extend to release tags (still need explicit approval).
- **Design-gating = checkpoint only the risky ones.** Design + implement **B2, B3, B4, B10** autonomously
  (surface design via the committed spec, no live approval pause). **PAUSE for the user's design approval**
  before implementing **B5, B6, B7, B8, B9, B11, B12**. Code-review + security-review gates run on every
  increment regardless.
- Per-increment workflow: brainstorm → spec `docs/specs/NNN` → plan `docs/plans/NNN` + ADR `docs/adrs/NNNN`
  → TDD (red/green) → `/code-review high main..HEAD` + `/security-review` → resolve findings → merge+push.
  (See memory `rlng-backlog-execution-program` for the full authorization record.)

## What's DONE this session
- **B2 / increment 018** (`934f1b5`): benchmark-only — `BenchmarkForEachScopeCopy`; scope-copy cost linear
  both axes, sub-ms typical → **accepted, no optimization** (ADR-0043). No prod change.
- **B3 / increment 019** (`2af21f2`): opt-in coercing numeric getters `GetIntCoerce`/`GetInt64Coerce`/
  `GetFloat64Coerce` (`pipe/get.go`); safe/honest per ADR-0035; additive, no SemVer break (ADR-0044).
- **B4 / increment 020 shipped** (`main@4b3089e`): `(*config.PipelineDef).Build` now **rejects** a
  hand-built def carrying a non-JSON-marshalable value (in any `any`-typed field: `Constants`/`Schema`/
  `Globals`) with a `*ConfigError` wrapping the new exported `config.ErrUnhashableDef`, instead of silently
  stamping the `{}` placeholder identity. Version-cleared marshal factored into shared
  `canonicalJSON`/`hashCanonical`. **`Hash()`/`MatchesRuleset` signatures unchanged** (direct-`Hash()`
  fallback retained + documented); existing hashes byte-identical (golden test green); parse paths
  unaffected. Spec/plan 020, ADR-0045; BACKLOG B4 → resolved. Commits `311b7db`(spec)→`4b3089e`(fix).
  Code-review: no findings; security-review: clean.

## Next action — B5 (increment 021): IMPLEMENT the approved design (Option A)
**Design checkpoint is CLEARED** — the user approved **Option A** and it is captured in
**`docs/specs/021-per-decision-options.md`** (already on `main`, commit `32978f7`). **Read spec 021 first —
it spells out the whole implementation (D1–D6 + the exact migration surface).** Summary of what to build:
- **`pipe` (breaking):** add `type Decision struct { Expr string; Options []expr.Option }`; change
  `pipe.Rule.Decisions` from `map[string]string` → `map[string]Decision` and **remove** `Rule.DecisionOptions`
  (`pipe/table.go:73`); `compileDecisions` (`pipe/table.go:155`) compiles each `Decisions[key].Expr` with its
  own `.Options` (drop the shared `opts` param); change `WithDefault` (`pipe/options.go:67`) to
  `WithDefault(map[string]Decision)`.
- **`config`:** replace `bareDecisions` (`config/build.go:360`) with a converter that builds
  `map[string]pipe.Decision` threading each `ExprDef`'s options —
  `Options: withStrictEnv(strict, schema, withConstants(constants, ed.options()))` — and **delete** the
  `hasOptions()` rejection; update the two call sites (`build.go:272` rule decisions, `:288/:292` defaults).
- **Migrate ~8 test files** that build `pipe.Rule{Decisions: map[string]string{…}}` or
  `WithDefault(map[string]string{…}, …)`: `pipe/{firing_test,firing_example_test,table_example_test,`
  `foreach_test,json_test,table_edges_test,table_policies_test,table_test}.go` (grep
  `Decisions\|WithDefault\|DecisionOptions`). Bare decisions → `map[string]Decision{"k": {Expr: "…"}}`.
- **TDD the new behavior** (spec 021 success criteria 1–6): per-decision fallback on one output not another;
  per-decision globals/coerce honored; default-decision options; bare regression; config builds instead of
  rejecting; constants/schema env still reaches every decision. `Hash()` of an unchanged ruleset stays
  byte-identical (golden test) — the parsed `PipelineDef` shape is untouched.

Start: `git checkout -b claude/per-decision-options-021 main`, then plan 021 + ADR-0046 (records Option A +
the breaking change; supersedes ADR-0007 §5's rejection) ride with the feat commit. Then full gate
(`/code-review` + `/security-review` over `main..HEAD`, `-race`, coverage) → auto merge+push (standing
authorization). **Note:** this is a breaking pre-1.0 API change — flag the commit subject and note `apidiff`.

## Exact state
- On `main`, clean, synced with `origin/main` at `32978f7` (= B4 merge `4b3089e` + handover + **spec 021**,
  a standalone spec commit; **no B5 code yet**). `git status --short` = empty.
- Full gate green as of B4 merge: `go build ./...`, `go test ./... -race`, `go vet`, `gofmt -l .`,
  `CGO_ENABLED=0 go build`, `go mod tidy`(no-op)/`verify` all clean. `config` coverage 99.8%.
  `benchstat` installed at `$(go env GOPATH)/bin/benchstat`.
- **B5 handed over at the post-design safepoint** (spec approved & committed, no implementation started) to
  avoid running the large breaking-change increment past a safe context budget mid-edit.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch (016's ledger); not used for the 017+ program — track via
  `docs/BACKLOG.md` + git log instead.
- Artifact numbering is continuous: **spec 021 DONE**, plans at 020 done, ADRs at 0045 done. **B5 = plan 021 + ADR-0046 next (spec 021 already on main).**
