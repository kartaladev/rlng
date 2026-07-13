# HANDOVER — rlng (updated 2026-07-13)

> **To the next session (READ FIRST, trust these over any memory):** `CLAUDE.md`, then `docs/BACKLOG.md`
> (the tracked backlog B1–B12), then `git log` on `main`. For the item you pick up, read its
> `docs/specs/*` + `docs/plans/*` + related ADRs first. Trust the repo (code + git history + `docs/`) over
> any recollection.

## Objective & roadmap position

`rlng` is a pure-Go rule + calculation engine on `expr-lang/expr` (debuggability-first, no cgo, typed
wrapping errors). Increments 011–016 merged & pushed. **Increment 016 (config `Provider`) complete.**

**Active program: execute ALL 12 backlog items (B1–B12) from `docs/BACKLOG.md`,** each as its own
increment. **B1 DONE (incr 017, `f10a8be`); B2 DONE (incr 018, `934f1b5`); B3 DONE (incr 019, `2af21f2`).**
Next up: **B4.**

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
- **B2 / increment 018** (`main@934f1b5`): benchmark-only — `BenchmarkForEachScopeCopy` measures
  `ForEach.Execute`'s per-element scope-copy cost (linear both axes, sub-ms typical, ~5ms at 1000×64);
  **accepted, no optimization** (ADR-0043). No production-code change.
- **B3 / increment 019 shipped** (`main@2af21f2`): added opt-in coercing numeric getters
  `GetIntCoerce`/`GetInt64Coerce`/`GetFloat64Coerce` (`pipe/get.go`) beside the strict getters. Accept a
  wider type set — any integer kind (overflow-checked), integral finite floats, `json.Number`, base-10
  numeric strings — converted safely/honestly per **ADR-0035**: no silent truncation/wrap, never
  manufacture `NaN`/`±Inf` from text (string OR `json.Number`; a value already stored as a float passes
  through), fail loud with `*ScopeTypeError`. Two shared helpers `coerceToInt64`/`coerceToFloat64` (stdlib
  only). **Additive, no SemVer break; strict getters unchanged.** Spec/plan 019, ADR-0044; BACKLOG B3 →
  resolved. Commits `28defd9`(spec)→`2af21f2`(feat). Code-review found one inconsistency (`json.Number`
  non-finite handling — fixed, covered, amended in); security-review clean.

## Next action — B4 (increment 020)
**B4 = `Hash()` rejects non-marshalable hand-built defs** (`docs/BACKLOG.md` B4; source ADR-0037).
**Contained edge case, autonomous** (B1–B4/B10 autonomous set — surface design via the committed spec, no
live approval pause). A hand-built `config.PipelineDef` carrying a non-JSON-marshalable value (`chan`/`func`)
currently falls back to a stable placeholder hash and **silently loses change-detection**. Parse paths can
never produce such values, so this affects only defs built by hand in Go. Design: have `Hash()` (or its
construction path) **validate/reject** the non-marshalable case with a typed error instead of the silent
placeholder. Start: `git checkout -b claude/hash-nonmarshalable-020 main`, read the `Hash()` impl + ADR-0037
+ the `config` hashing code and tests, brainstorm → spec/plan 020 → ADR-0045 → TDD (cover the reject branch
+ the existing marshalable path stays green) → gate → auto merge+push. **Note:** confirm whether changing
the silent-fallback to a hard error is a behavior/contract change worth calling out (it changes `Hash()`'s
error surface); if it adds/returns a new error, document it and keep it additive where possible.

## Exact state
- On `main`, clean, synced with `origin/main` at `2af21f2`. `git status --short` = empty.
- Full gate green as of B3 merge: `go build ./...`, `go test ./... -race`, `go vet`, `gofmt -l .`,
  `CGO_ENABLED=0 go build`, `go mod tidy`(no-op)/`verify` all clean. `pipe` coverage 99.0%.
  `benchstat` installed at `$(go env GOPATH)/bin/benchstat`.

## Gotchas / environment
- `govulncheck`/`golangci-lint` are NOT installed locally — CI runs them on push.
- `.superpowers/sdd/*` are gitignored scratch (016's ledger); not used for the 017+ program — track via
  `docs/BACKLOG.md` + git log instead.
- Artifact numbering is continuous: specs/plans at 019 done, ADRs at 0044 done. **B4 = spec/plan 020, ADR-0045.**
