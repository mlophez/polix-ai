package stock

import "context"

// Load is the loader of the load use case stock/load, a named function type of the domain (GOARCH-11).
type Load func(ctx context.Context, shop string) Loaded

// Loaded is what Load returns, a struct of the domain with its Valid (GOARCH-11, GOARCH-26).
type Loaded struct {
	Count int
	Err   error
}

// Valid tells why l is not what a shop loads: a negative count.
func (l Loaded) Valid() error {
	if l.Count < 0 {
		return context.Canceled
	}
	return nil
}
