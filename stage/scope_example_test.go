package stage_test

import (
	"fmt"

	"github.com/kartaladev/rlng/stage"
)

func ExampleScope() {
	s := stage.NewScope(map[string]any{"amount": 150})
	_ = s.Set("discount.rate", 0.1)

	rate, _ := s.Get("discount.rate")
	fmt.Println(rate)
	// Output: 0.1
}
