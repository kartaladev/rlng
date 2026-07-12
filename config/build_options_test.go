package config_test

import (
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildLintOptions covers Build's default (advisory-only) lint behavior
// versus WithLintErrors (promotes findings to a *LintError). Both cases build
// the same first-match table with no default and no catch-all — a lint smell
// — and only the option/assertion differ, so they share a table.
func TestBuildLintOptions(t *testing.T) {
	t.Parallel()

	doc := []byte(`{"stages":[{"name":"t","type":"decision-table","rules":[{"condition":"x > 1","decisions":{"y":"1"}}]}]}`)

	type testCase struct {
		name   string
		opts   []config.BuildOption
		assert func(t *testing.T, err error)
	}

	cases := []testCase{
		{
			name: "default Build does not enforce lint",
			opts: nil,
			assert: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "WithLintErrors promotes findings to a LintError",
			opts: []config.BuildOption{config.WithLintErrors()},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				var le *config.LintError
				require.ErrorAs(t, err, &le)
				require.NotEmpty(t, le.Findings)
				require.Equal(t, config.LintMissingDefault, le.Findings[0].Code)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, err := config.ParseJSON(doc)
			require.NoError(t, err)

			_, err = d.Build(tc.opts...)
			tc.assert(t, err)
		})
	}
}

// TestLintErrorMessage covers LintError.Error's rendering: the finding count
// and each finding's stage/code/message, semicolon-joined for two-or-more
// findings — the debuggability surface a caller reads on a failed Build.
func TestLintErrorMessage(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		err    *config.LintError
		assert func(t *testing.T, msg string)
	}

	cases := []testCase{
		{
			name: "single finding",
			err: &config.LintError{Findings: []config.Finding{
				{Stage: "t", Code: config.LintMissingDefault, Message: "no catch-all rule and no default: an unmatched input produces no output"},
			}},
			assert: func(t *testing.T, msg string) {
				assert.Equal(t, `config: 1 lint finding(s): stage "t": missing-default: no catch-all rule and no default: an unmatched input produces no output`, msg)
			},
		},
		{
			name: "multiple findings are semicolon-joined",
			err: &config.LintError{Findings: []config.Finding{
				{Stage: "t", Code: config.LintMissingDefault, Message: "no catch-all rule and no default: an unmatched input produces no output"},
				{Stage: "u", Rule: 2, Code: config.LintUnreachableRule, Message: "rule is unreachable: an earlier catch-all (true) rule always matches first"},
			}},
			assert: func(t *testing.T, msg string) {
				assert.Equal(t, `config: 2 lint finding(s): stage "t": missing-default: no catch-all rule and no default: an unmatched input produces no output; stage "u": unreachable-rule: rule is unreachable: an earlier catch-all (true) rule always matches first`, msg)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t, tc.err.Error())
		})
	}
}
