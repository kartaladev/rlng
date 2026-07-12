package config_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestExprDefUnmarshalYAML(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		yaml   string
		assert func(t *testing.T, e config.ExprDef, err error)
	}

	cases := []testCase{
		{
			name: "scalar shorthand sets Expr",
			yaml: `price * qty`,
			assert: func(t *testing.T, e config.ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "price * qty", e.Expr)
			},
		},
		{
			name: "mapping decodes fields",
			yaml: "expr: base * 1.1\nfallback: \"0\"\ncoerce: true",
			assert: func(t *testing.T, e config.ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "base * 1.1", e.Expr)
				assert.Equal(t, "0", e.Fallback)
				assert.True(t, e.Coerce)
			},
		},
		{
			name: "sequence node is rejected",
			yaml: `[1, 2, 3]`,
			assert: func(t *testing.T, e config.ExprDef, err error) {
				var ce *config.ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
		{
			name: "mapping with bad field type errors",
			yaml: "expr: base\ncoerce: notabool",
			assert: func(t *testing.T, e config.ExprDef, err error) {
				require.Error(t, err)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var e config.ExprDef
			err := yaml.Unmarshal([]byte(tc.yaml), &e)
			tc.assert(t, e, err)
		})
	}
}

func TestExprDefUnmarshalJSON(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		json   string
		assert func(t *testing.T, e config.ExprDef, err error)
	}

	cases := []testCase{
		{
			name: "string shorthand sets Expr",
			json: `"price * qty"`,
			assert: func(t *testing.T, e config.ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "price * qty", e.Expr)
			},
		},
		{
			name: "object decodes fields",
			json: `{"expr": "base * 1.1", "fallback": "0", "coerce": true}`,
			assert: func(t *testing.T, e config.ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "base * 1.1", e.Expr)
				assert.Equal(t, "0", e.Fallback)
				assert.True(t, e.Coerce)
			},
		},
		{
			name: "malformed json object errors",
			json: `{bad`,
			assert: func(t *testing.T, e config.ExprDef, err error) {
				require.Error(t, err)
			},
		},
		{
			name: "well-formed non-object non-string errors",
			json: `[1, 2, 3]`,
			assert: func(t *testing.T, e config.ExprDef, err error) {
				require.Error(t, err)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var e config.ExprDef
			err := json.Unmarshal([]byte(tc.json), &e)
			tc.assert(t, e, err)
		})
	}
}

// TestExprDefOptionsWiring verifies that an ExprDef's Fallback, Globals, and
// Coerce options are wired through Build into the compiled stage — observably,
// through the exported API (replacing the former white-box test of the internal
// options() mapping). A single stage exercises all three: Globals patches the
// value expression's default, that expression then errors (modulo by zero) so
// the Fallback value is used, and the non-bool Condition relies on Coerce to
// gate the stage on.
func TestExprDefOptionsWiring(t *testing.T) {
	t.Parallel()

	def := config.PipelineDef{Stages: []config.StageDef{
		{
			Name: "s",
			Type: "single-expr",
			Expr: &config.ExprDef{
				Expr:     "missing % 0", // Globals default -> 1 % 0 -> runtime error -> Fallback
				Fallback: "42",
				Globals:  map[string]any{"missing": 1},
			},
			Condition: &config.ExprDef{Expr: "1", Coerce: true}, // non-bool 1, coerced truthy -> stage runs
		},
	}}

	p, err := def.Build()
	require.NoError(t, err)

	sc := pipe.NewScope(nil)
	require.NoError(t, p.Run(context.Background(), sc))

	got, ok := sc.Get("s")
	require.True(t, ok, "coerced condition must gate the stage on")
	assert.Equal(t, 42, got, "Globals default + Fallback must be wired through Build")
}

func TestExprDefObjectFormRejectsUnknownKeyYAML(t *testing.T) {
	doc := []byte(`
stages:
  - name: s
    type: single-expr
    expr:
      expr: "1"
      fallbck: "2"
`)
	_, err := config.ParseYAML(doc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fallbck")
}

func TestExprDefObjectFormRejectsUnknownKeyJSON(t *testing.T) {
	doc := []byte(`{"stages":[{"name":"s","type":"single-expr","expr":{"expr":"1","fallbck":"2"}}]}`)
	_, err := config.ParseJSON(doc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fallbck")
}

func TestExprDefObjectFormValidKeysStillParse(t *testing.T) {
	doc := []byte(`{"stages":[{"name":"s","type":"single-expr","expr":{"expr":"1","fallback":"2","coerce":true}}]}`)
	d, err := config.ParseJSON(doc)
	require.NoError(t, err)
	require.Equal(t, "1", d.Stages[0].Expr.Expr)
	require.Equal(t, "2", d.Stages[0].Expr.Fallback)
}

func TestConfigErrorMessage(t *testing.T) {
	t.Parallel()

	cause := errors.New("boom")

	type testCase struct {
		name   string
		err    *config.ConfigError
		assert func(t *testing.T, e *config.ConfigError)
	}

	cases := []testCase{
		{
			name: "stage and field",
			err:  &config.ConfigError{Stage: "s", Field: "f", Cause: cause},
			assert: func(t *testing.T, e *config.ConfigError) {
				assert.Equal(t, `config: stage "s" field "f": boom`, e.Error())
				assert.ErrorIs(t, e, cause)
			},
		},
		{
			name: "stage only",
			err:  &config.ConfigError{Stage: "s", Cause: cause},
			assert: func(t *testing.T, e *config.ConfigError) {
				assert.Equal(t, `config: stage "s": boom`, e.Error())
				assert.ErrorIs(t, e, cause)
			},
		},
		{
			name: "field only",
			err:  &config.ConfigError{Field: "f", Cause: cause},
			assert: func(t *testing.T, e *config.ConfigError) {
				assert.Equal(t, `config: field "f": boom`, e.Error())
				assert.ErrorIs(t, e, cause)
			},
		},
		{
			name: "neither",
			err:  &config.ConfigError{Cause: cause},
			assert: func(t *testing.T, e *config.ConfigError) {
				assert.Equal(t, `config: boom`, e.Error())
				assert.ErrorIs(t, e, cause)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t, tc.err)
		})
	}
}
