package expr

import (
	"fmt"

	exprlang "github.com/expr-lang/expr"
	"github.com/shopspring/decimal"
)

// toDecimal converts a supported scalar to a decimal.Decimal. A string is parsed
// exactly; int/float are converted; a decimal passes through. Anything else is an
// error, surfaced from the expression as an eval error.
func toDecimal(v any) (decimal.Decimal, error) {
	switch x := v.(type) {
	case string:
		return decimal.NewFromString(x)
	case int:
		return decimal.NewFromInt(int64(x)), nil
	case int64:
		return decimal.NewFromInt(x), nil
	case float64:
		return decimal.NewFromFloat(x), nil
	case decimal.Decimal:
		return x, nil
	default:
		return decimal.Decimal{}, fmt.Errorf("decimal: unsupported type %T", v)
	}
}

// decimalExprOptions returns the expr-lang options that make the exact-decimal
// value type usable inside every compiled expression: a decimal(x) constructor,
// arithmetic operator overloads (decimal×decimal and mixed decimal×int / int×
// decimal), and rounding builtins. Operator overloads resolve at COMPILE time by
// static operand type: decimal(...) yields a statically decimal value, so wrapped
// arithmetic is exact in any mode; bare-variable arithmetic requires strict env
// (WithEnv). The names decimal, round, roundBank are reserved.
func decimalExprOptions() []exprlang.Option {
	dd := func(f func(a, b decimal.Decimal) decimal.Decimal) func(...any) (any, error) {
		return func(p ...any) (any, error) {
			a, err := toDecimal(p[0])
			if err != nil {
				return nil, err
			}
			b, err := toDecimal(p[1])
			if err != nil {
				return nil, err
			}
			return f(a, b), nil
		}
	}
	return []exprlang.Option{
		exprlang.Function("decimal", func(p ...any) (any, error) { return toDecimal(p[0]) },
			new(func(string) decimal.Decimal),
			new(func(int) decimal.Decimal),
			new(func(float64) decimal.Decimal),
			new(func(decimal.Decimal) decimal.Decimal)),

		// + - * / for decimal×decimal, decimal×int, int×decimal.
		exprlang.Function("addDecimal", dd(decimal.Decimal.Add),
			new(func(decimal.Decimal, decimal.Decimal) decimal.Decimal),
			new(func(decimal.Decimal, int) decimal.Decimal),
			new(func(int, decimal.Decimal) decimal.Decimal)),
		exprlang.Function("subDecimal", dd(decimal.Decimal.Sub),
			new(func(decimal.Decimal, decimal.Decimal) decimal.Decimal),
			new(func(decimal.Decimal, int) decimal.Decimal),
			new(func(int, decimal.Decimal) decimal.Decimal)),
		exprlang.Function("mulDecimal", dd(decimal.Decimal.Mul),
			new(func(decimal.Decimal, decimal.Decimal) decimal.Decimal),
			new(func(decimal.Decimal, int) decimal.Decimal),
			new(func(int, decimal.Decimal) decimal.Decimal)),
		exprlang.Function("divDecimal", dd(decimal.Decimal.Div),
			new(func(decimal.Decimal, decimal.Decimal) decimal.Decimal),
			new(func(decimal.Decimal, int) decimal.Decimal),
			new(func(int, decimal.Decimal) decimal.Decimal)),
		exprlang.Operator("+", "addDecimal"),
		exprlang.Operator("-", "subDecimal"),
		exprlang.Operator("*", "mulDecimal"),
		exprlang.Operator("/", "divDecimal"),

		// round (half-away-from-zero) and roundBank (half-even / banker's).
		exprlang.Function("round", func(p ...any) (any, error) {
			d, err := toDecimal(p[0])
			if err != nil {
				return nil, err
			}
			return d.Round(int32(p[1].(int))), nil
		}, new(func(decimal.Decimal, int) decimal.Decimal)),
		exprlang.Function("roundBank", func(p ...any) (any, error) {
			d, err := toDecimal(p[0])
			if err != nil {
				return nil, err
			}
			return d.RoundBank(int32(p[1].(int))), nil
		}, new(func(decimal.Decimal, int) decimal.Decimal)),
	}
}
