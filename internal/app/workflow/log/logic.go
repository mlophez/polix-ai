package log

import "github.com/mlophez/polix-ai/internal/app/state"

// Output is the decision of the logic of LogWorkflow: the entries of the log to show, in file order.
type Output struct {
	Entries []state.Entry
}

// Decide picks the entries of the log to show: the last tail of them, or all when tail is 0 or less or the log
// holds no more.
func Decide(st state.State, tail int) Output {
	entries := st.Entries
	if tail > 0 && len(entries) > tail {
		entries = entries[len(entries)-tail:]
	}
	return Output{Entries: entries}
}
