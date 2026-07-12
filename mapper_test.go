package rlng

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mapResult struct {
	Total int `mapstructure:"total"`
	Info  struct {
		Tag string `mapstructure:"tag"`
	} `mapstructure:"info"`
}

func TestNewMapper(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		tmpl   MappingTemplate
		assert func(t *testing.T, m *Mapper[mapResult], err error)
	}

	cases := []testCase{
		{
			name: "compiles fields",
			tmpl: MappingTemplate{"total": "1 + 1"},
			assert: func(t *testing.T, m *Mapper[mapResult], err error) {
				require.NoError(t, err)
				require.NotNil(t, m)
			},
		},
		{
			name: "empty template is valid",
			tmpl: MappingTemplate{},
			assert: func(t *testing.T, m *Mapper[mapResult], err error) {
				require.NoError(t, err)
				require.NotNil(t, m)
			},
		},
		{
			name: "bad field expression is a MappingError",
			tmpl: MappingTemplate{"total": "1 +"},
			assert: func(t *testing.T, m *Mapper[mapResult], err error) {
				assert.Nil(t, m)
				var me *MappingError
				require.ErrorAs(t, err, &me)
				assert.Equal(t, "total", me.Field)
			},
		},
		{
			name: "empty template key is rejected",
			tmpl: MappingTemplate{"": "1"},
			assert: func(t *testing.T, m *Mapper[mapResult], err error) {
				assert.Nil(t, m)
				require.ErrorIs(t, err, errEmptyMappingKey)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := NewMapper[mapResult](tc.tmpl)
			tc.assert(t, m, err)
		})
	}
}

func TestMapperMapStruct(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		tmpl   MappingTemplate
		scope  map[string]any
		assert func(t *testing.T, r mapResult, err error)
	}

	cases := []testCase{
		{
			name:  "single and nested fields",
			tmpl:  MappingTemplate{"total": "net + tax", "info.tag": "label"},
			scope: map[string]any{"net": 10, "tax": 2, "label": "big"},
			assert: func(t *testing.T, r mapResult, err error) {
				require.NoError(t, err)
				assert.Equal(t, 12, r.Total)
				assert.Equal(t, "big", r.Info.Tag)
			},
		},
		{
			name:  "field eval error is a MappingError",
			tmpl:  MappingTemplate{"total": "a % 0"},
			scope: map[string]any{"a": 1},
			assert: func(t *testing.T, r mapResult, err error) {
				var me *MappingError
				require.ErrorAs(t, err, &me)
				assert.Equal(t, "total", me.Field)
			},
		},
		{
			name:  "decode type mismatch is a MappingError",
			tmpl:  MappingTemplate{"total": `"not a number"`},
			scope: map[string]any{},
			assert: func(t *testing.T, r mapResult, err error) {
				var me *MappingError
				require.ErrorAs(t, err, &me)
				assert.Empty(t, me.Field) // final decode
			},
		},
		{
			name:  "colliding output paths are a MappingError, not a silent overwrite",
			tmpl:  MappingTemplate{"info": "label", "info.tag": "label"},
			scope: map[string]any{"label": "x"},
			assert: func(t *testing.T, r mapResult, err error) {
				var me *MappingError
				require.ErrorAs(t, err, &me)
				assert.Equal(t, "info.tag", me.Field)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := NewMapper[mapResult](tc.tmpl)
			require.NoError(t, err)
			r, err := m.Map(tc.scope)
			tc.assert(t, r, err)
		})
	}
}

// TestMapperMapToMap covers R = map[string]any (a structurally different R than
// the struct table above), so it is a separate focused test.
func TestMapperMapToMap(t *testing.T) {
	t.Parallel()

	m, err := NewMapper[map[string]any](MappingTemplate{"total": "1 + 2"})
	require.NoError(t, err)
	r, err := m.Map(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 3, r["total"])
}

// TestMapperNestedSiblingPaths covers two output paths sharing a nested prefix:
// the second reuses the intermediate map created by the first (no collision).
func TestMapperNestedSiblingPaths(t *testing.T) {
	t.Parallel()

	m, err := NewMapper[map[string]any](MappingTemplate{"info.tag": `"a"`, "info.note": `"b"`})
	require.NoError(t, err)
	r, err := m.Map(map[string]any{})
	require.NoError(t, err)
	info := r["info"].(map[string]any)
	assert.Equal(t, "a", info["tag"])
	assert.Equal(t, "b", info["note"])
}

func TestMappingErrorMessage(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		err  *MappingError
		want string
	}

	cause := errors.New("boom")
	cases := []testCase{
		{name: "field", err: &MappingError{Field: "total", Cause: cause}, want: `rlng: mapping field "total": boom`},
		{name: "final decode", err: &MappingError{Cause: cause}, want: `rlng: mapping: boom`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.err.Error())
			assert.ErrorIs(t, tc.err, cause)
		})
	}
}
