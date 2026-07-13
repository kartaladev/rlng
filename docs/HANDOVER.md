# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then `docs/BACKLOG.md`
> (the tracked backlog B1–B12), then `git log` on `main`. For the item you pick up, read its
> `docs/specs/*` + `docs/plans/*` + related ADRs first. Trust the repo (code + git history + `docs/`) over
> any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Increments 011–016 merged & pushed. **Increment 016 (config `Provider`) complete.**

**Active program: execute ALL 12 backlog items (B1–B12) from `docs/BACKLOG.md`,** each as its own
increment. **B1 DONE (incr 017, `main@f10a8be`); B2 DONE (incr 018, `main@934f1b5`).** Next up: **B3.**

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
- **B1 / increment 017** (`main@f10a8be`): `foreach` `Rollup.Key` dot-path-aware via shared `lookupPath`;
  spec/plan 017, ADR-0042. B1 → `docs/BACKLOG.md` Resolved. (Prior session.)
- **B2 / increment 018 shipped** (`main@934f1b5`): a benchmark-only increment — `BenchmarkForEachScopeCopy`
  (`pipe/foreach_bench_test.go`, blackbox) measures `ForEach.Execute`'s per-element scope-copy cost over a
  `scope{flat8,flat64,nested} × elements{1,10,100,1000}` grid, empty inner pipeline, no rollups, provenance
  off. Result: **linear in both axes, sub-ms for typical counts** (flat64×100 = 548µs), ~5ms only at the
  1000×64 extreme; `nested` +6 allocs/element pinpoints `cloneValue`'s per-node deep copy. **Decision:
  accepted, no optimization** (the copy is the price of the per-element isolation invariant, ADR-0040 D2).
  **No production-code change.** Spec/plan 018, ADR-0043; ADR-0040 watch-item + BACKLOG B2 → resolved.
  Commits `ca3f10e`(spec)→`934f1b5`(test). Code-review found one methodology-comment inaccuracy (fixed,
  amended in); security-review clean.

## Next action — B3 (increment 019)
**B3 = Numeric-coercing Scope getters** (`docs/BACKLOG.md` B3; source Spec 006 non-goal, `pipe/get.go`).
**Additive, autonomous** (design + implement autonomously, surface design via the committed spec; no live
approval pause — it is in the B1–B4/B10 autonomous set). Today typed getters (`GetInt`/`GetFloat64`/… in
`pipe/get.go`) are **strict**: a `float64` at an int path or a numeric string is a `*ScopeTypeError`. Add a
**coercing variant** (e.g. `GetIntCoerce`/… or an option) without breaking the strict API. Start:
`git checkout -b claude/coercing-scope-getters-019 main`, read `pipe/get.go` + `get_test.go` +
`cc-skills-golang:golang-safety` (numeric conversion overflow), brainstorm → spec/plan 019 → ADR-0044 →
TDD (cover every coercion/error branch — hot path) → gate → auto merge+push.

## Exact state
- On `main`, clean, synced with `origin/main` at `934f1b5`. `git status --short` = empty.
- Full gate green as of B2 merge: `go build ./...`, `go test ./... -race`, `go vet`, `gofmt -l .`,
  `CGO_ENABLED=0 go build`, `go mod tidy`(no-op)/`verify` all clean. `benchstat` now installed at
  `$(go env GOPATH)/bin/benchstat`.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch (016's ledger); not used for the 017+ program — track via
  `docs/BACKLOG.md` + git log instead.
- Artifact numbering is continuous: specs/plans at 018 done, ADRs at 0043 done. **B3 = spec/plan 019, ADR-0044.**
