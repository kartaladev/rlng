// Package stage provides rlng's Scope accumulator and the stage types that
// compose the expr evaluators (github.com/kartaladev/rlng/expr) into reusable
// rule and calculation units.
//
// Scope is a concurrency-safe map[string]any accumulator addressed by
// dot-separated paths (Set/Get), with a decoupled Snapshot that serves as the
// expression evaluation environment.
//
// A Stage is Name/Type/DependsOn/Execute. Three implementations are provided:
// SingleExpr (one value expression with an optional condition gate), MultiExpr
// (several named expressions in priority order, each visible to later ones), and
// DecisionTable (ordered condition+decisions rules with HitPolicySingle
// first-match or HitPolicyCollect accumulation). Stages compile at
// construction and only evaluate
// in Execute; failures are a *StageError that unwraps to the underlying expr
// error. Stages declare DependsOn but do not order themselves — dependency-DAG
// orchestration is a later increment.
package stage
