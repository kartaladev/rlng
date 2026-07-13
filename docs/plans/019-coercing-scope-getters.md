# Plan 019 — numeric-coercing Scope getters (B3)

- **Implements:** Spec 019 (`docs/specs/019-coercing-scope-getters.md`).
- **Records:** ADR-0044 (rides in Task 1's commit); aligns with ADR-0035.
- **Backlog:** graduates B3 (`docs/BACKLOG.md`).

One task, TDD red→green→refactor. Additive only — no strict-getter change. Standing program
authorization: commit the green task, run the whole-branch gate, auto merge+push.

## Task 1 — `GetIntCoerce` / `GetInt64Coerce` / `GetFloat64Coerce` (feature, TDD)

- [x] **Step 1 (red):** Extend the `pipe/get_test.go` table (external `package pipe_test`) with cases for
  every Spec 019 D3/D4 branch, driving the new methods through the public API. **Hot-path branches from the
  success criteria:** (1) int/uint-kind → int/int64, incl. `uint64` fits and `uint64` overflows → error;
  (2) integral `float64` → int; non-integral `float64` → error; `NaN`/`±Inf` float → error; out-of-range
  float → error; (3) integer `json.Number` → int/int64; non-integer `json.Number` → error (int targets);
  (4) numeric `string` → int/int64/float64; non-numeric `string` → error; `"NaN"`/`"Inf"` string → error
  (float target); (5) int/uint-kind → float64; `json.Number` → float64; `float64` pass-through incl. stored
  `NaN` returned; (6) `bool`/`nil`/slice/map → `*ScopeTypeError` per getter; missing path →
  `ErrPathNotFound`. Assert `*ScopeTypeError` `Expected`/`Actual` for representative error cases.
- [x] **Step 2 (green):** Implement the three methods in `pipe/get.go`. Factor the shared logic into small
  unexported helpers — an integer coercion (`coerceToInt64(v) (int64, ok/err)`, handling int/uint kinds
  overflow-checked, integral-float, `json.Number`, string) with `GetIntCoerce` range-checking its result to
  `int` (reusing the existing int64→int overflow guard), and a float coercion for `GetFloat64Coerce`. Use
  comma-ok type switches / `reflect` over int-uint kinds (mirroring `table.go`'s `toInt64`/`toFloat64`
  kind-switch but **error-returning and overflow-checked**, per golang-safety). No change to any strict
  getter or to `Scope`.
- [x] **Step 3 — docs & ADR:** Godoc each new method with its exact coercion matrix + fail-loud rules
  (Spec 019 D6). Author **ADR-0044** (Nygard: context = strict getters force a hand-written switch at
  loosely-typed edges; decision = three additive coercing methods with the safe/honest matrix, reusing
  `*ScopeTypeError`, aligned with ADR-0035; consequences = additive, no strict change, precision note for
  int→float). Move **B3** to `docs/BACKLOG.md`'s Resolved section (closing increment 019 / ADR-0044).
- [x] **Step 4 — verify (full library gate):** `go build ./...`, `go test ./... -race` (green),
  `go vet ./...`, `gofmt -l .` (empty), `CGO_ENABLED=0 go build ./...`, `go mod tidy` (no-op) / `go mod
  verify`; `pipe` coverage ≥ 85% and **every** new coercion/error branch covered.
- [x] **Step 5 — commit:** `feat(pipe): add coercing numeric Scope getters (B3)` (Backlog B3, Spec 019,
  Plan 019, ADR 0044).

## Whole-branch gate

`/code-review high main..HEAD` + `/security-review`; resolve/triage findings; confirm the full green gate;
then auto merge+push + delete branch (standing program authorization), and start the next backlog item (B4).
