package load

import "context"

// Adapter implements the port (GOARCH-5, GOARCH-24).
type Adapter struct{}

// Read is the method of the port.
func (a Adapter) Read(_ context.Context, shop string) (int, error) { return len(shop), nil }
