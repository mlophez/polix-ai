package hook

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/agent"
	"github.com/mlophez/polix-ai/internal/diff"
	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// Evaluation is the evaluation of the workflow for one event of a call, as far as it got: what it is about (the
// workflow, the repository, the event), the facts of the repository read so far and the outcomes of the steps
// that acted on this event, in order. It is asked again for the next step after every step, from the start of
// the event: the facts make the reads that already happened free and the journal replays what already acted, so
// a later entry sees what an earlier one did.
type Evaluation struct {
	Workflow workflow.Workflow
	Present  bool   // polix.yaml exists; without it only the protections apply
	Invalid  error  // why the workflow cannot be used (see Unusable); Workflow is then ignored
	Root     string // root of the working tree
	GitDir   string // git directory of the working tree, where the log lives
	System   string // the system polix runs on (GOOS), whose file names may ignore case
	Event    agent.Event
	Facts    Facts
	Journal  []Outcome // the outcomes of the steps that acted on this event, in order
}

// Facts are what an evaluation learned of the repository during the call, each read once when a step asked for
// it, and kept for the next events of the call. The content hash and the files read are kept per epoch, the number
// of runs carried out before them, since a run may rewrite files.
type Facts struct {
	Branch     string
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
	Contents   map[string]string // per epoch and path, see contentKey
	Unreadable map[string]bool   // per epoch and path, see contentKey
	Searches   map[string]bool   // per epoch, patterns and text: whether a matching file holds the text
	// TreeDiffs and TreeErrs are, per pair of trees (see treeKey), the paths that differ between them or why they
	// could not be compared; a tree identifies its content, so the answer holds at any epoch.
	TreeDiffs map[string][]string
	TreeErrs  map[string]error
	Warnings  []string // warnings of the call, such as an unknown content hash
}

// newerVersion matches the problem of a workflow of a later version of the schema.
var newerVersion = regexp.MustCompile(`version must be 1, got ([2-9]|\d{2,})`)

// Valid tells why the evaluation cannot run: it needs the repository and an event polix evaluates.
func (e Evaluation) Valid() error {
	var errs []error
	if e.Root == "" || e.GitDir == "" {
		errs = append(errs, errors.New("an evaluation needs the root and the git directory of the repository"))
	}
	errs = append(errs, e.Event.Valid(), e.Facts.Valid())
	return errors.Join(errs...)
}

// StateDir returns the folder of the state of polix, below the git directory.
func (e Evaluation) StateDir() string { return filepath.Join(e.GitDir, state.DirName) }

// Fold reports whether the file names of the system of the evaluation ignore case.
func (e Evaluation) Fold() bool { return FoldsCase(e.System) }

// Windows reports whether the evaluation runs on Windows.
func (e Evaluation) Windows() bool { return e.System == "windows" }

// KnowBranch returns the evaluation knowing the current branch.
func (e Evaluation) KnowBranch(branch string) Evaluation {
	e.Facts = e.Facts.learnBranch(branch)
	return e
}

// KnowHash returns the evaluation knowing the content hash at epoch, or why it could not be computed: then nothing
// tied to a content is observed, finished never holds, a run and a block with once act every time, and the call
// warns once.
func (e Evaluation) KnowHash(epoch int, hash string, err error) Evaluation {
	e.Facts = e.Facts.learnHash(epoch, hash, err)
	return e
}

// KnowChanged returns the evaluation knowing the files not committed yet.
func (e Evaluation) KnowChanged(files []string) Evaluation {
	e.Facts = e.Facts.learnChanged(files)
	return e
}

// KnowDiff returns the evaluation knowing the size of the change of each file not committed yet, or why it could
// not be measured: then the measure conditions hold, and the call warns once.
func (e Evaluation) KnowDiff(d diff.Diff, err error) Evaluation {
	e.Facts = e.Facts.learnDiff(d, err)
	return e
}

// KnowFiles returns the evaluation knowing the files of the working tree.
func (e Evaluation) KnowFiles(files []string) Evaluation {
	e.Facts = e.Facts.learnFiles(files)
	return e
}

// KnowRules returns the evaluation knowing the rules, or why they could not be loaded.
func (e Evaluation) KnowRules(rules []rule.Rule, err error) Evaluation {
	e.Facts = e.Facts.learnRules(rules, err)
	return e
}

// KnowState returns the evaluation knowing the log, or why it could not be read. The entries appended by the
// steps of the journal so far are in it.
func (e Evaluation) KnowState(s state.State, err error) Evaluation {
	e.Facts = e.Facts.learnState(s, err, len(e.Journal))
	return e
}

// KnowIgnored returns the evaluation knowing whether git ignores the path.
func (e Evaluation) KnowIgnored(path string, ignored bool) Evaluation {
	e.Facts = e.Facts.learnIgnored(path, ignored)
	return e
}

// KnowReal returns the evaluation knowing the real path of each of paths, in the same order.
func (e Evaluation) KnowReal(paths, reals []string) Evaluation {
	e.Facts = e.Facts.learnReal(paths, reals)
	return e
}

// KnowContent returns the evaluation knowing the content of the file at epoch, or that it cannot be read.
func (e Evaluation) KnowContent(epoch int, path, data string, err error) Evaluation {
	e.Facts = e.Facts.learnContent(epoch, path, data, err)
	return e
}

// KnowFound returns the evaluation knowing whether a file matching the patterns holds text at epoch.
func (e Evaluation) KnowFound(epoch int, patterns []string, text string, found bool) Evaluation {
	e.Facts = e.Facts.learnFound(epoch, patterns, text, found)
	return e
}

// KnowTreeDiff returns the evaluation knowing the paths that differ between the trees from and to, or why they
// could not be compared (a tree git no longer has, or git failing): then the trees count as different.
func (e Evaluation) KnowTreeDiff(from, to string, paths []string, err error) Evaluation {
	e.Facts = e.Facts.learnTreeDiff(from, to, paths, err)
	return e
}

// Note returns the evaluation with the outcome of the step that acted last: what it printed and why it failed.
func (e Evaluation) Note(output string, err error) Evaluation {
	o := Outcome{Output: output, Err: err}
	if o.Valid() != nil {
		return e
	}
	e.Journal = append(slices.Clone(e.Journal), o)
	return e
}

// Valid tells why the facts are not facts: negative counts.
func (f Facts) Valid() error {
	if f.Epoch < 0 || f.StateAt < 0 {
		return errors.New("the epoch and the steps of the facts cannot be negative")
	}
	return f.State.Valid()
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

// Hash returns the content hash at epoch, whether it is known and whether it was computed at all.
func (f Facts) Hash(epoch int) (hash string, known, computed bool) {
	h, ok := f.Hashes[epoch]
	return h, h != "", ok
}

// Content returns the content of the file at epoch, whether it can be read and whether it was read at all.
func (f Facts) Content(epoch int, path string) (data string, readable, read bool) {
	key := contentKey(epoch, path)
	if f.Unreadable[key] {
		return "", false, true
	}
	data, read = f.Contents[key]
	return data, read, read
}

// Found reports whether a file matching the patterns holds text at epoch, and whether it is known.
func (f Facts) Found(epoch int, patterns []string, text string) (found, known bool) {
	found, known = f.Searches[foundKey(epoch, patterns, text)]
	return found, known
}

// TreeDiff returns the paths that differ between the trees from and to, whether they were compared at all, and
// why they could not be.
func (f Facts) TreeDiff(from, to string) (paths []string, compared bool, err error) {
	key := treeKey(from, to)
	if err, failed := f.TreeErrs[key]; failed {
		return nil, true, err
	}
	paths, compared = f.TreeDiffs[key]
	return paths, compared, nil
}

func (f Facts) learnTreeDiff(from, to string, paths []string, err error) Facts {
	key := treeKey(from, to)
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
	m[foundKey(epoch, patterns, text)] = found
	f.Searches = m
	return f
}

func (f Facts) learnBranch(branch string) Facts {
	f.Branch, f.HasBranch = branch, true
	return f
}

func (f Facts) learnHash(epoch int, hash string, err error) Facts {
	if err != nil {
		hash = ""
		f.Warnings = appendNew(f.Warnings, fmt.Sprintf(
			"polix no puede calcular el hash del contenido (%v): no se registra lo observado ni se cumple finished, "+
				"y run y los block con once actúan cada vez", err))
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
		f.Warnings = appendNew(f.Warnings, fmt.Sprintf(
			"polix no puede medir el diff (%v): changed_lines, changed_files y risk_score se cumplen", err))
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

func (f Facts) learnContent(epoch int, path, data string, err error) Facts {
	key := contentKey(epoch, path)
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

// contentKey is the key of a file read at epoch.
func contentKey(epoch int, path string) string { return fmt.Sprintf("%d\x00%s", epoch, path) }

// treeKey is the key of the comparison of two trees.
func treeKey(from, to string) string { return from + "\x00" + to }

// foundKey is the key of a search at epoch.
func foundKey(epoch int, patterns []string, text string) string {
	return fmt.Sprintf("%d\x00%s\x00%s", epoch, strings.Join(patterns, "\x01"), text)
}

// Unusable returns why a workflow cannot be used from the error of loading it (nil when it can), with a hint when
// its problems may be options of a newer polix than running: the agent then updates instead of removing them.
// A workflow that requires a newer polix stays a workflow.TooOld.
func Unusable(err error, running string) error {
	if err == nil {
		return nil
	}
	var old workflow.TooOld
	if !errors.As(err, &old) && mayNeedNewer(err.Error()) {
		//nolint:staticcheck // ST1005: the agent reads this message as is, a sentence in Spanish
		return fmt.Errorf("%w\nSi %s usa opciones de una versión de polix más reciente que la instalada (%s), no las "+
			"quites: ejecuta polix update.", err, workflow.File, running)
	}
	return err
}

// mayNeedNewer reports whether the problems of a workflow may come from a newer polix: it requires one, it names
// something this one does not know (not a name of the old vocabulary, which comes with its new spelling), or it has
// a later version of the schema.
func mayNeedNewer(problems string) bool {
	if strings.Contains(problems, "requires polix") || newerVersion.MatchString(problems) {
		return true
	}
	for line := range strings.SplitSeq(problems, "\n") {
		if strings.Contains(line, "unknown") && !workflow.Renamed(line) {
			return true
		}
	}
	return false
}

// appendNew appends to a copy of list the texts of more it does not hold yet.
func appendNew(list []string, more ...string) []string {
	list = slices.Clone(list)
	for _, s := range more {
		if !slices.Contains(list, s) {
			list = append(list, s)
		}
	}
	return list
}
