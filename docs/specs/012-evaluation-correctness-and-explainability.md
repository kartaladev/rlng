# Spec 012 — Evaluation correctness & explainability

- **Status:** Draft (awaiting review)
- **Date:** 2026-07-12
- **Post-010 audit remediation.** Findings embedded inline (no standalone audit
  record).
- **Builds on:** Spec 001 (`expr` predicate/function), Spec 002 (`pipe` Scope),
  Spec 006 (provenance & firing), Spec 010 (hit policies, rule identity, collect).
- **Realized by:** Plan 012.
- **Anticipated ADRs:** ADR-0034 (fallback error/nil semantics — behavior change),
  ADR-0035 (coercion truthiness rules), ADR-0036 (multi-rule firing for
  collect/any).
- **Note:** numeric-fidelity aspects of the audit (`foldNumeric` integer
  accumulation, `HitPolicyAny` value equality) are **out of scope here** and are
  owned by Spec 014 (value serde consistency), since they are boundary/type
  concerns rather than evaluation-logic bugs. This spec covers evaluation *logic*
  and *explainability*.

## Context

The audit confirmed the evaluation core is sound (DAG ordering, concurrency
isolation, panic-safety, determinism all verified) but found four
evaluation-logic defects where a *default* behavior silently produces a wrong or
unexplainable decision.

1. **Collect mode records no firing rule.** `firing.go` documents that a firing
   rule is recorded "for every hit policy when a rule matches, and for the default
   when no rule matched." `executeSingle`/`executeUnique`/`executeAny` honor this,
   but `executeCollect` (`pipe/table.go:333-396`) calls `recordFiring` **only** in
   the no-match/default branch — never on a successful match. So a collect table
   that fires writes its outputs correctly, yet `Scope.FiringRule(stage)` returns
   `(_, false)` and `FiringRules()` omits the stage. This blanks the
   explainability trail for exactly the multi-reason "denied for reasons A, B, C"
   adverse-action shape that collect is *for*. It is also a hot-path branch with
   **zero test coverage** (as is `HitPolicyAny` firing at `table.go:300`).
2. **`HitPolicyAny` firing attribution is lossy.** When several rules agree and
   fire, only `matched[0]`'s ID/message is recorded (`table.go:300`), and
   `writeAgg` stores agreed values with an empty `Expression`/`Inputs`, so lineage
   dead-ends. A multi-rule agreement cannot be fully explained.
3. **Fallback silently swallows genuine evaluation errors.** When a `Function`'s
   main expression *errors* and a fallback is configured, `Apply`
   (`expr/function.go:72-84`) returns `(fallbackResult, nil)` — the triggering
   error (divide-by-zero, a typo'd field, a broken host function) is dropped
   entirely; it survives only if the fallback *also* fails. This directly
   contradicts the project's first-class debuggability mandate: a business user
   sees a plausible fallback number and never learns the rule is broken.
   `function_test.go` currently asserts `require.NoError` on exactly this,
   locking in the masking.
4. **`nil` main-result conflated with failure.** `Apply` (`function.go:80-82`)
   also fires the fallback when the main expression evaluates to `nil`, so a
   Function with a fallback can **never** return `nil`. A legitimate "no discount"
   (`nil`) is silently overwritten by the fallback, indistinguishable from a
   genuine zero.
5. **Lenient truthiness footguns.** In opt-in `WithCoerce` predicates
   (`expr/predicate.go:77-106`): `NaN` coerces to `true` (a rule that should read
   as invalid/false fires); `ParseBool` makes a *non-empty* string such as `"0"`,
   `"f"`, `"F"` coerce to `false` while `"no"`/`"2"` coerce to `true`
   (inconsistent with the documented "true iff non-empty"); and an unhandled type
   (`time.Time`, a non-nil pointer, a struct) silently coerces to `false` instead
   of erroring like the strict path. All are untested.

## Goals

1. **G1 — Explainable collect & any (ADR-0036).** Record firing for collect and
   any on a match, not only on default. Because these policies can fire multiple
   rules, extend firing to represent a **set** of firing rules (e.g. `FiringRules`
   gains multi-rule entries, or a `[]FiringRule` per stage) so every contributing
   rule ID/message is captured for adverse-action reasoning. Preserve per-rule
   provenance for `any` (attach each agreeing rule's expression/inputs). Add the
   missing firing tests for collect and any.
2. **G2 — Fallback surfaces failures, not masks them (ADR-0034).** By default a
   fallback triggers on **error only**, and the triggering error is not silently
   discarded — either propagated when no fallback, or, when a fallback is used,
   made observable (an injected logger/hook or an exposed cause), never dropped
   into a plausible-looking value. The nil-result-triggers-fallback behavior
   becomes **opt-in** (`WithFallbackOnNil`) so `nil` stays a first-class value by
   default. This changes existing behavior → ADR + SemVer note (pre-`v0.1.0`, so
   acceptable; update the offending `function_test.go` assertion).
3. **G3 — Honest, safe coercion (ADR-0035).** Define and document the `WithCoerce`
   truthiness rules precisely and safely: `NaN` (and `±Inf` as decided) → `false`;
   string coercion either strictly boolean or documented with its exact accepted
   set (resolve the `ParseBool`-subset inconsistency); an unhandled type →
   an `EvalError` (like strict mode), not a silent `false`. Cover the full matrix
   in tests.

## Non-goals

- Numeric type/precision fidelity (`foldNumeric`, `HitPolicyAny` numeric equality)
  — owned by Spec 014.
- Removing the fallback feature; it stays, with safer default semantics.
- Changing strict (non-coerce) predicate behavior, which is already correct.

## Hot-path branches (test targets)

- Collect firing: a matching collect table records firing (multi-rule); no-match
  records the default firing; `FiringRules()` includes the stage.
- Any firing: multiple agreeing rules each recorded; per-rule lineage present;
  conflict still errors.
- Fallback: main-error-with-fallback surfaces the cause (not dropped) and returns
  the fallback; main-error-no-fallback returns the `EvalError`; main-`nil`-no-opt
  returns `nil` (fallback NOT fired); `WithFallbackOnNil` fires on `nil`; fallback
  also failing joins both causes.
- Coercion matrix: `NaN`→false, `±Inf`→(decided), `"0"`/`"f"`/`"F"`→(decided,
  consistent), `"true"`/`"false"` as today, empty string→false, non-empty
  non-bool string→true, numeric zero/non-zero, empty/non-empty slice/map, and an
  unhandled type (`time.Time`, non-nil pointer, struct)→`EvalError`.
