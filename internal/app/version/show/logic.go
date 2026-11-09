package show

// Output is the decision of the logic of ShowVersion: the version to print.
type Output struct {
	Version string
}

// Decide decides what polix version prints: the version of the running binary, as it is ("dev" for a
// development build).
func Decide(running string) Output {
	return Output{Version: running}
}
