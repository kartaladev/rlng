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
