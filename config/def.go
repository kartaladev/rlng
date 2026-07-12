package config

// PipelineDef is a declarative pipeline: an ordered list of stages. List order
// is authoring order, which Build preserves into pipe.NewPipeline so the
// pipeline's deterministic tie-break ordering is reproducible.
type PipelineDef struct {
	Stages []StageDef `yaml:"stages" json:"stages"`
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
	HitPolicy string    `yaml:"hit_policy" json:"hit_policy"` // single (default) | collect
	Rules     []RuleDef `yaml:"rules" json:"rules"`
}

// NamedExprDef is one entry of a multi-expr stage.
type NamedExprDef struct {
	Name     string  `yaml:"name" json:"name"`
	Priority int     `yaml:"priority" json:"priority"`
	Expr     ExprDef `yaml:"expr" json:"expr"`
}

// RuleDef is one rule of a decision-table stage.
type RuleDef struct {
	Condition ExprDef            `yaml:"condition" json:"condition"`
	Decisions map[string]ExprDef `yaml:"decisions" json:"decisions"`
}
