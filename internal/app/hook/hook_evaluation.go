package hook

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Evaluation is the evaluation of the workflow for one event of a call, as far as it got: what it is about (the
// workflow, the repository, the event), the facts of the repository read so far (see Facts) and the outcomes of
// the steps that acted on this event, in order. It is asked again for the next step after every step, from the
// start of the event: the facts make the reads that already happened free and the journal replays what already
// acted, so a later entry sees what an earlier one did.
type Evaluation struct {
	Workflow workflow.Workflow
	Present  bool   // polix.yaml exists; without it only the protections apply
	Invalid  error  // why the workflow cannot be used (see Running); Workflow is then ignored
	Root     string // root of the working tree
	GitDir   string // git directory of the working tree, where the log lives
	System   string // the system polix runs on (GOOS), whose file names may ignore case
	Event    agent.Event
	Facts    Facts
	Journal  []Outcome // the outcomes of the steps that acted on this event, in order
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

// Fold reports whether the file names of the system of the evaluation usually ignore case: macOS and Windows.
func (e Evaluation) Fold() bool { return e.System == "darwin" || e.System == "windows" }

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

// KnowForeign returns the evaluation knowing whether the absolute folder dir is inside another git repository.
func (e Evaluation) KnowForeign(dir string, foreign bool) Evaluation {
	e.Facts = e.Facts.learnForeign(dir, foreign)
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

// Running returns the evaluation for the polix of version running: when the problems of its workflow may be options
// of a newer polix than running, Invalid carries a hint, so the agent updates polix instead of removing them. A
// workflow that requires a newer polix stays a workflow.TooOldError, and a usable one stays usable.
func (e Evaluation) Running(running string) Evaluation {
	var old workflow.TooOldError
	if e.Invalid == nil || errors.As(e.Invalid, &old) || !e.mayNeedNewer() {
		return e
	}
	//nolint:staticcheck // ST1005: the agent reads this message as is, a sentence in Spanish
	e.Invalid = fmt.Errorf("%w\nSi %s usa opciones de una versión de polix más reciente que la instalada (%s), no las "+
		"quites: ejecuta polix update.", e.Invalid, workflow.File, running)
	return e
}

// mayNeedNewer reports whether the problems of the workflow may come from a newer polix: it requires one, it names
// something this one does not know (not a name of the old vocabulary, which comes with its new spelling), or it has
// a later version of the schema.
func (e Evaluation) mayNeedNewer() bool {
	problems := e.Invalid.Error()
	if strings.Contains(problems, "requires polix") || newerVersion.MatchString(problems) {
		return true
	}
	for line := range strings.SplitSeq(problems, "\n") {
		if strings.Contains(line, "unknown") && !workflow.Problem(line).Renamed() {
			return true
		}
	}
	return false
}
