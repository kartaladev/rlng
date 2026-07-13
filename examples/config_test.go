package examples_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/config"
)

// Example_configEngine loads a pipeline from a JSON definition and runs it
// through an Engine, which returns the raw accumulated map[string]any.
func Example_configEngine() {
	def, err := config.Parse(context.Background(), config.FromJSONString(`{
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

	engine, err := rlng.New(pipeline)
	if err != nil {
		fmt.Println("engine:", err)
		return
	}
	out, err := engine.Evaluate(context.Background(), map[string]any{"price": 10, "qty": 2})
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	fmt.Printf("base=%v taxed=%.1f\n", out["base"], out["taxed"])

	// Output:
	// base=20 taxed=22.0
}

// Example_configEngine_yaml loads the same kind of pipeline from a YAML
// definition (stage expressions use the scalar-shorthand string form) and
// runs it through an Engine.
func Example_configEngine_yaml() {
	def, err := config.Parse(context.Background(), config.FromYAMLString(`
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

	engine, err := rlng.New(pipeline)
	if err != nil {
		fmt.Println("engine:", err)
		return
	}
	out, err := engine.Evaluate(context.Background(), map[string]any{"price": 10, "qty": 2})
	if err != nil {
		fmt.Println("evaluate:", err)
		return
	}
	fmt.Printf("base=%v taxed=%.1f\n", out["base"], out["taxed"])

	// Output:
	// base=20 taxed=22.0
}
