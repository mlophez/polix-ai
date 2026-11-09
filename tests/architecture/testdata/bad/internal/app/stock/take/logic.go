package take

type Output struct{ Count int }

// Decide takes a predeclared type and returns its output (GOARCH-23), through a helper of a sibling of logic.go.
func Decide(raw string) Output { return Output{Count: count(raw)} }
