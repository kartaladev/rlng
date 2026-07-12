package config

import (
	"fmt"
	"strings"
)

// buildConfig holds Build-time toggles assembled from BuildOption values.
type buildConfig struct {
	lintErrors bool
	schema     map[string]any // set by WithSchema; overrides PipelineDef.Schema when non-nil
	strict     bool           // set by WithStrict; also implied whenever a schema is present
	version    string         // set by WithRulesetVersion; overrides PipelineDef.Version
}

// BuildOption configures (*PipelineDef).Build.
type BuildOption func(*buildConfig)

// WithLintErrors runs Lint during Build and promotes every finding to a
// *LintError, so an authoring smell (missing default, unreachable rule) fails
// construction instead of surfacing only if the caller separately calls Lint.
func WithLintErrors() BuildOption {
	return func(c *buildConfig) { c.lintErrors = true }
}

// WithStrict requires strict compilation against the declared schema. Build
// returns a *ConfigError if no schema is available (neither PipelineDef.Schema
// nor WithSchema). Without WithStrict, strict mode is still enabled whenever a
// schema is present.
func WithStrict() BuildOption {
	return func(c *buildConfig) { c.strict = true }
}

// WithSchema supplies or overrides the type-check environment programmatically,
// for callers who cannot edit the document. It enables strict compilation.
func WithSchema(env map[string]any) BuildOption {
	return func(c *buildConfig) { c.schema = env }
}

// WithRulesetVersion sets the author-declared release label stamped onto the
// built pipeline's ruleset identity, overriding any version: field in the
// document. The content Hash() is always computed regardless; this only names
// the release.
func WithRulesetVersion(v string) BuildOption {
	return func(c *buildConfig) { c.version = v }
}

// LintError reports Lint findings promoted to a Build error by WithLintErrors.
type LintError struct{ Findings []Finding }

// Error renders the finding count and each finding's stage/code/message.
func (e *LintError) Error() string {
	msgs := make([]string, 0, len(e.Findings))
	for _, f := range e.Findings {
		msgs = append(msgs, fmt.Sprintf("stage %q: %s: %s", f.Stage, f.Code, f.Message))
	}
	return fmt.Sprintf("config: %d lint finding(s): %s", len(e.Findings), strings.Join(msgs, "; "))
}
