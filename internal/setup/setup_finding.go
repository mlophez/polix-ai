package setup

import (
	"errors"
	"fmt"
	"slices"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// Finding is one line of the report of polix doctor: its level, the area it is about and what it says. It is
// built with a literal followed by Valid.
type Finding struct {
	Level   string // LevelOK, LevelWarn or LevelFail
	Area    string // binary, version, path, git, repo, workflow, rules, hooks, plugin, codex or state
	Message string
}

// Inspection is what polix doctor observed on the machine and in the repository, read through the port: each
// fact with the error of reading it. Outside a repository the facts of the repository are empty. It is built with
// a literal followed by Valid.
type Inspection struct {
	Setup       Setup  // the home of the user, the root of the repository (empty outside one) and the version
	Exe         string // the running binary
	ExeErr      error  // why the running binary cannot be found
	Latest      string // the latest release, asked or known from the last check; empty when unknown
	LatestErr   error  // why the latest release could not be asked
	Path        string // the polix the PATH runs, its symlinks resolved
	PathErr     error  // why polix is not on the PATH
	SamePath    bool   // whether the polix of the PATH is the running binary
	GitErr      error  // why git is not on the PATH
	RepoErr     error  // why the folder is not inside a repository
	Workflow    workflow.Workflow
	Found       bool  // whether polix.yaml exists
	WorkflowErr error // why polix.yaml cannot be read, parsed or is not valid (workflow.Invalid)
	Rules       []rule.Rule
	Invalid     []string          // the problems of the invalid rules (rule.ParseAll)
	RulesErr    error             // why the rules cannot be read
	Missing     map[string]bool   // by check: whether its file does not exist
	Runnable    map[string]error  // by check: why it cannot run as a program (not executable, no interpreter)
	BaseErr     error             // why no base branch of the workflow exists
	Files       map[string][]byte // the files read (settings, hooks, configuration, manifest) that exist
	FileErrs    map[string]error  // the files that exist but cannot be read
	Embedded    map[string][]byte // the plugin embedded in the binary, below polix/
	Claude      []byte            // what `claude plugin list --json` printed; nil when not asked
	ClaudeErr   error             // why Claude Code could not be asked (ErrNoCLI without its CLI)
	SameFolder  bool              // whether Claude Code reads the plugin from the local marketplace
	CodexUsed   bool              // whether Codex is used here: on the PATH, or configured for the user or the project
	StateDir    string            // the folder of the state
	StateErr    error             // why the state cannot be written
}

// Levels of a finding.
const (
	LevelOK   = "ok"
	LevelWarn = "warn"
	LevelFail = "fail"
)

// Valid reports why f is not a finding: a known level, an area and a message.
func (f Finding) Valid() error {
	switch {
	case !slices.Contains([]string{LevelOK, LevelWarn, LevelFail}, f.Level):
		return fmt.Errorf("unknown level %q of a finding", f.Level)
	case f.Area == "" || f.Message == "":
		return errors.New("a finding needs an area and a message")
	}
	return nil
}

// Valid reports why i is not an inspection: its setup must be valid.
func (i Inspection) Valid() error { return i.Setup.Valid() }

// Problem reports whether the finding is a warning or a failure.
func (f Finding) Problem() bool { return f.Level != LevelOK }

// Failure reports whether the finding is a failure.
func (f Finding) Failure() bool { return f.Level == LevelFail }

// found returns the finding of level about area, with the message of format and args; none when it is not valid.
func found(level, area, format string, args ...any) []Finding {
	f := Finding{Level: level, Area: area, Message: fmt.Sprintf(format, args...)}
	if err := f.Valid(); err != nil {
		return nil
	}
	return []Finding{f}
}

// Failed reports whether any of the findings is a failure.
func Failed(findings []Finding) bool { return slices.ContainsFunc(findings, Finding.Failure) }
