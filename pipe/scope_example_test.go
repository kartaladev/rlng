package pipe_test

import (
	"fmt"

	"github.com/kartaladev/rlng/pipe"
)

func ExampleScope() {
	s := pipe.NewScope(map[string]any{"amount": 150})
	_ = s.Set("discount.rate", 0.1)

	rate, _ := s.Get("discount.rate")
	fmt.Println(rate)
	// Output: 0.1
}
