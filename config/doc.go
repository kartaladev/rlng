// Package config parses declarative YAML/JSON pipeline definitions and builds
// pipe.Pipeline values from them.
//
// A definition is an ordered list of stages; each stage names its type
// (single-expr, multi-expr, or decision-table), its dependencies, and its
// type-specific fields. Expression fields accept either a bare string (the
// expression) or an object with compile options (expr, fallback, globals,
// coerce). Parse with a Provider — e.g. FromYAMLBytes, FromJSONString,
// FromFile, or FromYAMLURL — then call Build to compile the definition into a
// *pipe.Pipeline.
//
// Strict mode & schema: A definition may declare a top-level schema block
// (field name -> representative value giving its type) to enable strict
// compilation. When schema is present, every stage expression is compiled with
// WithEnv, so field typos and type mismatches are caught at Build time instead
// of silently evaluating to nil. Call Build with WithStrict() to require strict
// compilation and error if no schema is available; Build with WithSchema(env)
// to supply schema programmatically for cases where it cannot be embedded in
// the document.
//
// Lint enforcement: Call Build with WithLintErrors() to promote static ruleset
// checks (unreachable rules, missing-default coverage gaps) to Build-time errors.
// Without WithLintErrors(), the same checks are available via the Lint function
// but do not block construction. LintError lists all violations and the stage
// each one occurs in.
//
// Ruleset identity: (*PipelineDef).Hash() is a deterministic content
// fingerprint (hex SHA-256 of the canonical parsed definition); a top-level
// version field (or the WithRulesetVersion BuildOption, which wins) names an
// author-declared release label, excluded from the hash. Build always stamps
// both onto the compiled Pipeline (pipe.WithRuleset), so every Scope it
// produces reports Scope.Ruleset(); MatchesRuleset checks a candidate
// definition against a persisted decision's stamped identity for safe replay.
package config
