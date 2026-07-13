// Package pipe provides rlng's Scope accumulator, the stage types that compose
// the expr evaluators (github.com/kartaladev/rlng/expr) into reusable rule and
// calculation units, and the Pipeline that runs them in dependency order.
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
// A Pipeline runs its stages sequentially in dependency order by default
// (deterministic and debuggable). The WithConcurrency / WithMaxParallel options
// opt into running independent stages of each dependency level concurrently;
// this is a pure speedup — the final Scope, the surfaced error, and the reported
// stage order are identical to sequential execution (ADR-0052).
//
// A Pipeline configured with WithRuleset(RulesetIdentity) records which ruleset
// produced each Scope it runs (Scope.Ruleset()); combined with the
// decision-table firing trail (FiringRule/FiringRulesFor), a Scope round-trips
// through its JSON MarshalJSON/UnmarshalJSON as a self-describing, replayable
// decision record.
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
package pipe
