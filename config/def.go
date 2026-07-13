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

	// Schema declares the input shape (field name -> a representative value
	// giving its type). When present, every stage expression compiles strictly
	// against it (expr.WithEnv): a field typo is a Build-time error instead of a
	// silent nil. Absent, compilation stays lenient (undefined vars allowed).
	Schema map[string]any `yaml:"schema" json:"schema"`

	// Version is an optional author-declared release label (e.g. "v2.3.1"). It
	// names WHICH release a ruleset is, distinct from Hash() which fingerprints
	// WHAT it contains — so Version is deliberately excluded from the content
	// hash. Empty is valid; it is not an error.
	Version string `yaml:"version" json:"version"`
}

// StageDef is a flat union over the four stage types, selected by Type. Fields
// not relevant to Type are ignored by Build.
type StageDef struct {
	Name      string   `yaml:"name" json:"name"`
	Type      string   `yaml:"type" json:"type"` // single-expr | multi-expr | decision-table | foreach
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

	// foreach: Collection is the Scope path to a []any; As names the
	// per-element binding (default "item", see pipe.WithForEachAs); Stages are
	// the inner sub-pipeline's stage definitions, built and topologically
	// sorted the same way as the top-level pipeline (an inner stage may itself
	// be a foreach — nesting is supported; each foreach in a nesting chain must
	// bind its element under a distinct `as`, see ErrForEachAsCollision, and
	// nested fan-out is multiplicative in the collection sizes); Rollups reduce
	// a per-element output key across all elements into a header value. Output
	// (shared with single-expr, above) names the per-element results list key
	// (default "items", see pipe.WithForEachOutput).
	//
	// These four carry `omitempty` on the JSON tag (unlike the older fields
	// above) so a stage that uses none of them serializes exactly as it did
	// before foreach existed — keeping Hash() stable for pre-foreach rulesets
	// so a persisted decision still MatchesRuleset across the upgrade.
	Collection string      `yaml:"collection" json:"collection,omitempty"`
	As         string      `yaml:"as" json:"as,omitempty"`
	Stages     []StageDef  `yaml:"stages" json:"stages,omitempty"`
	Rollups    []RollupDef `yaml:"rollups" json:"rollups,omitempty"`
}

// RollupDef declares one foreach roll-up: Key is the per-element output key
// to reduce, Agg is the aggregation name (list (default) | sum | min | max |
// count — the same vocabulary as StageDef.Aggregation), and As is the header
// key, namespaced under the foreach stage's own name, the reduced value is
// written to.
type RollupDef struct {
	Key string `yaml:"key" json:"key"`
	Agg string `yaml:"agg" json:"agg"`
	As  string `yaml:"as" json:"as"`
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
