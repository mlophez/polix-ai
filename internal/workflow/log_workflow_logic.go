package workflow

import "github.com/mlophez/polix-ai/internal/state"

// LogWorkflowOutput is the decision of the logic of LogWorkflow: the entries of the log to show, in file order.
type LogWorkflowOutput struct {
	Entries []state.Entry
}

// LogWorkflow picks the entries of the log to show: the last tail of them, or all when tail is 0 or less or the
// log holds no more.
func LogWorkflow(st state.State, tail int) LogWorkflowOutput {
	entries := st.Entries
	if tail > 0 && len(entries) > tail {
		entries = entries[len(entries)-tail:]
	}
	return LogWorkflowOutput{Entries: entries}
}
