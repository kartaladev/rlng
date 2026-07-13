package expr

import (
	"sort"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// references returns the sorted, unique referenced paths read by src: the deepest
// statically-known member path per reference ("a.b.c"), or the bare identifier
// when the chain is not statically resolvable (dynamic/index access, method
// calls). src is already known to compile, so a parse failure yields nil. It is
// computed once at compile time and cached on the Function/Predicate.
func references(src string) []string {
	tree, err := parser.Parse(src)
	if err != nil {
		return nil
	}
	v := &refVisitor{paths: map[string]struct{}{}, callees: map[string]struct{}{}}
	node := tree.Node
	ast.Walk(&node, v)
	if len(v.paths) == 0 {
		return nil
	}
	// Exclude call callees (function names supplied by the env, not data fields)
	// and any path that is a strict prefix (proper ancestor) of another collected
	// path — the deepest static path wins ("a", "a.b" drop out for "a.b.c").
	all := make([]string, 0, len(v.paths))
	for p := range v.paths {
		if _, isCallee := v.callees[p]; isCallee {
			continue
		}
		all = append(all, p)
	}
	refs := make([]string, 0, len(all))
	for _, p := range all {
		if isStrictPrefixOfAny(p, all) {
			continue
		}
		refs = append(refs, p)
	}
	if len(refs) == 0 {
		return nil
	}
	sort.Strings(refs)
	return refs
}

type refVisitor struct {
	paths   map[string]struct{}
	callees map[string]struct{}
}

func (r *refVisitor) Visit(node *ast.Node) {
	switch n := (*node).(type) {
	case *ast.IdentifierNode:
		r.paths[n.Value] = struct{}{}
	case *ast.MemberNode:
		if p, ok := staticPath(*node); ok {
			r.paths[p] = struct{}{}
		}
	case *ast.CallNode:
		// A call callee (e.g. `discount` in `discount(x)`) is a function name
		// supplied by the env, not a data field; exclude it. Method-call members
		// (`foo.bar()`, MemberNode.Method) are never recorded as a path by
		// staticPath, so only the receiver identifier survives. (Builtins like len
		// are BuiltinNode, already not IdentifierNodes.)
		if id, ok := n.Callee.(*ast.IdentifierNode); ok {
			r.callees[id.Value] = struct{}{}
		}
	}
}

// staticPath returns the dot-path of a fully static member chain rooted at an
// identifier ("a", "a.b", `a["b"]`), or ok=false for a dynamic/index property
// (a[i], a[0]) or a method-call member (foo.bar()). Both `a.b` and `a["b"]`
// decode to a StringNode property, so bracket-string access is a static path;
// a non-string property ends the chain.
func staticPath(n ast.Node) (string, bool) {
	switch t := n.(type) {
	case *ast.IdentifierNode:
		return t.Value, true
	case *ast.MemberNode:
		if t.Method {
			return "", false
		}
		prop, ok := t.Property.(*ast.StringNode)
		if !ok {
			return "", false
		}
		base, ok := staticPath(t.Node)
		if !ok {
			return "", false
		}
		return base + "." + prop.Value, true
	default:
		return "", false
	}
}

// isStrictPrefixOfAny reports whether p is a proper ancestor path of some other
// entry in all (some q equals p + "." + ...), so intermediate paths ("a", "a.b"
// for "a.b.c") drop out in favor of the deepest static path.
func isStrictPrefixOfAny(p string, all []string) bool {
	for _, q := range all {
		if len(q) > len(p) && strings.HasPrefix(q, p+".") {
			return true
		}
	}
	return false
}
