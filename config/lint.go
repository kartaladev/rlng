package config

import "strings"

// Lint finding codes.
const (
	// LintUnreachableRule marks a decision-table rule that can never fire because
	// an earlier rule is an unconditional catch-all (a `true` condition) under a
	// first-match policy (single/unique).
	LintUnreachableRule = "unreachable-rule"
	// LintMissingDefault marks a first-match decision table (single/unique) that
	// has neither a catch-all rule nor a default block, so an unmatched input
	// silently produces no output.
	LintMissingDefault = "missing-default"
)

// Severity levels for a Finding.
const (
	SeverityWarning = "warning"
)

// Finding is one issue reported by Lint. It is advisory: Lint performs static
// analysis of the definition without evaluating any expression.
type Finding struct {
	Stage    string // the stage the finding concerns
	Rule     int    // rule index within the stage, or -1 when not rule-specific
	Severity string // SeverityWarning
	Code     string // machine-readable code (LintUnreachableRule, ...)
	Message  string // human-readable explanation
}

// Lint statically analyzes the definition and returns advisory findings —
// authoring smells that a compile (Build) does not catch, such as an unreachable
// rule shadowed by an earlier catch-all, or a first-match table with no default
// and no catch-all (a silent no-match gap). It never evaluates expressions and
// returns an empty slice for a clean definition.
func (d *PipelineDef) Lint() []Finding {
	var findings []Finding
	for _, sd := range d.Stages {
		if sd.Type != decisionTableType {
			continue
		}
		findings = append(findings, sd.lintTable()...)
	}
	return findings
}

const decisionTableType = "decision-table"

// lintTable checks one decision-table stage. Coverage/unreachability apply only
// to first-match policies (single/unique); collect accumulates all matches (an
// empty result is valid) and any permits overlap by design.
func (sd StageDef) lintTable() []Finding {
	if !isFirstMatch(sd.HitPolicy) {
		return nil
	}

	var findings []Finding
	catchAll := -1
	for i, r := range sd.Rules {
		if isCatchAll(r.Condition.Expr) {
			catchAll = i
			break
		}
	}

	switch {
	case catchAll >= 0 && catchAll < len(sd.Rules)-1:
		// Rules after an unconditional catch-all can never fire.
		for i := catchAll + 1; i < len(sd.Rules); i++ {
			findings = append(findings, Finding{
				Stage:    sd.Name,
				Rule:     i,
				Severity: SeverityWarning,
				Code:     LintUnreachableRule,
				Message:  "rule is unreachable: an earlier catch-all (true) rule always matches first",
			})
		}
	case catchAll < 0 && len(sd.Default) == 0:
		// No catch-all and no default: an unmatched input yields no output.
		findings = append(findings, Finding{
			Stage:    sd.Name,
			Rule:     -1,
			Severity: SeverityWarning,
			Code:     LintMissingDefault,
			Message:  "no catch-all rule and no default: an unmatched input produces no output",
		})
	}
	return findings
}

// isFirstMatch reports whether the hit policy resolves to a single winning rule.
func isFirstMatch(hitPolicy string) bool {
	switch hitPolicy {
	case "", "single", "unique":
		return true
	default:
		return false
	}
}

// isCatchAll reports whether a condition is an unconditional truth. Detection is
// best-effort and syntactic: it recognizes the literal `true`, a parenthesized
// `(true)`, and the trivial tautology `1 == 1`. A semantic always-true condition
// it does not recognize may still be flagged missing-default (a false positive);
// this is advisory analysis, not evaluation.
func isCatchAll(condition string) bool {
	s := strings.TrimSpace(condition)
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "("), ")"))
	switch s {
	case "true", "1 == 1":
		return true
	default:
		return false
	}
}
