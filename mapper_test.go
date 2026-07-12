package rlng_test

import (
	"errors"
	"testing"

	"github.com/kartaladev/rlng"
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
		tmpl   rlng.MappingTemplate
		assert func(t *testing.T, m *rlng.Mapper[mapResult], err error)
	}

	cases := []testCase{
		{
			name: "compiles fields",
			tmpl: rlng.MappingTemplate{"total": "1 + 1"},
			assert: func(t *testing.T, m *rlng.Mapper[mapResult], err error) {
				require.NoError(t, err)
				require.NotNil(t, m)
			},
		},
		{
			name: "empty template is valid",
			tmpl: rlng.MappingTemplate{},
			assert: func(t *testing.T, m *rlng.Mapper[mapResult], err error) {
				require.NoError(t, err)
				require.NotNil(t, m)
			},
		},
		{
			name: "bad field expression is a MappingError",
			tmpl: rlng.MappingTemplate{"total": "1 +"},
			assert: func(t *testing.T, m *rlng.Mapper[mapResult], err error) {
				assert.Nil(t, m)
				var me *rlng.MappingError
				require.ErrorAs(t, err, &me)
				assert.Equal(t, "total", me.Field)
			},
		},
		{
			name: "empty template key is rejected",
			tmpl: rlng.MappingTemplate{"": "1"},
			assert: func(t *testing.T, m *rlng.Mapper[mapResult], err error) {
				assert.Nil(t, m)
				require.ErrorIs(t, err, rlng.ErrEmptyMappingKey)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := rlng.NewMapper[mapResult](tc.tmpl)
			tc.assert(t, m, err)
		})
	}
}

func TestMapperMapStruct(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		tmpl   rlng.MappingTemplate
		scope  map[string]any
		assert func(t *testing.T, r mapResult, err error)
	}

	cases := []testCase{
		{
			name:  "single and nested fields",
			tmpl:  rlng.MappingTemplate{"total": "net + tax", "info.tag": "label"},
			scope: map[string]any{"net": 10, "tax": 2, "label": "big"},
			assert: func(t *testing.T, r mapResult, err error) {
				require.NoError(t, err)
				assert.Equal(t, 12, r.Total)
				assert.Equal(t, "big", r.Info.Tag)
			},
		},
		{
			name:  "field eval error is a MappingError",
			tmpl:  rlng.MappingTemplate{"total": "a % 0"},
			scope: map[string]any{"a": 1},
			assert: func(t *testing.T, r mapResult, err error) {
				var me *rlng.MappingError
				require.ErrorAs(t, err, &me)
				assert.Equal(t, "total", me.Field)
			},
		},
		{
			name:  "decode type mismatch is a MappingError",
			tmpl:  rlng.MappingTemplate{"total": `"not a number"`},
			scope: map[string]any{},
			assert: func(t *testing.T, r mapResult, err error) {
				var me *rlng.MappingError
				require.ErrorAs(t, err, &me)
				assert.Empty(t, me.Field) // final decode
			},
		},
		{
			name:  "colliding output paths are a MappingError, not a silent overwrite",
			tmpl:  rlng.MappingTemplate{"info": "label", "info.tag": "label"},
			scope: map[string]any{"label": "x"},
			assert: func(t *testing.T, r mapResult, err error) {
				var me *rlng.MappingError
				require.ErrorAs(t, err, &me)
				assert.Equal(t, "info.tag", me.Field)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := rlng.NewMapper[mapResult](tc.tmpl)
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

	m, err := rlng.NewMapper[map[string]any](rlng.MappingTemplate{"total": "1 + 2"})
	require.NoError(t, err)
	r, err := m.Map(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 3, r["total"])
}

// TestMapperNestedSiblingPaths covers two output paths sharing a nested prefix:
// the second reuses the intermediate map created by the first (no collision).
func TestMapperNestedSiblingPaths(t *testing.T) {
	t.Parallel()

	m, err := rlng.NewMapper[map[string]any](rlng.MappingTemplate{"info.tag": `"a"`, "info.note": `"b"`})
	require.NoError(t, err)
	r, err := m.Map(map[string]any{})
	require.NoError(t, err)
	info := r["info"].(map[string]any)
	assert.Equal(t, "a", info["tag"])
	assert.Equal(t, "b", info["note"])
}

func TestMappingErrorMessage(t *testing.T) {
	t.Parallel()

	cause := errors.New("boom")

	type testCase struct {
		name   string
		err    *rlng.MappingError
		assert func(t *testing.T, e *rlng.MappingError)
	}

	cases := []testCase{
		{
			name: "field",
			err:  &rlng.MappingError{Field: "total", Cause: cause},
			assert: func(t *testing.T, e *rlng.MappingError) {
				assert.Equal(t, `rlng: mapping field "total": boom`, e.Error())
				assert.ErrorIs(t, e, cause)
			},
		},
		{
			name: "final decode",
			err:  &rlng.MappingError{Cause: cause},
			assert: func(t *testing.T, e *rlng.MappingError) {
				assert.Equal(t, `rlng: mapping: boom`, e.Error())
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
