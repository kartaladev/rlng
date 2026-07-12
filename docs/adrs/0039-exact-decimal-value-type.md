# ADR-0039 — Exact-decimal value type

- **Status:** Accepted
- **Date:** 2026-07-13
- **Prompted by:** Spec 014 (`docs/specs/014-value-serde-consistency.md`), Plan
  014 (`docs/plans/014-value-serde-consistency.md`), Task 1. Realizes G4 of
  Spec 014, under the umbrella of ADR-0038 (value serde consistency).

## Context

The audit's motivating example: `principal * rate` for a $250,000 loan at
7.25% must be exactly `$18,125.00`. Plain `float64` arithmetic
(`250000.0 * 0.0725`) lands at `18124.999999999996` — wrong for reconciliation,
and unfixable by scaling alone because the error comes from the multiplication
itself, not from decimal-string parsing.

ADR-0030 recorded an interim "integer minor units" approach (store money as
integer cents) and explicitly deferred exact-decimal support. That interim
only holds for pure addition/subtraction of pre-scaled amounts; it breaks the
instant a **rate** (a genuine non-integer fraction, not a scaled amount)
appears inside an expression — which is the common case for fee/interest
calculations, not an edge case. rlng needs a value type that stays exact
through multiplication and division by a rate, not just integer addition.

The engine's core quality criterion is **debuggability**: pure Go, no cgo, so
a developer can step through evaluation with a normal debugger. Any decimal
solution must honor that constraint, and the project's dependency-minimalism
rule (`CLAUDE.md`, "Library quality gates") means a new dependency needs
explicit justification.

## Decision

**D1 — Depend on `github.com/shopspring/decimal` v1.4.0.** Rather than build
an in-house decimal type, rlng takes `shopspring/decimal` as a new direct
dependency: it is pure Go (`big.Int`-backed, no cgo — confirmed via
`go list -deps` showing zero cgo files and `CGO_ENABLED=0 go build ./...`
succeeding), it is the de-facto standard decimal library in the Go ecosystem,
and it ships the rounding modes needed (including half-even/banker's
rounding) so rlng does not have to reimplement them. This is the one new
dependency Spec 014 budgets for (D1 of the spec's resolved decisions); nothing
else is added. The canonical Go type across the engine is
`github.com/shopspring/decimal`.`Decimal`, used directly — not wrapped in a
project-local type — so callers can use the library's own API (`Round`,
`RoundBank`, `StringFixed`, etc.) without an extra indirection layer.

**D2 — Surface: a `decimal()` builtin, arithmetic operator overloads, and
rounding builtins, wired into every compiled expression unconditionally.**
`expr/decimal.go` adds `decimalExprOptions()`, a set of `expr-lang` options
appended inside `buildExprOpts` (`expr/options.go`) so decimal support is
available in every `expr.NewFunction`/`expr.NewPredicate` with no extra
per-call option:

- `decimal(x)` — constructs a `decimal.Decimal` from a `string` (parsed
  exactly via `decimal.NewFromString`), `int`, `int64`, `float64`, or an
  existing `decimal.Decimal` (pass-through). An unparseable string, or any
  other Go type reaching the underlying `toDecimal` conversion, is a
  **typed eval-time error** (`decimal: unsupported type %T` / the underlying
  parse error) — consistent with the project's debuggability principle of
  surfaced, not silent, failures.
- Arithmetic operator overloads for `+ - * /` covering `decimal×decimal`,
  `decimal×int`, and `int×decimal` operand combinations (registered via
  `exprlang.Operator` mapped to `addDecimal`/`subDecimal`/`mulDecimal`/
  `divDecimal` host functions). Native `int`/`float64` arithmetic between two
  non-decimal operands is untouched — decimal is additive, not a rewrite of
  expr's evaluation model (Spec 014 non-goal).
- `round(x, places)` (half-away-from-zero, `decimal.Decimal.Round`) and
  `roundBank(x, places)` (half-even/banker's, `decimal.Decimal.RoundBank`),
  both accepting anything `toDecimal` accepts as their first argument.
- **Reserved identifier names:** `decimal`, `round`, `roundBank`. An
  expression author cannot declare a variable or config field with one of
  these names without shadowing the builtin; this is a deliberate, documented
  trade-off in exchange for these being available with no opt-in ceremony.

**D3 — Operator overloading resolves at COMPILE time by static operand
type; document both the universal path and its ergonomic-but-conditional
counterpart.** `expr-lang`'s operator overload mechanism rewrites an operator
node into the corresponding host-function call only when the checker can
statically determine that at least one operand is a `decimal.Decimal` at
compile time:

- `decimal("0.1") + decimal("0.2")` resolves in **default** (lenient,
  `AllowUndefinedVariables`) mode, because the `decimal(...)` builtin's
  declared return-type hint makes the operand statically `decimal.Decimal` —
  this is the **universal, always-works path**: wrap any operand you want to
  do exact arithmetic with in `decimal(...)`.
- Bare-variable arithmetic over *undefined* identifiers whose runtime value
  happens to be `decimal.Decimal` (e.g. `principal * rate` where `principal`
  and `rate` are never declared to the compiler) does **not** resolve in
  lenient mode — the checker cannot know their type, so it falls back to a
  native operator that fails at runtime once it sees a non-numeric Go type.
  This path only resolves under **strict env mode** (`expr.WithEnv`, from
  Spec 011/ADR-0031), where the declared environment tells the checker
  `principal` and `rate` are `decimal.Decimal`, letting it route through the
  overload.

  Both paths are empirically verified (`expr/decimal_test.go`:
  `TestDecimalArithmetic` covers the wrapped/lenient path;
  `TestDecimalStrictEnvBareArithmetic` covers the strict-env bare-variable
  path) and are not to be "fixed" into each other — they are two genuinely
  different resolution timings inherent to how `expr-lang` overloads work,
  and both are documented so a caller picks the right one deliberately rather
  than being surprised by a runtime `invalid operation` error.

**D4 — Division uses the library's default `DivisionPrecision` plus an
explicit rounding builtin where an exact scale is required.** `divDecimal`
calls `decimal.Decimal.Div` directly; callers needing a specific fixed scale
call `roundBank(x / y, places)` explicitly rather than the engine silently
picking a scale on their behalf.

## Consequences

- **One new dependency, justified.** `github.com/shopspring/decimal` becomes
  a direct dependency of the module; it pulls no transitive cgo, keeping
  `CGO_ENABLED=0 go build ./...` green and preserving the project's core
  debuggability constraint (a plain Go debugger and readable stack traces
  work through decimal arithmetic exactly as through any other Go code).
- **Money and rate math can now be made exact,** closing the audit's B1
  blocker: `decimal("250000") * decimal("0.0725")` equals exactly
  `decimal.RequireFromString("18125")`, and `roundBank(principal * rate, 2)`
  under strict env yields exactly `"18125.00"` (verified in
  `expr/decimal_test.go`).
- **Decimal is opt-in per-expression, not automatic.** An author must
  explicitly reach for `decimal(...)`/a decimal-typed field; existing
  `int`/`float64` expressions are completely unaffected by this change
  (`go test ./expr/ -race` shows the full pre-existing `expr` suite passing
  unmodified after this addition — no regression to native numeric or
  `WithEnv` strict-mode behavior).
- **Three identifiers are now reserved** (`decimal`, `round`, `roundBank`)
  across every compiled expression in the engine, unconditionally — this is
  documented here and in the `expr/decimal.go` godoc so it is discoverable
  rather than a surprise collision.
- Config-literal decimal declaration (a `!decimal`-tag-equivalent object
  form), aggregation fidelity when a decimal participates in a fold, and
  Scope JSON round-tripping of a `decimal` kind are follow-on tasks of Plan
  014 (ADR-0038 D3/D4) — this ADR covers only the `expr`-level foundation
  (Task 1): the type is usable inside expressions, not yet wired through
  config/aggregation/persistence.
