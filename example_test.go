package rlng_test

import (
	"context"
	"fmt"

	"github.com/kartaladev/rlng"
	"github.com/kartaladev/rlng/pipe"
)

type input struct {
	Price float64 `mapstructure:"price"`
	Qty   int     `mapstructure:"qty"`
}

type result struct {
	Total float64 `mapstructure:"total"`
}

func ExampleTypedEngine() {
	base, _ := pipe.NewSingleExpr("base", "price * qty")
	taxed, _ := pipe.NewSingleExpr("taxed", "base * 1.1", pipe.WithDependsOn("base"))
	pipeline, _ := pipe.NewPipeline([]pipe.Stage{base, taxed})

	mapper, _ := rlng.NewMapper[result](rlng.MappingTemplate{"total": "taxed"})
	engine, _ := rlng.NewTypedEngine[input, result](pipeline, mapper)

	out, err := engine.Evaluate(context.Background(), input{Price: 10, Qty: 2})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%.1f\n", out.Total)
	// Output: 22.0
}
