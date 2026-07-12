package pipe_test

import "time"

// fixedClock is a test pipe.Clock returning a constant time.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// stubClock is a test pipe.Clock returning preset times in order; once exhausted
// it repeats the last time. Useful for a deterministic start/finish pair.
type stubClock struct {
	times []time.Time
	i     int
}

func (c *stubClock) Now() time.Time {
	t := c.times[c.i]
	if c.i < len(c.times)-1 {
		c.i++
	}
	return t
}
