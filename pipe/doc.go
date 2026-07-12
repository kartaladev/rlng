// Package stage provides rlng's Scope accumulator and the stage types that
// compose the expr evaluators (github.com/kartaladev/rlng/expr) into reusable
// rule and calculation units.
//
// Scope is a mutex-guarded map[string]any accumulator addressed by dot-separated
// paths (Set/Get), with a decoupled Snapshot that serves as the expression
// evaluation environment. Its Set/Get/Snapshot operations are safe for
// concurrent use; note, however, that Get and Snapshot return live references to
// nested maps/slices, so a caller that shares one Scope across goroutines must
// not read a returned nested value concurrently with a Set that writes into it
// (see the Get/Snapshot docs). Engines give each evaluation its own Scope, so
// concurrent Engine use is safe.
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
