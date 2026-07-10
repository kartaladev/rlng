# ADR-0004 — Stage option convention

- **Status:** Accepted
- **Date:** 2026-07-11
- **Prompted by:** Spec 002 (docs/specs/002-scope-and-stages.md) + whole-branch
  review of increment 2

## Context

Increment 2 originally gave each stage constructor its own option type —
one per stage (`SingleExpr`, `MultiExpr`, `DecisionTable`) — with duplicated
or near-duplicated `With*` dependency-declaration functions, each carrying a
stage-specific prefix to avoid colliding across the three distinct option
types. This is the shape Go's lack of function overloading forces when each
constructor's option type is distinct, but it fragments what is conceptually
a single small set of stage knobs (dependencies, output, gating, hit policy)
into three parallel APIs, and it drifted from the shared `expr.Option`
pattern already established in increment 1 (`expr/options.go`), where a
single `Option` type is used by both `Predicate` and `Function`, and each
constructor simply ignores the options that do not apply to it.

Separately, the whole-branch review of increment 2 (review Important #1)
flagged that `NewMultiExpr` and `NewDecisionTable` silently accepted an empty
stage name and wrote into a `""` top-level namespace via `Set("."+key, ...)`,
while `NewSingleExpr` only failed late, at `Execute`. This needed a
consistent, early validation fix across all three constructors regardless of
the option-type decision.

The library is pre-release (no tagged version yet), so an exported-API
convention change here has no SemVer cost — this is the cheapest point to
settle it.

## Decision

1. **A single shared `stage.Option` type**, defined in `stage/options.go`,
   used by all three stage constructors — mirroring `expr.Option`:

   ```go
   type Option func(*stageConfig)

   func WithDependsOn(deps ...string) Option   // all stage types
   func WithOutput(path string) Option         // SingleExpr only; ignored elsewhere
   func WithCondition(condition string, opts ...expr.Option) Option // SingleExpr only
   func WithExprOptions(opts ...expr.Option) Option                 // SingleExpr only
   func WithHitPolicy(h HitPolicy) Option      // DecisionTable only; ignored elsewhere
   ```

   Option names are unprefixed (a single `WithDependsOn`, not one
   stage-prefixed variant per stage type); an option that does not apply to a
   given stage type is silently ignored, documented per option. The old
   per-stage option types and their duplicated, stage-prefixed `With*`
   dependency functions are removed. `NamedExpr.Options` and
   `Rule.ConditionOptions`/`Rule.DecisionOptions` remain unchanged — those are
   constructor **data** (per-expression `expr.Option`s), not stage options,
   and are out of scope for this ADR.

2. **Hit-policy naming aligned to the DMN domain term.** `type HitPolicy int`
   keeps its name, but its constants and setter are renamed to say what they
   are: the two hit-policy constants become `HitPolicySingle`/
   `HitPolicyCollect` (`HitPolicySingle` remains the iota-0 default), and the
   setter option becomes `WithHitPolicy`. The previous names used a generic
   "mode" term already used elsewhere in the codebase's vocabulary (e.g.
   `Scope`'s strict/lenient mode); "hit policy" is the DMN (Decision Model and
   Notation) term this stage's semantics are modeled on, and naming it
   explicitly avoids confusing DecisionTable's hit policy with unrelated
   modes.

3. **Empty stage-name validation.** `NewSingleExpr`, `NewMultiExpr`, and
   `NewDecisionTable` now reject `name == ""` up front, before compiling
   anything, returning `&StageError{Stage: name, Type: <TypeConst>, Cause:
   errEmptyStageName}`. This closes the silent `""`-namespace write and the
   late-failure-at-Execute gap the review flagged, uniformly across all three
   constructors.

## Consequences

- One option surface to learn and document instead of three; adding a new
  cross-cutting option (e.g. a future `WithTimeout`) is a single addition
  instead of three.
- Options are silently ignored when inapplicable to a given stage type —
  callers get no compile-time or runtime signal if they pass, say,
  `WithHitPolicy` to `NewSingleExpr`. This trades a small footgun for API
  minimalism; each option's godoc states which stage types it applies to,
  and stage-level example tests demonstrate correct usage.
- `stageConfig` is unexported, so this is purely a public-API shape change;
  no internal state leaks.
- Renaming the hit-policy constants and setter after they were introduced in
  ADR-0003 is a breaking rename, but the library has not been tagged, so
  there is no SemVer consumer to break. ADR-0003 is updated in the same
  change to keep it consistent with the code rather than superseded, since
  the underlying hit-policy semantics are unchanged — only names moved.
- All three `New*` constructors now share the same empty-name failure mode,
  so callers can rely on `errors.As(err, &StageError{})` uniformly instead of
  discovering a `MultiExpr`/`DecisionTable` empty-name bug only once
  `Execute` runs.
