package hook

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/state"
)

// Facts are what an evaluation learned of the repository during the call, each read once when a step asked for
// it, and kept for the next events of the call. The content hash and the files read are kept per epoch, the number
// of runs carried out before them, since a run may rewrite files.
type Facts struct {
	Branch     string
	Head       string // the commit HEAD points to, learned with the branch; "" when unknown, as without commits
	HasBranch  bool
	Hashes     map[int]string // content hash per epoch; "" when it could not be computed
	Epoch      int            // runs carried out by the earlier events of the call
	Changed    []string
	HasChanged bool
	Diff       diff.Diff // the size of the change of each file not committed yet
	DiffErr    error     // the diff could not be measured: the measure conditions hold
	HasDiff    bool
	Files      []string
	HasFiles   bool
	Rules      []rule.Rule
	RulesErr   error
	HasRules   bool
	State      state.State
	StateErr   error // the log could not be read: the loop guard fails open
	HasState   bool
	StateAt    int // the steps of the journal that acted before the log was read: their entries are in it
	Ignored    map[string]bool
	Reals      map[string]string
	Foreign    map[string]bool   // per absolute folder, whether it is inside another git repository
	Missing    []string          // the programs the PATH does not run, of those looked up
	HasMissing bool              // whether the programs were looked up
	Contents   map[string]string // per epoch and path, see contentKey
	Unreadable map[string]bool   // per epoch and path, see contentKey
	Searches   map[string]bool   // per epoch, patterns and text: whether a matching file holds the text
	// TreeDiffs and TreeErrs are, per pair of trees (see treeKey), the paths that differ between them or why they
	// could not be compared; a tree identifies its content, so the answer holds at any epoch.
	TreeDiffs map[string][]string
	TreeErrs  map[string]error
	Warnings  []string // warnings of the call, such as an unknown content hash
	// Blocked tells that an earlier event of the call blocked: the answer of the call is a block, so none of its
	// messages for the user is shown (see Answered).
	Blocked bool
	// Pending are the observed lines of the earlier events of the call that are written only when the whole call
	// does not block, by its last event (see Evaluation.Last): the notices of notify, which a blocked call never
	// shows to the user. A block of the call drops them (see Answered).
	Pending []state.Entry
}

// Valid tells why the facts are not facts: negative counts, or a pending line that could not be appended.
func (f Facts) Valid() error {
	if f.Epoch < 0 || f.StateAt < 0 {
		return errors.New("the epoch and the steps of the facts cannot be negative")
	}
	errs := []error{f.State.Valid()}
	for _, e := range f.Pending {
		errs = append(errs, e.Valid())
	}
	return errors.Join(errs...)
}

// Carry returns the facts the next event of the call starts from: after epoch runs, with the log s as this event
// left it when it was read.
func (f Facts) Carry(epoch int, s state.State) Facts {
	f.Epoch = epoch
	if f.HasState && f.StateErr == nil {
		f.State = s
	}
	f.StateAt = 0
	return f
}

// Answered returns the facts after an event of the call answered a with the lines pending to write, for the next
// event: the answers of the events are merged into one, so a block marks the call as blocked, and the messages for
// the user of an answer that does not block become warnings of the call. A later event then knows the messages of
// the call its block hides, and whether the block of an earlier one hides its own. An answer that does not block
// keeps a copy of pending, in place of the lines the facts held (see Pending); a block drops them, since the user
// never sees their notices.
func (f Facts) Answered(a agent.Answer, pending []state.Entry) Facts {
	if a.Block {
		f.Blocked = true
		f.Pending = nil
		return f
	}
	f.Pending = slices.Clone(pending)
	for _, w := range a.Warnings {
		f = f.warned(w)
	}
	return f
}

// Hash returns the content hash at epoch, whether it is known and whether it was computed at all.
func (f Facts) Hash(epoch int) (hash string, known, computed bool) {
	h, ok := f.Hashes[epoch]
	return h, h != "", ok
}

// Content returns the content of the file at epoch, whether it can be read and whether it was read at all.
func (f Facts) Content(epoch int, path string) (data string, readable, read bool) {
	key := f.contentKey(epoch, path)
	if f.Unreadable[key] {
		return "", false, true
	}
	data, read = f.Contents[key]
	return data, read, read
}

// Found reports whether a file matching the patterns holds text at epoch, and whether it is known.
func (f Facts) Found(epoch int, patterns []string, text string) (found, known bool) {
	found, known = f.Searches[f.foundKey(epoch, patterns, text)]
	return found, known
}

// TreeDiff returns the paths that differ between the trees from and to, whether they were compared at all, and
// why they could not be.
func (f Facts) TreeDiff(from, to string) (paths []string, compared bool, err error) {
	key := f.treeKey(from, to)
	if err, failed := f.TreeErrs[key]; failed {
		return nil, true, err
	}
	paths, compared = f.TreeDiffs[key]
	return paths, compared, nil
}

func (f Facts) learnTreeDiff(from, to string, paths []string, err error) Facts {
	key := f.treeKey(from, to)
	if err != nil {
		m := make(map[string]error, len(f.TreeErrs)+1)
		maps.Copy(m, f.TreeErrs)
		m[key] = err
		f.TreeErrs = m
		return f
	}
	m := make(map[string][]string, len(f.TreeDiffs)+1)
	maps.Copy(m, f.TreeDiffs)
	m[key] = slices.Clone(paths)
	f.TreeDiffs = m
	return f
}

func (f Facts) learnFound(epoch int, patterns []string, text string, found bool) Facts {
	m := make(map[string]bool, len(f.Searches)+1)
	maps.Copy(m, f.Searches)
	m[f.foundKey(epoch, patterns, text)] = found
	f.Searches = m
	return f
}

func (f Facts) learnBranch(branch, head string) Facts {
	f.Branch, f.Head, f.HasBranch = branch, head, true
	return f
}

func (f Facts) learnHash(epoch int, hash string, err error) Facts {
	if err != nil {
		hash = ""
		f = f.warned(fmt.Sprintf(
			"polix cannot hash the content (%v): nothing observed is recorded, finished does not hold, "+
				"and run and the blocks with once act every time", err))
	}
	hashes := make(map[int]string, len(f.Hashes)+1)
	maps.Copy(hashes, f.Hashes)
	hashes[epoch] = hash
	f.Hashes = hashes
	return f
}

func (f Facts) learnChanged(files []string) Facts {
	f.Changed, f.HasChanged = slices.Clone(files), true
	return f
}

func (f Facts) learnDiff(d diff.Diff, err error) Facts {
	if err != nil {
		d = diff.Diff{}
		f = f.warned(fmt.Sprintf(
			"polix cannot measure the diff (%v): changed_lines, changed_files and risk_score hold", err))
	}
	f.Diff, f.DiffErr, f.HasDiff = d.Narrow(nil), err, true
	return f
}

func (f Facts) learnFiles(files []string) Facts {
	f.Files, f.HasFiles = slices.Clone(files), true
	return f
}

func (f Facts) learnRules(rules []rule.Rule, err error) Facts {
	f.Rules, f.RulesErr, f.HasRules = slices.Clone(rules), err, true
	return f
}

func (f Facts) learnState(s state.State, err error, at int) Facts {
	if err != nil {
		s = state.State{}
	}
	f.State, f.StateErr, f.HasState, f.StateAt = s, err, true, at
	return f
}

func (f Facts) learnIgnored(path string, ignored bool) Facts {
	m := make(map[string]bool, len(f.Ignored)+1)
	maps.Copy(m, f.Ignored)
	m[path] = ignored
	f.Ignored = m
	return f
}

func (f Facts) learnReal(paths, reals []string) Facts {
	m := make(map[string]string, len(f.Reals)+len(paths))
	maps.Copy(m, f.Reals)
	for i, p := range paths {
		if i < len(reals) {
			m[p] = reals[i]
		}
	}
	f.Reals = m
	return f
}

func (f Facts) learnForeign(dir string, foreign bool) Facts {
	m := make(map[string]bool, len(f.Foreign)+1)
	maps.Copy(m, f.Foreign)
	m[dir] = foreign
	f.Foreign = m
	return f
}

func (f Facts) learnMissing(programs []string) Facts {
	f.Missing, f.HasMissing = slices.Clone(programs), true
	return f
}

func (f Facts) learnContent(epoch int, path, data string, err error) Facts {
	key := f.contentKey(epoch, path)
	if err != nil {
		m := make(map[string]bool, len(f.Unreadable)+1)
		maps.Copy(m, f.Unreadable)
		m[key] = true
		f.Unreadable = m
		return f
	}
	m := make(map[string]string, len(f.Contents)+1)
	maps.Copy(m, f.Contents)
	m[key] = data
	f.Contents = m
	return f
}

// warned returns the facts with the warning text on a copy of their warnings, unless they already hold it.
func (f Facts) warned(text string) Facts {
	f.Warnings = slices.Clone(f.Warnings)
	if !slices.Contains(f.Warnings, text) {
		f.Warnings = append(f.Warnings, text)
	}
	return f
}

// contentKey is the key of a file read at epoch.
func (Facts) contentKey(epoch int, path string) string { return fmt.Sprintf("%d\x00%s", epoch, path) }

// treeKey is the key of the comparison of two trees.
func (Facts) treeKey(from, to string) string { return from + "\x00" + to }

// foundKey is the key of a search at epoch.
func (Facts) foundKey(epoch int, patterns []string, text string) string {
	return fmt.Sprintf("%d\x00%s\x00%s", epoch, strings.Join(patterns, "\x01"), text)
}
