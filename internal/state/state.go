// Package state is the append-only log of polix in <git-dir>/polix/log.jsonl: the facts it
// records (a reviewer finished, the tests passed) tied to the content hash they were observed on, and every
// decision it takes, which also serves as evidence of the process each change followed.
//
// Only polix writes it, from events it has seen; the agent never declares that something is done.
package state

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// DirName is the folder of the state inside the git directory, so it is never versioned and goes with the
// branch or worktree.
const DirName = "polix"

// Entry kinds.
const (
	KindFact      = "fact"       // something polix observed, e.g. reviewed
	KindDecision  = "decision"   // what polix answered to an event
	KindInjected  = "injected"   // a rule already injected as context in a session
	KindStopBlock = "stop_block" // a stop polix blocked, to bound loops
)

// Entry is one line of the log.
type Entry struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Agent   string `json:"agent,omitempty"`
	Session string `json:"session,omitempty"`
	Event   string `json:"event,omitempty"`
	Actor   string `json:"actor,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Hash    string `json:"hash,omitempty"`   // content hash; on decisions only when the call already computed it
	Fact    string `json:"fact,omitempty"`   // fact entries
	Rule    string `json:"rule,omitempty"`   // injected entries: the rule id
	Action  string `json:"action,omitempty"` // decision entries: the action of the workflow entry, or builtin
	Line    int    `json:"line,omitempty"`   // decision and stop_block entries: line of the entry in the workflow
	Outcome string `json:"outcome,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// Store reads and appends the log of one repository.
type Store struct {
	Dir     string
	entries []Entry
	loaded  bool
}

// Open returns the store under gitDir; the folder is created on the first append.
func Open(gitDir string) *Store { return &Store{Dir: filepath.Join(gitDir, DirName)} }

func (s *Store) path() string { return filepath.Join(s.Dir, "log.jsonl") }

// Entries returns every entry of the log, read once per process.
func (s *Store) Entries() ([]Entry, error) {
	if s.loaded {
		return s.entries, nil
	}
	f, err := os.Open(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		s.loaded = true
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			s.entries = append(s.entries, e)
		}
	}
	s.loaded = true
	return s.entries, sc.Err()
}

// Append writes one entry as a single line. Each line is written with one call in append mode, which keeps
// concurrent hooks from interleaving lines in practice; a real lock is pending for the PoC.
func (s *Store) Append(e Entry) error {
	if e.At == "" {
		e.At = time.Now().Format(time.RFC3339)
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.path(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	if s.loaded {
		s.entries = append(s.entries, e)
	}
	return nil
}

// Done reports whether fact was recorded on the branch for exactly this content hash.
func (s *Store) Done(fact, branch, hash string) bool {
	entries, _ := s.Entries()
	for _, e := range entries {
		if e.Kind == KindFact && e.Fact == fact && e.Branch == branch && e.Hash == hash {
			return true
		}
	}
	return false
}

// Injected reports whether rule was already injected as context in session.
func (s *Store) Injected(rule, session string) bool {
	entries, _ := s.Entries()
	for _, e := range entries {
		if e.Kind == KindInjected && e.Rule == rule && e.Session == session {
			return true
		}
	}
	return false
}

// StopBlocks counts the times the workflow entry at line blocked event (stop or subagent_stop) of actor in
// session on this content hash: the loop guard. Counting per entry keeps several pending blocks from using up
// the guard of each other, and per event and actor keeps subagents apart from the main session.
func (s *Store) StopBlocks(session, event, actor string, line int, hash string) int {
	entries, _ := s.Entries()
	n := 0
	for _, e := range entries {
		if e.Kind == KindStopBlock && e.Session == session && e.Event == event && e.Actor == actor &&
			e.Line == line && e.Hash == hash {
			n++
		}
	}
	return n
}
