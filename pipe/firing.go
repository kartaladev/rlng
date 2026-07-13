package pipe

import "sort"

// FiringRule identifies the decision-table rule that produced a stage's output,
// making a decision explainable ("Denied by rule CREDIT_MIN_650"). It is
// recorded for every hit policy when a rule matches, and for the default (else)
// decisions when no rule matched. For policies that can fire several rules
// (collect, any), use FiringRulesFor to retrieve all contributing rules.
type FiringRule struct {
	Stage     string `json:"stage"`                // the decision-table stage name
	RuleID    string `json:"rule_id,omitempty"`    // the matched rule's ID ("" when the default fired)
	Message   string `json:"message,omitempty"`    // the matched rule's Message (if any)
	IsDefault bool   `json:"is_default,omitempty"` // true when the table's default decisions fired
}

// recordFiring notes the single rule that fired for a stage (single/unique
// policies, or a default), replacing any prior record for that stage. It is
// always recorded (independent of provenance) for a cheap audit.
func (s *Scope) recordFiring(stage, ruleID, message string, isDefault bool) {
	s.recordFirings(stage, []FiringRule{{Stage: stage, RuleID: ruleID, Message: message, IsDefault: isDefault}})
}

// recordFirings notes the ordered set of rules that fired for a stage (collect
// and any policies can fire several), replacing any prior record for that stage.
func (s *Scope) recordFirings(stage string, rules []FiringRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.firing == nil {
		s.firing = make(map[string][]FiringRule)
	}
	s.firing[stage] = rules
}

// firingMap returns a shallow copy of the raw firing map (composite stage key ->
// firing rules) so a caller can re-key it without holding the lock. The
// FiringRule slices are shared (recorded firings are treated as immutable).
// Returns nil when nothing has fired.
func (s *Scope) firingMap() map[string][]FiringRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.firing) == 0 {
		return nil
	}
	out := make(map[string][]FiringRule, len(s.firing))
	for k, v := range s.firing {
		out[k] = v
	}
	return out
}

// recordElementFirings merges src (a per-element scope's firing map) into s,
// re-keying each entry to prefix + "." + key. A foreach uses it to surface each
// element's firing under "<stage>[i].<inner stage key>", preserving the inner
// stage — and, for a nested foreach, the inner element index — instead of
// flattening it away. Always recorded (independent of provenance, like
// recordFirings); callers pass a non-empty src (they guard emptiness
// themselves, as ForEach.Execute does). Mirrors recordElementDerivations.
func (s *Scope) recordElementFirings(prefix string, src map[string][]FiringRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.firing == nil {
		s.firing = make(map[string][]FiringRule, len(src))
	}
	mergePrefixed(s.firing, prefix, src, func(_ string, rules []FiringRule) []FiringRule { return rules })
}

// FiringRule returns the first rule that fired for the named decision-table
// stage, or false if that stage did not run, matched nothing, and had no
// default. For a policy that can fire several rules (collect, any), use
// FiringRulesFor to get all of them.
func (s *Scope) FiringRule(stage string) (FiringRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := s.firing[stage]
	if len(rules) == 0 {
		return FiringRule{}, false
	}
	return rules[0], true
}

// FiringRulesFor returns every rule that fired for the named stage, in firing
// order (nil if the stage recorded none).
func (s *Scope) FiringRulesFor(stage string) []FiringRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := s.firing[stage]
	if len(rules) == 0 {
		return nil
	}
	out := make([]FiringRule, len(rules))
	copy(out, rules)
	return out
}

// FiringRules returns every recorded firing rule across all stages, sorted by
// stage name (and, within a stage, in firing order) — a compact audit trail of
// which rules decided each decision-table stage.
func (s *Scope) FiringRules() []FiringRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stages := make([]string, 0, len(s.firing))
	for stage := range s.firing {
		stages = append(stages, stage)
	}
	sort.Strings(stages)
	var out []FiringRule
	for _, stage := range stages {
		out = append(out, s.firing[stage]...)
	}
	return out
}
