package examples_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/config"
)

// Example_configBareEngine loads a pipeline from a JSON definition and runs it
// through a BareEngine, which returns the raw accumulated map[string]any.
func Example_configBareEngine() {
	def, err := config.ParseJSON([]byte(`{
		"stages": [
			{"name": "base", "type": "single-expr", "expr": "price * qty"},
			{"name": "taxed", "type": "single-expr", "expr": "base * 1.1", "depends_on": ["base"]}
		]
	}`))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	pipeline, err := def.Build()
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	engine := rlng.NewBareEngine(pipeline)
	out, err := engine.Evaluate(context.Background(), map[string]any{"price": 10, "qty": 2})
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	fmt.Printf("base=%v taxed=%.1f\n", out["base"], out["taxed"])

	// Output:
	// base=20 taxed=22.0
}

// Example_configBareEngine_yaml loads the same kind of pipeline from a YAML
// definition (stage expressions use the scalar-shorthand string form) and
// runs it through a BareEngine.
func Example_configBareEngine_yaml() {
	def, err := config.ParseYAML([]byte(`
stages:
  - name: base
    type: single-expr
    expr: price * qty
  - name: taxed
    type: single-expr
    expr: base * 1.1
    depends_on: [base]
`))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	pipeline, err := def.Build()
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	engine := rlng.NewBareEngine(pipeline)
	out, err := engine.Evaluate(context.Background(), map[string]any{"price": 10, "qty": 2})
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	fmt.Printf("base=%v taxed=%.1f\n", out["base"], out["taxed"])

	// Output:
	// base=20 taxed=22.0
}
