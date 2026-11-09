// Package drop: the use case has no in.go (GOARCH-3).
package drop

type Output struct{}

func Decide() Output { return Output{} }
