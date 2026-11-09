package state

import (
	"errors"
	"testing"
)

func TestLoadedValid(t *testing.T) {
	boom := errors.New("boom")
	entries := []Entry{{Kind: KindDecision}}
	for _, tc := range []struct {
		name   string
		loaded Loaded
		valid  bool
	}{
		{"no log", Loaded{}, true},
		{"a log", Loaded{State: State{Entries: entries, Skipped: 1}}, true},
		{"a log that cannot be read", Loaded{Err: boom}, true},
		{"entries with the error", Loaded{State: State{Entries: entries}, Err: boom}, false},
		{"skipped lines with the error", Loaded{State: State{Skipped: 1}, Err: boom}, false},
		{"a state that is not valid", Loaded{State: State{Skipped: -1}}, false},
	} {
		if err := tc.loaded.Valid(); (err == nil) != tc.valid {
			t.Errorf("%s: Valid() = %v, want valid %v", tc.name, err, tc.valid)
		}
	}
}

// Fail empties the state and keeps why, on a copy.
func TestLoadedFail(t *testing.T) {
	boom := errors.New("boom")
	l := Loaded{State: State{Entries: []Entry{{Kind: KindDecision}}, Skipped: 2}}
	failed := l.Fail(boom)
	if len(failed.State.Entries) != 0 || failed.State.Skipped != 0 || !errors.Is(failed.Err, boom) ||
		failed.Valid() != nil {
		t.Fatalf("Fail = %+v", failed)
	}
	if len(l.State.Entries) != 1 || l.Err != nil {
		t.Fatal("Fail returns a copy")
	}
}
