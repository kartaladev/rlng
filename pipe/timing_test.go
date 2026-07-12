package pipe_test

import (
	"testing"
	"time"

	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeTiming(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		build  func() *pipe.Scope
		run    bool // whether to stamp timing by running an (empty) pipeline
		assert func(t *testing.T, s *pipe.Scope)
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []testCase{
		{
			name:  "unstamped scope reports not-run",
			build: func() *pipe.Scope { return pipe.NewScope(nil) },
			run:   false,
			assert: func(t *testing.T, s *pipe.Scope) {
				_, ok := s.StartedAt()
				assert.False(t, ok)
				_, ok = s.Duration()
				assert.False(t, ok)
			},
		},
		{
			name: "injected clock yields deterministic duration",
			build: func() *pipe.Scope {
				return pipe.NewScope(nil, pipe.WithClock(&stubClock{
					times: []time.Time{start, start.Add(5 * time.Millisecond)},
				}))
			},
			run: true,
			assert: func(t *testing.T, s *pipe.Scope) {
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
			build: func() *pipe.Scope { return pipe.NewScope(nil, pipe.WithClock(nil)) },
			run:   true,
			assert: func(t *testing.T, s *pipe.Scope) {
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
				// Pipeline.Run stamps timing (markStarted/markFinished) around the
				// stages; an empty pipeline stamps with exactly two clock reads and
				// no intervening work.
				p, err := pipe.NewPipeline()
				require.NoError(t, err)
				require.NoError(t, p.Run(t.Context(), s))
			}
			tc.assert(t, s)
		})
	}
}
