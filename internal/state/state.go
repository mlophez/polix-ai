// Package state is the domain of the append-only log of polix in <git-dir>/polix/log.jsonl: the facts it records
// (a reviewer finished, the tests passed) tied to the content hash they were observed on, and every decision it
// takes, which also serves as evidence of the process each change followed.
//
// Only polix writes it, from events it has seen; the agent never declares that something is done. Reading and
// writing the file is the job of the adapters, through pkg/jsonl; this package decodes and encodes its lines.
package state

import (
	"fmt"
	"slices"
)

// State is the log as read: its entries in file order and how many lines were skipped for being over MaxLine.
type State struct {
	Entries []Entry
	Skipped int
}

// DirName is the folder of the state inside the git directory, so it is never versioned and goes with the
// branch or worktree.
const DirName = "polix"

// LogName is the name of the log inside DirName.
const LogName = "log.jsonl"

// Valid tells why the state is not valid: a negative count of skipped lines. Its entries are what the log
// holds, kept as read even when an entry would not be valid to append.
func (s State) Valid() error {
	if s.Skipped < 0 {
		return fmt.Errorf("skipped lines must not be negative, got %d", s.Skipped)
	}
	return nil
}

// Append returns a copy of the state with e at the end, as the log reads after appending it, or the error of
// an entry that is not valid.
func (s State) Append(e Entry) (State, error) {
	if err := e.Valid(); err != nil {
		return s, err
	}
	entries := slices.Clone(s.Entries)
	entries = append(entries, e)
	out := State{Entries: entries, Skipped: s.Skipped}
	if err := out.Valid(); err != nil {
		return s, err
	}
	return out, nil
}

// Done reports whether fact was recorded on the branch for exactly this content hash; never for an unknown
// (empty) hash.
func (s State) Done(fact, branch, hash string) bool {
	if hash == "" {
		return false
	}
	for _, e := range s.Entries {
		if e.Kind == KindFact && e.Fact == fact && e.Branch == branch && e.Hash == hash {
			return true
		}
	}
	return false
}

// Injected reports whether rule was already injected as context in session.
func (s State) Injected(rule, session string) bool {
	for _, e := range s.Entries {
		if e.Kind == KindInjected && e.Rule == rule && e.Session == session {
			return true
		}
	}
	return false
}

// StopBlocks counts the times the workflow entry at line blocked event (stop or subagent_stop) of actor in
// session on this content hash: the loop guard. Counting per entry keeps several pending blocks from using up
// the guard of each other, and per event and actor keeps subagents apart from the main session. hash is empty
// when the content hash is unknown; those blocks are counted together.
func (s State) StopBlocks(session, event, actor string, line int, hash string) int {
	n := 0
	for _, e := range s.Entries {
		if e.Kind == KindStopBlock && e.Session == session && e.Event == event && e.Actor == actor &&
			e.Line == line && e.Hash == hash {
			n++
		}
	}
	return n
}
