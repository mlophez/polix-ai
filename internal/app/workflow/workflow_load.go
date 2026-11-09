package workflow

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// Load loads polix.yaml of the repository at root for the polix of version running, which decides whether the
// file requires a newer one (TooOldError). It is the loader of the use case LoadWorkflow (internal/app/workflow/load),
// which cmd/polix builds once and passes to every use case that reads polix.yaml. It never fails: what stops the
// reading is data of its result.
type Load func(ctx context.Context, root, running string) Loaded

// Loaded is what Load read from polix.yaml. Each use case decides what an absent file, a workflow with problems or
// a file that cannot be read means to it.
type Loaded struct {
	// Workflow is the workflow of the file; the default one without the file, and empty when Err stops the reading.
	Workflow Workflow
	Exists   bool // whether polix.yaml exists
	// Err tells why the workflow cannot be used as it is: the file cannot be read, requires a newer polix (TooOldError),
	// is not yaml or has an unknown top level key (UnknownKeyError), which stop the reading; or the problems of a
	// workflow that decodes (Invalid), which comes with them. nil when the workflow is valid.
	Err error
}

// TooOldError is the error of a workflow that requires a newer polix than the one running.
type TooOldError struct {
	Requires string // the version polix.yaml requires
	Running  string // the version of the running polix
}

// UnknownKeyError is the error of a top level key of polix.yaml that is not one of TopKeys; a name of the old vocabulary
// comes with its new spelling.
type UnknownKeyError struct {
	Key  string // the key, as written
	Line int    // its line in the file
}

// TopKeys are the keys of polix.yaml.
var TopKeys = []string{"version", "requires", "base_branch", "max_stop_retries", "guardrails", "vars", "on", "rules",
	"plans", "reports", "guidance", "risk"}

// RiskKeys are the keys of the section risk.
var RiskKeys = []string{"weights"}

// Valid reports why l is not what polix.yaml loads: an error without the file, a workflow with an error that stops
// the reading, or a workflow with problems without them.
func (l Loaded) Valid() error {
	_, invalid := errors.AsType[InvalidError](l.Err)
	switch {
	case !l.Exists && l.Err != nil:
		return fmt.Errorf("an error loaded without %s: %w", File, l.Err)
	case l.Err != nil && !invalid && !reflect.DeepEqual(l.Workflow, Workflow{}):
		return fmt.Errorf("a workflow loaded from a %s that cannot be used: %w", File, l.Err)
	case l.Err == nil:
		if err := l.Workflow.Valid(); err != nil {
			return fmt.Errorf("a workflow loaded without its problems: %w", err)
		}
	}
	return nil
}

// Fail returns l failed with err: no workflow, and err stops every use of it.
func (l Loaded) Fail(err error) Loaded {
	failed := l
	failed.Workflow, failed.Err = Workflow{}, err
	return failed
}

// Valid tells why the error is not valid: it names both versions.
func (e TooOldError) Valid() error {
	if e.Requires == "" || e.Running == "" {
		return errors.New("too old needs the version required and the running one")
	}
	return nil
}

func (e TooOldError) Error() string {
	return fmt.Sprintf("%s requires polix %s or newer and this is %s: run polix update", File, e.Requires, e.Running)
}

// Valid tells why the error is not valid: it names the line of its key, which may be empty ("": x).
func (e UnknownKeyError) Valid() error {
	if e.Line < 1 {
		return errors.New("an unknown key needs its line")
	}
	return nil
}

func (e UnknownKeyError) Error() string {
	if hint, ok := renamed["top"][e.Key]; ok {
		return fmt.Sprintf("%s:%d: unknown key %q: %s", File, e.Line, e.Key, hint)
	}
	return fmt.Sprintf("%s:%d: unknown key %q", File, e.Line, e.Key)
}
