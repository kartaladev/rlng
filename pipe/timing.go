package pipe

import "time"

// Clock is the time source used to stamp evaluation timing. Any type with a
// Now() time.Time method satisfies it — including github.com/jonboulle/clockwork
// clocks (real or fake), so tests can inject a deterministic time source without
// this package depending on clockwork.
type Clock interface {
	Now() time.Time
}

// realClock is the default Clock, delegating to time.Now.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// NowFunc returns a host function (for expr.WithFunction) that yields the given
// clock's current time, so temporal rules can call now() deterministically —
// e.g. expr.WithFunction("now", pipe.NowFunc(clk)) then a rule like
// `expires.Before(now())`. Backing now() with an injected clock (rather than the
// wall clock) keeps rule evaluation reproducible and testable.
func NowFunc(clock Clock) func(...any) (any, error) {
	return func(...any) (any, error) { return clock.Now(), nil }
}

// WithClock sets the Clock used to stamp evaluation timing. It defaults to the
// real (time.Now) clock; tests inject a deterministic Clock for stable output. A
// nil clock is ignored (the default is kept).
func WithClock(clock Clock) ScopeOption {
	return func(s *Scope) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// markStarted records the start of an evaluation. Called by Pipeline.Run.
func (s *Scope) markStarted() {
	now := s.clock.Now()
	s.mu.Lock()
	s.startedAt = now
	s.mu.Unlock()
}

// markFinished records the elapsed evaluation time. Called by Pipeline.Run,
// including when a stage errors — a partial run still took time.
func (s *Scope) markFinished() {
	now := s.clock.Now()
	s.mu.Lock()
	s.duration = now.Sub(s.startedAt)
	s.mu.Unlock()
}

// StageTiming pairs a stage name with how long its Execute took.
type StageTiming struct {
	Stage    string
	Duration time.Duration
}

// timeStage runs a stage's work, timing it with the Scope's clock and recording
// the elapsed duration under name (in first-execution order). The stage's error,
// if any, is returned unchanged — a stage that errored still took time.
func (s *Scope) timeStage(name string, run func() error) error {
	start := s.clock.Now()
	err := run()
	elapsed := s.clock.Now().Sub(start)

	s.mu.Lock()
	if s.stageTimes == nil {
		s.stageTimes = make(map[string]time.Duration)
	}
	if _, seen := s.stageTimes[name]; !seen {
		s.stageOrder = append(s.stageOrder, name)
	}
	s.stageTimes[name] = elapsed
	s.mu.Unlock()
	return err
}

// StageDuration reports how long the named stage's Execute took, and false if no
// stage of that name has run.
func (s *Scope) StageDuration(name string) (time.Duration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.stageTimes[name]
	return d, ok
}

// StageTimings reports each stage's Execute duration in execution order — a
// per-stage breakdown for observability, complementing the total Duration.
func (s *Scope) StageTimings() []StageTiming {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]StageTiming, 0, len(s.stageOrder))
	for _, name := range s.stageOrder {
		out = append(out, StageTiming{Stage: name, Duration: s.stageTimes[name]})
	}
	return out
}

// StartedAt reports when the pipeline run began, and false if no run has stamped
// the Scope.
func (s *Scope) StartedAt() (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.startedAt, !s.startedAt.IsZero()
}

// Duration reports how long the pipeline run took, and false if no run has
// stamped the Scope.
func (s *Scope) Duration() (time.Duration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.duration, !s.startedAt.IsZero()
}
