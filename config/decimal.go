package config

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// ErrDecimalLiteral is returned when a {"$dec": "…"} config literal does not
// parse as an exact decimal.
var ErrDecimalLiteral = errors.New("config: invalid $dec decimal literal")

// decLiteralKey is the reserved sole-key marker of a decimal literal object
// ({"$dec": "<string>"}), shared by hydration and canonical hashing.
const decLiteralKey = "$dec"

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
		// A map whose sole key is the reserved "$dec" marker is a decimal
		// literal: its value MUST be a string. A non-string value (e.g. an
		// unquoted number that YAML/JSON decoded to a float/int) is a malformed
		// literal — fail loud rather than silently treating it as an ordinary
		// map, which would mask an author's typo.
		if raw, hasDec := x[decLiteralKey]; hasDec && len(x) == 1 {
			s, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("%w: $dec value must be a quoted string, got %T (%v)", ErrDecimalLiteral, raw, raw)
			}
			d, err := decimal.NewFromString(s)
			if err != nil {
				return nil, fmt.Errorf("%w: %q: %v", ErrDecimalLiteral, s, err)
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
