package rlng_test

import (
	"errors"
	"testing"

	"github.com/kartaladev/rlng"
	"github.com/shopspring/decimal"
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

// decimalMapResult exercises every decimalNarrowHook branch: decimal->decimal
// (AsDecimal), decimal->int with an integral value (AsInt), decimal->float
// (AsFloat), and decimal->string (AsString).
type decimalMapResult struct {
	AsDecimal decimal.Decimal `mapstructure:"as_decimal"`
	AsInt     int             `mapstructure:"as_int"`
	AsFloat   float64         `mapstructure:"as_float"`
	AsString  string          `mapstructure:"as_string"`
}

// decimalIntResult isolates the decimal->int target so the lossy-narrowing
// case can fail the whole Map call without the other kind conversions in the
// way.
type decimalIntResult struct {
	AsInt int `mapstructure:"as_int"`
}

// TestMapperDecimalFidelity covers mapper.go's decimalNarrowHook: every
// non-decimal decode is already covered by TestMapperMapStruct above, so this
// table isolates the decimal-source branches.
func TestMapperDecimalFidelity(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		run  func(t *testing.T)
	}

	cases := []testCase{
		{
			name: "decimal->decimal exact, decimal->int integral, decimal->float, decimal->string",
			run: func(t *testing.T) {
				m, err := rlng.NewMapper[decimalMapResult](rlng.MappingTemplate{
					"as_decimal": `decimal("18125.00")`,
					"as_int":     `decimal("42")`,
					"as_float":   `decimal("3.14")`,
					"as_string":  `decimal("18125.5")`,
				})
				require.NoError(t, err)
				r, err := m.Map(map[string]any{})
				require.NoError(t, err)
				assert.True(t, decimal.RequireFromString("18125.00").Equal(r.AsDecimal))
				assert.Equal(t, 42, r.AsInt)
				assert.InDelta(t, 3.14, r.AsFloat, 1e-9)
				assert.Equal(t, "18125.5", r.AsString)
			},
		},
		{
			name: "fractional decimal into int field is a lossy-narrowing MappingError",
			run: func(t *testing.T) {
				m, err := rlng.NewMapper[decimalIntResult](rlng.MappingTemplate{"as_int": `decimal("3.5")`})
				require.NoError(t, err)
				_, err = m.Map(map[string]any{})
				require.Error(t, err)
				var me *rlng.MappingError
				require.ErrorAs(t, err, &me)
				require.ErrorIs(t, err, rlng.ErrLossyResultNarrowing)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
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

// TestMapperDecimalOverflowNarrowing guards the whole-branch review finding: an
// integer-valued decimal that exceeds int64 range must not be silently
// truncated by IntPart when mapped into an integer field — it is a lossy
// narrowing.
func TestMapperDecimalOverflowNarrowing(t *testing.T) {
	t.Parallel()

	type result struct {
		N int64 `mapstructure:"n"`
	}
	mapper, err := rlng.NewMapper[result](rlng.MappingTemplate{"n": "big"})
	require.NoError(t, err)

	_, err = mapper.Map(map[string]any{"big": decimal.RequireFromString("99999999999999999999")})
	require.Error(t, err)
	assert.ErrorIs(t, err, rlng.ErrLossyResultNarrowing)
}

// TestMapperDecimalNarrowUint64AboveInt64 guards against a false rejection: an
// integer decimal in (MaxInt64, MaxUint64] fits a uint64 result field exactly,
// so it must decode, not error. Regression for the audit finding (unsigned
// kinds shared the signed IsInt64 guard).
func TestMapperDecimalNarrowUint64AboveInt64(t *testing.T) {
	t.Parallel()

	type result struct {
		N uint64 `mapstructure:"n"`
	}
	mapper, err := rlng.NewMapper[result](rlng.MappingTemplate{"n": "big"})
	require.NoError(t, err)

	// 2^63 = MaxInt64 + 1: above int64 range, exactly representable as uint64.
	got, err := mapper.Map(map[string]any{"big": decimal.RequireFromString("9223372036854775808")})
	require.NoError(t, err)
	assert.Equal(t, uint64(9223372036854775808), got.N)

	// A negative decimal has no uint64 representation and must still fail loud.
	_, err = mapper.Map(map[string]any{"big": decimal.RequireFromString("-5")})
	require.Error(t, err)
	assert.ErrorIs(t, err, rlng.ErrLossyResultNarrowing)
}
