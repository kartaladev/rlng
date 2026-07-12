package pipe

import "github.com/kartaladev/rlng/expr"

// Option configures a stage. A single Option type is shared across all stage
// constructors (as expr.Option is shared across Predicate and Function); an
// option that does not apply to a given stage type is ignored — documented per
// option below.
type Option func(*stageConfig)

type stageConfig struct {
	deps      []string
	output    string
	hasOutput bool
	condition string
	condOpts  []expr.Option
	exprOpts  []expr.Option
	hitPolicy HitPolicy
}

func newStageConfig(opts []Option) *stageConfig {
	c := &stageConfig{hitPolicy: HitPolicySingle}
	for _, o := range opts {
		o(c)
	}
	return c
}

// WithDependsOn declares the stages this stage depends on (all stage types;
// consumed by the DAG orchestrator in a later increment).
func WithDependsOn(deps ...string) Option { return func(c *stageConfig) { c.deps = deps } }

// WithOutput sets the Scope path a SingleExpr writes its result to (default: the
// stage name). Ignored by MultiExpr and DecisionTable.
func WithOutput(path string) Option {
	return func(c *stageConfig) { c.output = path; c.hasOutput = true }
}

// WithCondition gates a SingleExpr on a boolean predicate; when it tests false
// the stage writes nothing. Ignored by MultiExpr and DecisionTable.
func WithCondition(condition string, opts ...expr.Option) Option {
	return func(c *stageConfig) { c.condition = condition; c.condOpts = opts }
}

// WithExprOptions passes options to a SingleExpr's value expression (e.g.
// expr.WithFallback, expr.WithGlobals). Ignored by MultiExpr and DecisionTable.
func WithExprOptions(opts ...expr.Option) Option { return func(c *stageConfig) { c.exprOpts = opts } }

// WithHitPolicy sets a DecisionTable's hit policy (default HitPolicySingle).
// Ignored by SingleExpr and MultiExpr.
func WithHitPolicy(h HitPolicy) Option { return func(c *stageConfig) { c.hitPolicy = h } }
