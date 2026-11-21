// Package place is a use case whose logic imports context, which GOARCH-41-logic denies (GOARCH-41).
package place

import "context"

// Decide takes a context, which the logic never needs.
func Decide(ctx context.Context) bool { return ctx.Err() == nil }
