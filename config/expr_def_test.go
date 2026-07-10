package config

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestExprDefUnmarshalYAML(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		yaml   string
		assert func(t *testing.T, e ExprDef, err error)
	}

	cases := []testCase{
		{
			name: "scalar shorthand sets Expr",
			yaml: `price * qty`,
			assert: func(t *testing.T, e ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "price * qty", e.Expr)
			},
		},
		{
			name: "mapping decodes fields",
			yaml: "expr: base * 1.1\nfallback: \"0\"\ncoerce: true",
			assert: func(t *testing.T, e ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "base * 1.1", e.Expr)
				assert.Equal(t, "0", e.Fallback)
				assert.True(t, e.Coerce)
			},
		},
		{
			name: "sequence node is rejected",
			yaml: `[1, 2, 3]`,
			assert: func(t *testing.T, e ExprDef, err error) {
				var ce *ConfigError
				require.ErrorAs(t, err, &ce)
			},
		},
		{
			name: "mapping with bad field type errors",
			yaml: "expr: base\ncoerce: notabool",
			assert: func(t *testing.T, e ExprDef, err error) {
				require.Error(t, err)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var e ExprDef
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
		assert func(t *testing.T, e ExprDef, err error)
	}

	cases := []testCase{
		{
			name: "string shorthand sets Expr",
			json: `"price * qty"`,
			assert: func(t *testing.T, e ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "price * qty", e.Expr)
			},
		},
		{
			name: "object decodes fields",
			json: `{"expr": "base * 1.1", "fallback": "0", "coerce": true}`,
			assert: func(t *testing.T, e ExprDef, err error) {
				require.NoError(t, err)
				assert.Equal(t, "base * 1.1", e.Expr)
				assert.Equal(t, "0", e.Fallback)
				assert.True(t, e.Coerce)
			},
		},
		{
			name: "malformed json object errors",
			json: `{bad`,
			assert: func(t *testing.T, e ExprDef, err error) {
				require.Error(t, err)
			},
		},
		{
			name: "well-formed non-object non-string errors",
			json: `[1, 2, 3]`,
			assert: func(t *testing.T, e ExprDef, err error) {
				require.Error(t, err)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var e ExprDef
			err := json.Unmarshal([]byte(tc.json), &e)
			tc.assert(t, e, err)
		})
	}
}

func TestExprDefOptions(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		def  ExprDef
		want int // number of options produced
	}

	cases := []testCase{
		{name: "none", def: ExprDef{Expr: "1"}, want: 0},
		{name: "fallback only", def: ExprDef{Expr: "1", Fallback: "0"}, want: 1},
		{name: "globals only", def: ExprDef{Expr: "1", Globals: map[string]any{"k": 1}}, want: 1},
		{name: "coerce only", def: ExprDef{Expr: "1", Coerce: true}, want: 1},
		{name: "all three", def: ExprDef{Expr: "1", Fallback: "0", Globals: map[string]any{"k": 1}, Coerce: true}, want: 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Len(t, tc.def.options(), tc.want)
		})
	}
}

func TestConfigErrorMessage(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		err  *ConfigError
		want string
	}

	cause := errors.New("boom")
	cases := []testCase{
		{name: "stage and field", err: &ConfigError{Stage: "s", Field: "f", Cause: cause}, want: `config: stage "s" field "f": boom`},
		{name: "stage only", err: &ConfigError{Stage: "s", Cause: cause}, want: `config: stage "s": boom`},
		{name: "field only", err: &ConfigError{Field: "f", Cause: cause}, want: `config: field "f": boom`},
		{name: "neither", err: &ConfigError{Cause: cause}, want: `config: boom`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.err.Error())
			assert.ErrorIs(t, tc.err, cause)
		})
	}
}
