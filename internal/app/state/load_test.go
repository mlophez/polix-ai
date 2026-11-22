package state

import (
	"errors"
	"testing"
)

func TestLoadedValid(t *testing.T) {
	boom := errors.New("boom")
	records := []Record{{Legacy: "decision"}}
	for _, tc := range []struct {
		name   string
		loaded Loaded
		valid  bool
	}{
		{"no log", Loaded{}, true},
		{"a log", Loaded{State: State{Records: records, Skipped: 1}}, true},
		{"a log that cannot be read", Loaded{Err: boom}, true},
		{"records with the error", Loaded{State: State{Records: records}, Err: boom}, false},
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
	l := Loaded{State: State{Records: []Record{{Legacy: "decision"}}, Skipped: 2}}
	failed := l.Fail(boom)
	if len(failed.State.Records) != 0 || failed.State.Skipped != 0 || !errors.Is(failed.Err, boom) ||
		failed.Valid() != nil {
		t.Fatalf("Fail = %+v", failed)
	}
	if len(l.State.Records) != 1 || l.Err != nil {
		t.Fatal("Fail returns a copy")
	}
}

// TestStateLoaded tests the state a log loads: its records and the count of skipped lines, kept as read; an empty
// state with why for a log that cannot be read or a state that is not valid.
func TestStateLoaded(t *testing.T) {
	// The records and the count of skipped lines make the state, kept as read.
	t.Run("the state as read", func(t *testing.T) {
		s := State{Records: []Record{{Legacy: "fact", Fact: "reviewed"}, {Actor: "x"}}, Skipped: 1}
		got := s.Loaded(nil)
		if len(got.State.Records) != 2 || got.State.Records[1].Actor != "x" || got.State.Skipped != 1 || got.Err != nil {
			t.Fatalf("got %+v", got)
		}
		if got := (State{}).Loaded(nil); len(got.State.Records) != 0 || got.Err != nil {
			t.Fatalf("an empty log: %+v", got)
		}
	})

	// A log that cannot be read loads an empty state with why.
	t.Run("unreadable", func(t *testing.T) {
		boom := errors.New("boom")
		got := State{Records: []Record{{Legacy: "decision"}}, Skipped: 1}.Loaded(boom)
		if len(got.State.Records) != 0 || got.State.Skipped != 0 || !errors.Is(got.Err, boom) {
			t.Fatalf("got %+v", got)
		}
	})

	// A state that is not valid loads an empty state with why.
	t.Run("invalid", func(t *testing.T) {
		got := State{Records: []Record{{Legacy: "decision"}}, Skipped: -1}.Loaded(nil)
		if len(got.State.Records) != 0 || got.Err == nil {
			t.Fatalf("got %+v", got)
		}
	})
}
