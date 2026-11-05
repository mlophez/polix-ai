package version

// ShowVersionOutput is the decision of the logic of ShowVersion: the version to print.
type ShowVersionOutput struct {
	Version string
}

// ShowVersion decides what polix version prints: the version of the running binary, as it is ("dev" for a
// development build).
func ShowVersion(running string) ShowVersionOutput {
	return ShowVersionOutput{Version: running}
}
