package pipe

// RulesetIdentity names which ruleset produced a decision. Hash is a
// deterministic content fingerprint (the config path fills it from
// (*config.PipelineDef).Hash()); Version is an optional author-declared release
// label. The two are orthogonal: Hash proves what ran, Version names which
// release it was. The zero value means "no identity".
type RulesetIdentity struct {
	Hash    string `json:"hash,omitempty"`
	Version string `json:"version,omitempty"`
}

// WithRuleset records which ruleset this Pipeline evaluates, so Run can stamp
// the identity onto each Scope. It returns the Pipeline for chaining. Configure
// it once, before Run — it is not safe to call concurrently with Run.
func (p *Pipeline) WithRuleset(id RulesetIdentity) *Pipeline {
	p.ruleset = id
	return p
}

// stampRuleset records id on the Scope. The zero identity is a no-op, so an
// un-configured Pipeline leaves Ruleset reporting absent.
func (s *Scope) stampRuleset(id RulesetIdentity) {
	if id == (RulesetIdentity{}) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ruleset = id
}

// Ruleset returns the ruleset identity stamped onto this Scope during Run, and
// false if the Scope was never stamped (the pipeline carried no identity, or it
// has not run). A stamped Scope is self-describing: its inputs, firing rules,
// provenance, and the ruleset that produced them travel together.
func (s *Scope) Ruleset() (RulesetIdentity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ruleset, s.ruleset != RulesetIdentity{}
}
