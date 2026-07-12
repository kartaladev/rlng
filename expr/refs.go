package expr

import (
	"sort"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// references returns the sorted, unique top-level identifiers read by src. src is
// already known to compile, so a parse failure yields nil. It is computed once at
// compile time and cached on the Function/Predicate.
func references(src string) []string {
	tree, err := parser.Parse(src)
	if err != nil {
		return nil
	}
	v := &refVisitor{seen: map[string]struct{}{}, callees: map[string]struct{}{}}
	node := tree.Node
	ast.Walk(&node, v)
	if len(v.seen) == 0 {
		return nil
	}
	refs := make([]string, 0, len(v.seen))
	for name := range v.seen {
		// A call callee (e.g. `discount` in `discount(x)`) is a function name
		// supplied by the env, not a data field the expression reads; exclude it
		// so References reports only value inputs. (Builtins like len are
		// BuiltinNode, already not IdentifierNodes.)
		if _, isCallee := v.callees[name]; isCallee {
			continue
		}
		refs = append(refs, name)
	}
	if len(refs) == 0 {
		return nil
	}
	sort.Strings(refs)
	return refs
}

type refVisitor struct {
	seen    map[string]struct{}
	callees map[string]struct{}
}

func (r *refVisitor) Visit(node *ast.Node) {
	switch n := (*node).(type) {
	case *ast.IdentifierNode:
		r.seen[n.Value] = struct{}{}
	case *ast.CallNode:
		if id, ok := n.Callee.(*ast.IdentifierNode); ok {
			r.callees[id.Value] = struct{}{}
		}
	}
}
