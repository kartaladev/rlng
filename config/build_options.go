package config

import (
	"fmt"
	"strings"
)

// buildConfig holds Build-time toggles assembled from BuildOption values.
type buildConfig struct {
	lintErrors bool
	schema     map[string]any // set in Task 4
	strict     bool           // set in Task 4
}

// BuildOption configures (*PipelineDef).Build.
type BuildOption func(*buildConfig)

// WithLintErrors runs Lint during Build and promotes every finding to a
// *LintError, so an authoring smell (missing default, unreachable rule) fails
// construction instead of surfacing only if the caller separately calls Lint.
func WithLintErrors() BuildOption {
	return func(c *buildConfig) { c.lintErrors = true }
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
