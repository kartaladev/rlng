package config

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// ErrDecimalLiteral is returned when a {"$dec": "…"} config literal does not
// parse as an exact decimal.
var ErrDecimalLiteral = errors.New("config: invalid $dec decimal literal")

// hydrateDecimals recursively replaces every {"$dec": "<string>"} object in m
// with a decimal.Decimal, in place. The object form is used because a YAML
// !decimal tag collapses to a plain string when decoded into map[string]any.
func hydrateDecimals(m map[string]any) error {
	for k, v := range m {
		nv, err := hydrateValue(v)
		if err != nil {
			return err
		}
		m[k] = nv
	}
	return nil
}

func hydrateValue(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		if raw, ok := decLiteral(x); ok {
			d, err := decimal.NewFromString(raw)
			if err != nil {
				return nil, fmt.Errorf("%w: %q: %v", ErrDecimalLiteral, raw, err)
			}
			return d, nil
		}
		if err := hydrateDecimals(x); err != nil {
			return nil, err
		}
		return x, nil
	case []any:
		for i := range x {
			nv, err := hydrateValue(x[i])
			if err != nil {
				return nil, err
			}
			x[i] = nv
		}
		return x, nil
	default:
		return v, nil
	}
}

// decLiteral reports whether m is exactly {"$dec": "<string>"} and returns the
// string. A map with $dec plus other keys is NOT a literal (returns false).
func decLiteral(m map[string]any) (string, bool) {
	if len(m) != 1 {
		return "", false
	}
	s, ok := m["$dec"].(string)
	return s, ok
}
