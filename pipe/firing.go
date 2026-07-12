package pipe

import "sort"

// FiringRule identifies the decision-table rule that produced a stage's output,
// making a decision explainable ("Denied by rule CREDIT_MIN_650"). It is
// recorded for every hit policy when a rule matches, and for the default (else)
// decisions when no rule matched.
type FiringRule struct {
	Stage     string // the decision-table stage name
	RuleID    string // the matched rule's ID ("" when the default fired)
	Message   string // the matched rule's Message (if any)
	IsDefault bool   // true when the table's default decisions fired
}

// recordFiring notes which rule of a decision-table stage fired. It is always
// recorded (independent of provenance) so the firing rule is available for a
// cheap audit even without full lineage tracking.
func (s *Scope) recordFiring(stage, ruleID, message string, isDefault bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.firing == nil {
		s.firing = make(map[string]FiringRule)
	}
	s.firing[stage] = FiringRule{Stage: stage, RuleID: ruleID, Message: message, IsDefault: isDefault}
}

// FiringRule returns the rule that fired for the named decision-table stage, or
// false if that stage did not run, matched nothing, and had no default.
func (s *Scope) FiringRule(stage string) (FiringRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fr, ok := s.firing[stage]
	return fr, ok
}

// FiringRules returns every recorded firing rule, sorted by stage name — a
// compact audit trail of which rule decided each decision-table stage.
func (s *Scope) FiringRules() []FiringRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FiringRule, 0, len(s.firing))
	for _, fr := range s.firing {
		out = append(out, fr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stage < out[j].Stage })
	return out
}
