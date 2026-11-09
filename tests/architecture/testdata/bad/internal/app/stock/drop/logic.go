// Package drop: the use case has no in.go (GOARCH-3), and its Decide takes five parameters (GOARCH-45).
package drop

type Output struct{}

func Decide(_, _ int, _, _, _ string) Output { return Output{} }
