package count

import "time"

// Threshold is exported by a sibling of logic.go, which exports no name (GOARCH-23).
const Threshold = 3

// audit reads the clock in a sibling of logic.go (GOARCH-4), and calls the port through the method box promotes
// from a file that is not an in file (GOARCH-32).
func audit(b box) bool {
	_ = b.Close()
	return time.Now().IsZero()
}
