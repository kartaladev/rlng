package rlng_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/stage"
)

type input struct {
	Price float64 `mapstructure:"price"`
	Qty   int     `mapstructure:"qty"`
}

type result struct {
	Total float64 `mapstructure:"total"`
}

func ExampleEngine() {
	base, _ := stage.NewSingleExpr("base", "price * qty")
	taxed, _ := stage.NewSingleExpr("taxed", "base * 1.1", stage.WithDependsOn("base"))
	pipeline, _ := stage.NewPipeline(base, taxed)

	mapper, _ := rlng.NewMapper[result](rlng.MappingTemplate{"total": "taxed"})
	engine := rlng.New[input, result](pipeline, mapper)

	out, err := engine.Evaluate(context.Background(), input{Price: 10, Qty: 2})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%.1f\n", out.Total)
	// Output: 22.0
}
