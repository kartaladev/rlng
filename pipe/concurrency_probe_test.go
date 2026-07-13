package pipe_test

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/kartaladev/rlng/pipe"
)

// cyclicBarrier releases waiters in batches of size n; it re-arms after each
// batch, so a level of more than n gated stages passing through a size-c
// semaphore is released in c-sized waves without deadlock.
type cyclicBarrier struct {
	mu    sync.Mutex
	size  int
	count int
	gen   chan struct{}
}

func newCyclicBarrier(size int) *cyclicBarrier {
	return &cyclicBarrier{size: size, gen: make(chan struct{})}
}

func (b *cyclicBarrier) wait() {
	b.mu.Lock()
	ch := b.gen
	b.count++
	if b.count == b.size {
		b.count = 0
		b.gen = make(chan struct{})
		b.mu.Unlock()
		close(ch)
		return
	}
	b.mu.Unlock()
	<-ch
}

// probe tracks the peak number of stages executing concurrently and gates each
// stage on a cyclic barrier so a whole batch demonstrably overlaps.
type probe struct {
	active  atomic.Int32
	peak    atomic.Int32
	barrier *cyclicBarrier
}

func (p *probe) recordPeak(n int32) {
	for {
		old := p.peak.Load()
		if n <= old || p.peak.CompareAndSwap(old, n) {
			return
		}
	}
}

// gatedStage marks itself active on Execute, records the concurrency peak, waits
// on the barrier so its batch overlaps, then sets its output.
type gatedStage struct {
	name string
	deps []string
	pr   *probe
}

func (s *gatedStage) Name() string        { return s.name }
func (s *gatedStage) Type() string        { return "test-gated" }
func (s *gatedStage) DependsOn() []string { return s.deps }
func (s *gatedStage) Execute(_ context.Context, sc *pipe.Scope) error {
	n := s.pr.active.Add(1)
	s.pr.recordPeak(n)
	s.pr.barrier.wait()
	s.pr.active.Add(-1)
	return sc.Set(s.name, true)
}

// erroringStage returns a fixed error without touching the Scope.
type erroringStage struct {
	name string
	deps []string
	err  error
}

func (s *erroringStage) Name() string                               { return s.name }
func (s *erroringStage) Type() string                               { return "test-err" }
func (s *erroringStage) DependsOn() []string                        { return s.deps }
func (s *erroringStage) Execute(context.Context, *pipe.Scope) error { return s.err }

// cancelThenErrStage cancels the run context and then returns its own error,
// exercising that the wave runner surfaces the stage error (as sequential Run
// does) rather than masking it with context.Canceled.
type cancelThenErrStage struct {
	name   string
	deps   []string
	cancel context.CancelFunc
	err    error
}

func (s *cancelThenErrStage) Name() string        { return s.name }
func (s *cancelThenErrStage) Type() string        { return "test-cancel-err" }
func (s *cancelThenErrStage) DependsOn() []string { return s.deps }
func (s *cancelThenErrStage) Execute(context.Context, *pipe.Scope) error {
	s.cancel()
	return s.err
}

// setStage records that it ran by setting a bool at its own name.
type setStage struct {
	name string
	deps []string
}

func (s *setStage) Name() string        { return s.name }
func (s *setStage) Type() string        { return "test-set" }
func (s *setStage) DependsOn() []string { return s.deps }
func (s *setStage) Execute(_ context.Context, sc *pipe.Scope) error {
	return sc.Set(s.name, true)
}
