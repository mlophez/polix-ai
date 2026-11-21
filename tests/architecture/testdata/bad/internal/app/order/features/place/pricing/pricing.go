// Package pricing is a helper of the use case place, whose logic imports it, so it is as pure as the logic
// (GOARCH-4); the domain cart, the use case cancel and the helper tax import it too (GOARCH-9).
package pricing

import (
	// GOARCH-4: os is out of the list GOARCH-41-logic, which a helper of the logic follows.
	"os"
	"time"
)

// Output is a name reserved to the use cases (GOARCH-23).
type Output struct{}

// Rate reads the environment and the clock (GOARCH-4).
func Rate() int {
	_ = os.Getenv("RATE")
	_ = time.Now()
	return 1
}
