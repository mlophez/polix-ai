package load

import "github.com/mlophez/polix-ai/internal/app/state"

// Output is the decision of LoadState: what the log loads.
type Output struct {
	Loaded state.Loaded
}

// Decide builds the state of the log from the entries of its lines that decode and the count of the lines skipped,
// validated with the domain (State.Valid and Loaded.Valid): a log that cannot be read, or a state that is not valid,
// loads an empty state with why.
func Decide(entries []state.Entry, skipped int, err error) Output {
	if err != nil {
		return Output{Loaded: state.Loaded{}.Fail(err)}
	}
	s := state.State{Entries: entries, Skipped: skipped}
	if err := s.Valid(); err != nil {
		return Output{Loaded: state.Loaded{}.Fail(err)}
	}
	loaded := state.Loaded{State: s}
	if err := loaded.Valid(); err != nil {
		return Output{Loaded: state.Loaded{}.Fail(err)}
	}
	return Output{Loaded: loaded}
}
