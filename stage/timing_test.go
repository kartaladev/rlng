package stage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeTiming(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() *Scope
		run    bool // whether to stamp via markStarted/markFinished
		assert func(t *testing.T, s *Scope)
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []testCase{
		{
			name:  "unstamped scope reports not-run",
			build: func() *Scope { return NewScope(nil) },
			run:   false,
			assert: func(t *testing.T, s *Scope) {
				_, ok := s.StartedAt()
				assert.False(t, ok)
				_, ok = s.Duration()
				assert.False(t, ok)
			},
		},
		{
			name: "injected clock yields deterministic duration",
			build: func() *Scope {
				times := []time.Time{start, start.Add(5 * time.Millisecond)}
				i := 0
				return NewScope(nil, WithClock(func() time.Time {
					t := times[i]
					if i < len(times)-1 {
						i++
					}
					return t
				}))
			},
			run: true,
			assert: func(t *testing.T, s *Scope) {
				at, ok := s.StartedAt()
				require.True(t, ok)
				assert.Equal(t, start, at)
				d, ok := s.Duration()
				require.True(t, ok)
				assert.Equal(t, 5*time.Millisecond, d)
			},
		},
		{
			name:  "nil clock in WithClock is ignored (keeps default)",
			build: func() *Scope { return NewScope(nil, WithClock(nil)) },
			run:   true,
			assert: func(t *testing.T, s *Scope) {
				_, ok := s.Duration()
				assert.True(t, ok, "default time.Now clock must still stamp timing")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := tc.build()
			if tc.run {
				s.markStarted()
				s.markFinished()
			}
			tc.assert(t, s)
		})
	}
}
