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
	v := &refVisitor{seen: map[string]struct{}{}}
	node := tree.Node
	ast.Walk(&node, v)
	if len(v.seen) == 0 {
		return nil
	}
	refs := make([]string, 0, len(v.seen))
	for name := range v.seen {
		refs = append(refs, name)
	}
	sort.Strings(refs)
	return refs
}

type refVisitor struct{ seen map[string]struct{} }

func (r *refVisitor) Visit(node *ast.Node) {
	if id, ok := (*node).(*ast.IdentifierNode); ok {
		r.seen[id.Value] = struct{}{}
	}
}
