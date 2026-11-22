// Package state is the domain of the append-only log of polix, one file per session of the agent in
// <git-dir>/polix/sessions/<name>.jsonl (see Session.File), subagents included. Each line is the record of one event
// of the agent (Record): its data, what polix did with it (its actions, with their result and reason) and what polix
// observed by itself on it (a session or a subagent started, a subagent finished, a check of polix passed, an entry
// blocked, an inject reached the AI, a notice reached the user), tied to the content hash it was observed on when a
// query needs it. The actions serve as evidence of the process each change followed; the observations are the memory
// of the session: the repeat of the verbs, the loop guard, the condition finished and the subagents still running
// (Running) are queries over the observations of the records of one session.
//
// Only polix writes it, from events it has seen; the agent never declares that something is done, and a command the
// agent runs is never observed. A record without a session is never written (see Record.Valid). Reading and writing
// the files, and the format of their lines, are the job of the adapters, through internal/platform/jsonl:
// stateadapters.Log (internal/app/state/adapters) reads and decodes one for every use case that reads it, and
// AnswerHook, its only writer, encodes its lines. The flat lines an earlier polix wrote (legacy records) are kept as
// read, to be shown, and ignored by every query.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// State is the log as read: its records in file order and how many lines were skipped for being over MaxLine.
type State struct {
	Records []Record
	Skipped int
}

// Query selects observations: those of its Outcome, and of each other field that is set, the fields of the record
// being those of the record the observation is in. With ByHash, only the observations of exactly Hash; an empty Hash
// then never matches, since an unknown content is no content. With ByBranch, only the observations of records of
// Branch, an empty one included (on a detached HEAD the branch is ""), or, when both the query and the record carry a
// Head, those of the same Head: another branch on the same commit, a detached HEAD included, selects them, while a
// record without Head or a query without one (no commit yet, git failing) matches only by branch. Without ByBranch,
// Head is ignored. With ByHead, only the observations of records whose Head is exactly the non empty Head of the
// query, on any branch (Branch is ignored); with an empty Head it selects none. Legacy records are never selected.
type Query struct {
	Outcome  string
	Session  string
	Actor    string
	Event    string
	Branch   string
	Head     string // the commit HEAD points to, "" when unknown; read only with ByBranch or ByHead
	Key      string
	Line     int // the line of the workflow entry; 0 for any
	Hash     string
	ByHash   bool
	ByBranch bool
	ByHead   bool
}

// Session is the id of a session of the agent, which names the file of its log (see File).
type Session string

// DirName is the folder of the state inside the git directory, so it is never versioned and goes with the
// branch or worktree.
const DirName = "polix"

// SessionsDir is the folder of the files of the sessions inside DirName.
const SessionsDir = "sessions"

// FileExt is the extension of the file of a session.
const FileExt = ".jsonl"

// maxSessionName is the longest id of a session that names its file as it is.
const maxSessionName = 128

// File returns the base name of the file of the session, inside SessionsDir, or an error for an empty id. The id
// names it as it is when it is made only of A-Z, a-z, 0-9, ".", "_" and "-", is at most 128 characters long and is not
// only dots; otherwise the name is the lowercase hex sha256 of the id. So the id the agent sends never reaches a
// path with a separator, as "." or "..", nor with a length or characters the file systems refuse.
func (s Session) File() (string, error) {
	if s == "" {
		return "", errors.New("a session needs an id to name its file")
	}
	if len(s) <= maxSessionName && strings.Trim(string(s), ".") != "" &&
		strings.Trim(string(s), "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-") == "" {
		return string(s) + FileExt, nil
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:]) + FileExt, nil
}

// Valid tells why the state is not valid: a negative count of skipped lines. Its records are what the log holds,
// kept as read even when a record would not be valid to append.
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

// Append returns a copy of the state with r at the end, as the log reads after appending it, or the error of a record
// that is not to be appended (see Record.Writable).
func (s State) Append(r Record) (State, error) {
	if err := r.Writable(); err != nil {
		return s, err
	}
	records := slices.Clone(s.Records)
	records = append(records, r)
	out := State{Records: records, Skipped: s.Skipped}
	if err := out.Valid(); err != nil {
		return s, err
	}
	return out, nil
}

// observations calls yield with every observation of the records that are not legacy, in file order, with its
// record and its position (Match.Seq), until yield returns false. The records are the ones of the state.
func (s State) observations(yield func(seq int, r *Record, o *Observation) bool) {
	seq := 0
	for i := range s.Records {
		r := &s.Records[i]
		if r.Legacy != "" {
			continue
		}
		for j := range r.Observations {
			if !yield(seq, r, &r.Observations[j]) {
				return
			}
			seq++
		}
	}
}

// matched returns the match of the observation o of the record r at position seq, or why it is not one.
func (s State) matched(seq int, r *Record, o *Observation) (Match, error) {
	m := Match{Observation: *o, Record: *r, Seq: seq}
	return m, m.Valid()
}

// Observed returns the observations the query selects, in file order; none for a query that is not valid.
func (s State) Observed(q Query) []Match {
	if q.Valid() != nil || q.ByHash && q.Hash == "" {
		return nil
	}
	var out []Match
	s.observations(func(seq int, r *Record, o *Observation) bool {
		if o.Outcome != q.Outcome || !q.selects(r, o) {
			return true
		}
		if m, err := s.matched(seq, r, o); err == nil {
			out = append(out, m)
		}
		return true
	})
	return out
}

// Chain returns, in file order, the observations the query selects on the content of q.Hash or of any of also, or on
// a content that reached one of those only through the runs of the events of the selected observations: walking the
// log back, the Before of a selected observation, the content its event started from, joins the contents selected.
// With no observation carrying a Before it selects what Observed selects with ByHash, on q.Hash and also. A command
// that writes files the tree does not ignore leaves a new content on every run, so the blocks on a content the agent
// left alone are counted together. None for a query that is not valid or without q.Hash.
func (s State) Chain(q Query, also ...string) []Match {
	if q.Valid() != nil || q.Hash == "" {
		return nil
	}
	hashes := []string{q.Hash}
	for _, h := range also {
		if h != "" && !slices.Contains(hashes, h) {
			hashes = append(hashes, h)
		}
	}
	var all []Match
	s.observations(func(seq int, r *Record, o *Observation) bool {
		if o.Outcome != q.Outcome || !q.selectsAnyHash(r, o) {
			return true
		}
		if m, err := s.matched(seq, r, o); err == nil {
			all = append(all, m)
		}
		return true
	})
	var out []Match
	for _, m := range slices.Backward(all) {
		if !slices.Contains(hashes, m.Hash) {
			continue
		}
		out = append(out, m)
		if m.Before != "" && !slices.Contains(hashes, m.Before) {
			hashes = append(hashes, m.Before)
		}
	}
	slices.Reverse(out)
	return out
}

// Latest returns the last observation the query selects, and whether there is one.
func (s State) Latest(q Query) (Match, bool) {
	found := s.Observed(q)
	if len(found) == 0 {
		return Match{}, false
	}
	return found[len(found)-1], true
}

// Index returns the position (Match.Seq) of the last observation the query selects, or -1 when there is none or the
// query is not valid, so the observations of several queries can be ordered: an inject is due again when a session
// started after it reached the AI.
func (s State) Index(q Query) int {
	if q.Valid() != nil || q.ByHash && q.Hash == "" {
		return -1
	}
	last := -1
	s.observations(func(seq int, r *Record, o *Observation) bool {
		if o.Outcome == q.Outcome && q.selects(r, o) {
			last = seq
		}
		return true
	})
	return last
}

// selects reports whether the observation o of the record r matches every field the query sets, its outcome apart.
func (q Query) selects(r *Record, o *Observation) bool {
	return q.selectsAnyHash(r, o) && (!q.ByHash || o.Hash == q.Hash)
}

// selectsAnyHash reports whether the observation o of the record r matches every field the query sets, its outcome
// and its hash apart.
func (q Query) selectsAnyHash(r *Record, o *Observation) bool {
	return (q.Session == "" || r.Session == q.Session) && (q.Actor == "" || r.Actor == q.Actor) &&
		(q.Event == "" || r.Event == q.Event) && q.selectsBranch(r) &&
		(q.Key == "" || o.Key == q.Key) && (q.Line == 0 || o.Line == q.Line)
}

// selectsBranch reports whether r is of the branch of the query: of any branch without Branch nor ByBranch; with
// ByBranch, of exactly Branch or of the same non empty Head; with ByHead, of the same non empty Head alone.
func (q Query) selectsBranch(r *Record) bool {
	if q.ByHead {
		return q.Head != "" && r.Head == q.Head
	}
	if !q.ByBranch {
		return q.Branch == "" || r.Branch == q.Branch
	}
	return r.Branch == q.Branch || q.Head != "" && r.Head == q.Head
}
