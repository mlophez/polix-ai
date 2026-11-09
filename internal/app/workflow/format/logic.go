package format

// Output is the decision of the logic of FormatWorkflow: what to write to polix.yaml, or why nothing.
type Output struct {
	Missing bool   // there is no polix.yaml: nothing to format
	Problem string // why polix.yaml cannot be formatted; empty when it can
	Changed bool   // the formatted polix.yaml differs from the one on disk
	Write   []byte // the formatted polix.yaml to write; nil when nothing is written
}

// Decide decides how to format polix.yaml, whose data is data and whose existence is found: without the file
// nothing; when formatData fails, its error as the problem; else whether the format changes it, and the data to
// write when it does, unless check only asks whether it is formatted.
func Decide(data []byte, found, check bool) Output {
	if !found {
		return Output{Missing: true}
	}
	formatted, err := formatData(data)
	if err != nil {
		return Output{Problem: err.Error()}
	}
	out := Output{Changed: string(formatted) != string(data)}
	if out.Changed && !check {
		out.Write = formatted
	}
	return out
}
