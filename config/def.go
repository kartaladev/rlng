package config

// PipelineDef is a declarative pipeline: an ordered list of stages. List order
// is authoring order, which Build preserves into pipe.NewPipeline so the
// pipeline's deterministic tie-break ordering is reproducible.
//
// Constants are pipeline-level named values injected as compile-time defaults
// (via the variable patcher) into every stage expression, so a threshold or
// label is declared once and referenced by name; runtime input overrides them.
//
// Mapping is an optional output template (output dot-path -> expression) that a
// consumer passes to rlng.NewMapper to project the final Scope into a typed
// result. Build does not consume it (mapping lives above the pipeline); it is
// parsed and exposed here so a whole decision service is one document.
type PipelineDef struct {
	Constants map[string]any    `yaml:"constants" json:"constants"`
	Stages    []StageDef        `yaml:"stages" json:"stages"`
	Mapping   map[string]string `yaml:"mapping" json:"mapping"`
}

// StageDef is a flat union over the three stage types, selected by Type. Fields
// not relevant to Type are ignored by Build.
type StageDef struct {
	Name      string   `yaml:"name" json:"name"`
	Type      string   `yaml:"type" json:"type"` // single-expr | multi-expr | decision-table
	DependsOn []string `yaml:"depends_on" json:"depends_on"`

	// single-expr
	Expr      *ExprDef `yaml:"expr" json:"expr"`
	Condition *ExprDef `yaml:"condition" json:"condition"`
	Output    string   `yaml:"output" json:"output"`

	// multi-expr
	Exprs []NamedExprDef `yaml:"exprs" json:"exprs"`

	// decision-table
	HitPolicy   string             `yaml:"hit_policy" json:"hit_policy"`   // single (default) | collect | unique | any
	Aggregation string             `yaml:"aggregation" json:"aggregation"` // collect only: list (default) | sum | min | max | count
	Rules       []RuleDef          `yaml:"rules" json:"rules"`
	Default     map[string]ExprDef `yaml:"default" json:"default"` // else decisions applied when no rule matches
}

// NamedExprDef is one entry of a multi-expr stage.
type NamedExprDef struct {
	Name     string  `yaml:"name" json:"name"`
	Priority int     `yaml:"priority" json:"priority"`
	Expr     ExprDef `yaml:"expr" json:"expr"`
}

// RuleDef is one rule of a decision-table stage. ID and Message are optional
// metadata that make a firing rule identifiable (see pipe.FiringRule).
type RuleDef struct {
	ID        string             `yaml:"id" json:"id"`
	Message   string             `yaml:"message" json:"message"`
	Condition ExprDef            `yaml:"condition" json:"condition"`
	Decisions map[string]ExprDef `yaml:"decisions" json:"decisions"`
}
