package pipe

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"

	"github.com/shopspring/decimal"
)

// errNotNumericKind reports that a reflect.Value is not one of the supported
// numeric kinds. Callers translate it into their own contextual message (a
// *ScopeTypeError for the coercing getters, a classification-guaranteed
// unreachable for the aggregation folds).
var errNotNumericKind = errors.New("value is not a numeric kind")

// int64FromNumeric converts an integer-kind reflect.Value to int64, checking
// that a uint kind does not exceed math.MaxInt64 (which would wrap). A
// non-integer kind is errNotNumericKind. This is the single overflow-checked
// int64 conversion shared by pipe/get.go (coercing getters) and pipe/table.go
// (integer aggregation folds); its divergence previously caused pipe bug #3.
func int64FromNumeric(rv reflect.Value) (int64, error) {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := rv.Uint()
		if u > math.MaxInt64 {
			return 0, fmt.Errorf("uint64(%d) overflows int64", u)
		}
		return int64(u), nil
	default:
		return 0, errNotNumericKind
	}
}

// float64FromNumeric converts an integer- or float-kind reflect.Value to
// float64 (integer magnitudes above 2^53 may lose precision, inherent to
// float64). A non-numeric kind is errNotNumericKind.
func float64FromNumeric(rv reflect.Value) (float64, error) {
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		return rv.Float(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(rv.Uint()), nil
	default:
		return 0, errNotNumericKind
	}
}

// decimalFromNumeric converts an integer- or finite-float-kind reflect.Value to
// decimal.Decimal, preserving the full uint64 range via big.Int (int64(u) would
// wrap). ok is false for a non-finite float (NaN/±Inf — no decimal form; passing
// it to decimal.NewFromFloat panics) or a non-numeric kind.
func decimalFromNumeric(rv reflect.Value) (decimal.Decimal, bool) {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return decimal.NewFromInt(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return decimal.NewFromBigInt(new(big.Int).SetUint64(rv.Uint()), 0), true
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return decimal.Decimal{}, false
		}
		return decimal.NewFromFloat(f), true
	default:
		return decimal.Decimal{}, false
	}
}
