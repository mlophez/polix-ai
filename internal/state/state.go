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
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"
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

// MaxLine is the longest line of the log that is read; longer lines are skipped.
const MaxLine = 4 * 1024 * 1024

// MaxReason is the longest reason stored in an entry; longer reasons are cut, so every line stays far below
// MaxLine.
const MaxReason = 8 * 1024

// Store reads and appends the log of one repository.
type Store struct {
	Dir     string
	entries []Entry
	loaded  bool
	skipped int
}

// Open returns the store under gitDir; the folder is created on the first append.
func Open(gitDir string) *Store { return &Store{Dir: filepath.Join(gitDir, DirName)} }

func (s *Store) path() string { return filepath.Join(s.Dir, "log.jsonl") }

// Entries returns every entry of the log, read once per process. Lines over MaxLine are skipped and counted
// (see Skipped); it fails only when the log exists and cannot be read.
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
	var entries []Entry
	skipped := 0
	err = readLines(f, MaxLine, func(line []byte, tooLong bool) {
		if tooLong {
			skipped++
			return
		}
		var e Entry
		if json.Unmarshal(line, &e) == nil {
			entries = append(entries, e)
		}
	})
	if err != nil {
		return nil, err
	}
	s.entries, s.skipped, s.loaded = entries, skipped, true
	return s.entries, nil
}

// Skipped returns how many lines Entries skipped for being over MaxLine.
func (s *Store) Skipped() int { return s.skipped }

// readLines calls fn with every line of r without its newline; a line over limit is discarded as it is read
// and reported with tooLong, so a huge line never stops the reading nor fills the memory.
func readLines(r io.Reader, limit int, fn func(line []byte, tooLong bool)) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var buf []byte
	tooLong := false
	for {
		chunk, err := br.ReadSlice('\n')
		if !tooLong {
			if len(buf)+len(chunk) > limit+1 {
				tooLong, buf = true, buf[:0]
			} else {
				buf = append(buf, chunk...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if tooLong || len(buf) > 0 {
			line := buf
			if n := len(line); n > 0 && line[n-1] == '\n' {
				line = line[:n-1]
			}
			fn(line, tooLong)
		}
		buf, tooLong = buf[:0], false
		if err != nil {
			return nil
		}
	}
}

// Cut shortens s to at most limit bytes, on a rune boundary, marking the cut.
func Cut(s string, limit int) string {
	const mark = "… [recortado]"
	if len(s) <= limit {
		return s
	}
	n := limit - len(mark)
	if n < 0 {
		n = 0
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + mark
}

// Append writes one entry as a single line. Each line is written with one call in append mode, which keeps
// concurrent hooks from interleaving lines in practice; a real lock is pending for the PoC.
func (s *Store) Append(e Entry) error {
	if e.At == "" {
		e.At = time.Now().Format(time.RFC3339)
	}
	e.Reason = Cut(e.Reason, MaxReason)
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

// Done reports whether fact was recorded on the branch for exactly this content hash; never for an unknown
// (empty) hash.
func (s *Store) Done(fact, branch, hash string) bool {
	if hash == "" {
		return false
	}
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
// the guard of each other, and per event and actor keeps subagents apart from the main session. hash is empty
// when the content hash is unknown; those blocks are counted together. It returns the error of reading the log,
// so the guard can let the agent stop instead of blocking it forever.
func (s *Store) StopBlocks(session, event, actor string, line int, hash string) (int, error) {
	entries, err := s.Entries()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.Kind == KindStopBlock && e.Session == session && e.Event == event && e.Actor == actor &&
			e.Line == line && e.Hash == hash {
			n++
		}
	}
	return n, nil
}
