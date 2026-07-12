package config_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng/config"
	"github.com/kartaladev/rlng/pipe"
)

func ExampleParseYAML() {
	const src = `
stages:
  - name: base
    type: single-expr
    expr: price * qty
  - name: taxed
    type: single-expr
    expr: base * 1.1
    depends_on: [base]
`
	def, err := config.ParseYAML([]byte(src))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	p, err := def.Build()
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	sc := pipe.NewScope(map[string]any{"price": 10.0, "qty": 2.0})
	if err := p.Run(context.Background(), sc); err != nil {
		fmt.Println("run:", err)
		return
	}
	v, _ := sc.Get("taxed")
	fmt.Printf("%.1f\n", v)
	// Output: 22.0
}
