package state

import (
	"context"
	"errors"
)

// Load loads the log at path. It is the method Load of stateadapters.Log (internal/app/state/adapters), which cmd/polix
// builds once and passes to every use case that reads the log. It never fails: what stops the reading is data of its
// result.
type Load func(ctx context.Context, path string) Loaded

// Loaded is what Load read from the log: its state, empty without the file, or why it cannot be read.
type Loaded struct {
	State State // the log as read; empty without the file or with Err
	Err   error // why the log cannot be read; nil when it can or does not exist
}

// MaxLine is the longest line of the log that is read; longer lines are skipped.
const MaxLine = 4 * 1024 * 1024

// MaxReason is the longest reason stored in an action; the writer of the log cuts longer reasons, so every line stays
// far below MaxLine.
const MaxReason = 8 * 1024

// Valid tells why l is not what the log loads: a state with the error of its reading, or a state that is not valid.
func (l Loaded) Valid() error {
	if l.Err != nil && (len(l.State.Records) > 0 || l.State.Skipped != 0) {
		return errors.New("a state loaded from a log that cannot be read")
	}
	return l.State.Valid()
}

// Fail returns l failed with err: an empty state, and why.
func (l Loaded) Fail(err error) Loaded {
	failed := l
	failed.State, failed.Err = State{}, err
	return failed
}

// Loaded builds what the log loads from s, the state of the records of its lines that decode and the count of the
// lines skipped, and err, why the log cannot be read. It validates them with the domain (State.Valid and
// Loaded.Valid): a log that cannot be read, or a state that is not valid, loads an empty state with why.
func (s State) Loaded(err error) Loaded {
	if err != nil {
		return Loaded{}.Fail(err)
	}
	if err := s.Valid(); err != nil {
		return Loaded{}.Fail(err)
	}
	loaded := Loaded{State: s}
	if err := loaded.Valid(); err != nil {
		return Loaded{}.Fail(err)
	}
	return loaded
}
