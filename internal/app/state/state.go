// Package state is the domain of the append-only log of polix in <git-dir>/polix/log.jsonl: what polix observed by
// itself (a session or a subagent started, a subagent finished, a check of polix passed, an entry blocked, an inject
// reached the AI, a notice reached the user), tied to the content hash it was observed on when a query needs it, and
// every decision it takes, which also
// serves as evidence of the process each change followed. The repeat of the verbs, the loop guard and the condition
// finished are queries over the observed entries.
//
// Only polix writes it, from events it has seen; the agent never declares that something is done, and a command
// the agent runs is never observed. Reading and writing the file, and the format of its lines, are the job of the
// adapters, through internal/platform/jsonl: the use case LoadState (internal/app/state/load) reads and decodes it
// for every use case that reads it, and AnswerHook, its only writer, encodes its lines. Lines of kinds this polix no
// longer writes (fact, injected and stop_block) are kept as read and ignored by every query.
package state

import (
	"errors"
	"fmt"
	"slices"
)

// State is the log as read: its entries in file order and how many lines were skipped for being over MaxLine.
type State struct {
	Entries []Entry
	Skipped int
}

// Query selects observed entries: those of its Outcome, and of each other field that is set. With ByHash, only the
// entries of exactly Hash; an empty Hash then never matches, since an unknown content is no content. With ByBranch,
// only the entries of Branch, an empty one included (on a detached HEAD the branch is ""), or, when both the query
// and the entry carry a Head, those of the same Head: another branch on the same commit, a detached HEAD included,
// selects them, while an entry without Head (written by an older polix) or a query without one (no commit yet, git
// failing) matches only by branch. Without ByBranch, Head is ignored.
type Query struct {
	Outcome  string
	Session  string
	Actor    string
	Event    string
	Branch   string
	Head     string // the commit HEAD points to, "" when unknown; read only with ByBranch
	Key      string
	Line     int // the line of the workflow entry; 0 for any
	Hash     string
	ByHash   bool
	ByBranch bool
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

// Valid tells why the query selects nothing that can be observed: an unknown outcome or a negative line.
func (q Query) Valid() error {
	var errs []error
	if !slices.Contains(Outcomes, q.Outcome) {
		errs = append(errs, fmt.Errorf("a query needs an outcome of %v, got %q", Outcomes, q.Outcome))
	}
	if q.Line < 0 {
		errs = append(errs, errors.New("the line of a query must not be negative"))
	}
	return errors.Join(errs...)
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

// Observed returns the observed entries the query selects, in file order; none for a query that is not valid.
func (s State) Observed(q Query) []Entry {
	if q.Valid() != nil || q.ByHash && q.Hash == "" {
		return nil
	}
	var out []Entry
	for _, e := range s.Entries {
		if e.Kind == KindObserved && e.Outcome == q.Outcome && q.selects(e) {
			out = append(out, e)
		}
	}
	return out
}

// Latest returns the last observed entry the query selects, and whether there is one.
func (s State) Latest(q Query) (Entry, bool) {
	found := s.Observed(q)
	if len(found) == 0 {
		return Entry{}, false
	}
	return found[len(found)-1], true
}

// Index returns the position in Entries of the last observed entry the query selects, or -1 when there is none or
// the query is not valid, so the observations of several queries can be ordered: an inject is due again when a
// session started after it reached the AI.
func (s State) Index(q Query) int {
	if q.Valid() != nil || q.ByHash && q.Hash == "" {
		return -1
	}
	for i, e := range slices.Backward(s.Entries) {
		if e.Kind == KindObserved && e.Outcome == q.Outcome && q.selects(e) {
			return i
		}
	}
	return -1
}

// selects reports whether e matches every field the query sets, its outcome apart.
func (q Query) selects(e Entry) bool {
	return (q.Session == "" || e.Session == q.Session) && (q.Actor == "" || e.Actor == q.Actor) &&
		(q.Event == "" || e.Event == q.Event) && q.selectsBranch(e) &&
		(q.Key == "" || e.Key == q.Key) && (q.Line == 0 || e.Line == q.Line) && (!q.ByHash || e.Hash == q.Hash)
}

// selectsBranch reports whether e is of the branch of the query: of any branch without Branch nor ByBranch; with
// ByBranch, of exactly Branch or of the same non empty Head.
func (q Query) selectsBranch(e Entry) bool {
	if !q.ByBranch {
		return q.Branch == "" || e.Branch == q.Branch
	}
	return e.Branch == q.Branch || q.Head != "" && e.Head == q.Head
}
