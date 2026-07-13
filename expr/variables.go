package expr

import (
	"math"
	"reflect"

	"github.com/expr-lang/expr/ast"
	"github.com/shopspring/decimal"
)

// decimalDecimalType is decimal.Decimal's reflect.Type, used by Visit to
// recognize a decimal global/local so it can be patched as a `decimal("…")`
// call default instead of being skipped as a non-scalar struct.
var decimalDecimalType = reflect.TypeOf(decimal.Decimal{})

// variablePatcher rewrites each identifier that matches a declared variable
// into `identifier ?? <literal>`, so the variable acts as a default overridable
// by the runtime environment. Lookup precedence is locals, then globals. Scalar
// values become literals; a decimal.Decimal becomes a `decimal("<string>")`
// call (the decimal builtin is always registered, see decimalExprOptions);
// anything else is skipped (the identifier is then a normal, undefined-allowed
// lookup).
type variablePatcher struct {
	globals map[string]any
	locals  map[string]any
}

// newPatcher returns a patcher, or nil when no variables are declared so the
// caller can omit exprlang.Patch entirely.
func newPatcher(globals, locals map[string]any) *variablePatcher {
	if len(globals) == 0 && len(locals) == 0 {
		return nil
	}
	return &variablePatcher{globals: mergeInto(nil, globals), locals: mergeInto(nil, locals)}
}

func (v *variablePatcher) lookup(name string) (any, bool) {
	if val, ok := v.locals[name]; ok {
		return val, true
	}
	val, ok := v.globals[name]
	return val, ok
}

// Visit implements ast.Visitor.
func (v *variablePatcher) Visit(node *ast.Node) {
	ident, ok := (*node).(*ast.IdentifierNode)
	if !ok {
		return
	}
	value, found := v.lookup(ident.Value)
	if !found {
		return
	}

	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return
	}

	if rv.Type() == decimalDecimalType {
		d := rv.Interface().(decimal.Decimal)
		call := &ast.CallNode{
			Callee:    &ast.IdentifierNode{Value: "decimal"},
			Arguments: []ast.Node{&ast.StringNode{Value: d.String()}},
		}
		ast.Patch(node, &ast.BinaryNode{Operator: "??", Left: ident, Right: call})
		return
	}

	var literal ast.Node
	switch rv.Kind() {
	case reflect.Bool:
		literal = &ast.BoolNode{Value: rv.Bool()}
	case reflect.String:
		literal = &ast.StringNode{Value: rv.String()}
	case reflect.Float32, reflect.Float64:
		literal = &ast.FloatNode{Value: rv.Float()}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Guard against truncation on 32-bit builds, where int is narrower
		// than int64; on 64-bit this range check is always satisfied.
		if i := rv.Int(); i >= math.MinInt && i <= math.MaxInt {
			literal = &ast.IntegerNode{Value: int(i)}
		} else {
			return // overflows ast.IntegerNode.Value (int): skip silently
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if rv.Uint() > math.MaxInt {
			return // overflows ast.IntegerNode.Value (int): skip silently
		}
		literal = &ast.IntegerNode{Value: int(rv.Uint())}
	default:
		return // non-scalar: skip silently (no global logging)
	}
	literal.SetType(rv.Type())

	ast.Patch(node, &ast.BinaryNode{Operator: "??", Left: ident, Right: literal})
}
