# Plan 008 — Production-hardening pass

- **Implements:** Spec 008 (docs/specs/008-production-hardening.md)
- **Depends on ADRs:** 0015 (scope isolation/paths), 0016 (expr recursion/fallback), 0017 (config strictness/attribution), 0018 (engine/mapper).
- **Branch:** `fix/production-hardening` off `main`.
- **Method:** TDD (red → green) per fix; every new typed-error/hot-path branch gets a covering table-test case. Full `-race` gate + `/code-review` + `/security-review` over `main..HEAD` before delivery.

## Task 1 — Stage cluster (`stage/scope.go`, `stage/timing.go`)  [ADR-0015]
- **B1** deep-copy nested `map[string]any` spine in `NewScope`; `cloneMaps` helper. Test: shared nested-map input isolated (mutating scope doesn't touch seed); `-race` test of concurrent `Evaluate`.
- **M4** `Set` rejects empty segments → new `ErrEmptyPathSegment`. Branches: empty first/middle/last segment.
- **M5** *(declined during review)* `NewScope` stores seed keys verbatim (deep-copied); nesting dotted keys was rejected — it risks non-deterministic prefix-collision data loss. Branch: `{"a", "a.b"}` both survive deterministically.
- **M7** `Set` lazy-inits `s.data`. Branch: `(&Scope{}).Set(...)` no panic.

## Task 2 — Expr cluster (`expr/env.go`, `expr/function.go`, `expr/refs.go`)  [ADR-0016]
- **B2** depth-bounded `convertValue`/`structToMap` → `maxEnvDepth`; `toEnv` returns `*EvalError` when exceeded. Branches: within-bound struct ok; over-bound/cyclic → error not crash.
- **M1** compile fallback with `mainOpts`. Branch: fallback result coerced to return kind.
- **M9** `errors.Join` main error into fallback error. Branches: main-error→fallback-ok (cause preserved); main-error→fallback-error (both preserved).
- **L4** `refVisitor` skips call-callee identifiers. Branch: `discount(price)` → refs excludes `discount`.

## Task 3 — Config cluster (`config/parse.go`, `config/build.go`)  [ADR-0017]
- **M2** strict decoders (`KnownFields`/`DisallowUnknownFields`). Branches: unknown key YAML/JSON → error; `hit_policy` typo rejected.
- **M8** don't re-prefix a `*stage.StageError`; set `Field: "condition"` for the condition path. Branches: single stage-error passthrough; bad condition attributed to field.
- **L5** `Build()` rejects zero-stage def. Branch: empty def → ConfigError.
- **Simplify** `buildTable` uses a direct field check instead of `ed.options()` double-alloc.

## Task 4 — Root cluster (`mapper.go`, `engine.go`, `errors.go`)  [ADR-0018]
- **M3** `setNested` returns collision error; `Map` wraps as `*MappingError`. Branches: prefix collision leaf-then-map; map-then-leaf.
- **M6** `flatten` nil-input guard. Branches: nil pointer → error; nil interface → error; empty map ok.
- **R5** distinguish final-decode `MappingError`; `NewMapper` rejects empty key. Branches: empty template key rejected.

## Task 5 — Low cluster (`stage/provenance.go`, `stage/table.go`)  [Spec 008 §Low]
- **L1** truncation signalled (bool/flag or `Truncated` accessor). Branch: chain > bound reports truncated.
- **L2** index derivations by namespace for `derivationsFor`. Branch: parity with current output; perf note only.
- **L3** collect-mode records per-rule `Inputs` faithfully. Branch: two matched rules referencing same id → both recorded.
- **Simplify** dedup `executeSingle`/`executeCollect` shared scaffolding.

## Task 6 — Docs & gate
- Fix `stage/doc.go` "concurrency-safe" wording; document `LoadFile` trust boundary.
- `go test ./... -race -cover`, `vet`, `gofmt`, `golangci-lint`; `/code-review` + `/security-review` over `main..HEAD`; update `docs/HANDOVER.md`.
