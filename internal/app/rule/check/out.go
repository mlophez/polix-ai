package check

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/proc"
)

// Adapter implements Port on the real disk, git and processes of a repository. The entry fills
// it and passes it as the port; there is no constructor.
type Adapter struct {
	Repo     *git.Repo     // the repository whose rules are checked
	Log      io.Writer     // where RunCheck reports each invocation, its rules, outcome and duration; nil reports nothing
	Rules    rule.Load     // the loader of the rules (internal/app/rule/load), which cmd/polix builds once
	Workflow workflow.Load // the loader of polix.yaml (internal/app/workflow/load), which cmd/polix builds once too
}

// timeout bounds one check, within the deadline of the caller.
const timeout = 5 * time.Minute

// logIDs is how many ids of the rules of an invocation the log names before it only counts them.
const logIDs = 5

// LoadRules loads the rules of the folder dir of the repository through the loader of the rules, without
// retired.yaml, ordered by id, and stops where a strict reading does (rule.Loaded.Strict): at the first invalid rule
// of the files in the order of their names, or at a file that cannot be read or is not yaml. Without the folder
// there are no rules.
func (a Adapter) LoadRules(ctx context.Context, dir string) ([]rule.Rule, error) {
	loaded := a.Rules(ctx, a.Repo.Root, dir)
	if loaded.Strict != nil {
		return nil, loaded.Strict
	}
	return loaded.Rules, nil
}

// LoadWorkflow loads polix.yaml of the repository through the loader of polix.yaml. Without polix.yaml it is the
// default workflow; a polix.yaml that cannot be read, cannot be parsed or requires a newer polix is an error, never
// a silent default. The problems of a workflow that parses (Invalid) are not errors here: the shell stops on those
// of its runners, and polix config check reports the rest.
func (a Adapter) LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, error) {
	loaded := a.Workflow(ctx, a.Repo.Root, running)
	if _, invalid := errors.AsType[workflow.Invalid](loaded.Err); loaded.Err != nil && !invalid {
		return workflow.Workflow{}, loaded.Err
	}
	return loaded.Workflow, nil
}

// Changed asks git for the files changed against the first of candidates that exists, locally or on origin. When
// none exists the error says how to name one; a failure of git is returned as it is.
func (a Adapter) Changed(ctx context.Context, candidates []string) ([]string, error) {
	ref, err := a.Repo.ResolveBase(ctx, candidates)
	if err != nil {
		return nil, fmt.Errorf("%w: set base_branch in %s or pass --base", err, workflow.File)
	}
	return a.Repo.Changed(ctx, ref)
}

// RunCheck runs one invocation from the root, which may decide several rules: the argv of its runner, or without
// one the check of its first rule, the only one then, as a program
// (on Windows through its interpreter by extension, elsewhere through its shebang). On Windows a runner of bash,
// sh, python, python3 or py runs the program the PATH names unless it is a Store alias or, for bash and sh, the
// bash of WSL in the Windows folder; then the interpreter of its kind (proc.Command). With Log, it reports the
// command and the rules it decides before running, and whether it passed and how long it took after.
func (a Adapter) RunCheck(ctx context.Context, run rule.Run) (string, error) {
	var check string
	if len(run.Rules) > 0 {
		check = filepath.Join(a.Repo.Root, filepath.FromSlash(run.Rules[0].Check))
	}
	start := time.Now()
	a.logStart(run)
	out, err := proc.RunCheck(ctx, a.Repo.Root, run.Argv, check, timeout)
	a.logEnd(err, time.Since(start))
	return out, err
}

// logStart reports the invocation about to run: its command, or the check run as a program, and its rules.
func (a Adapter) logStart(run rule.Run) {
	if a.Log == nil {
		return
	}
	ids := make([]string, len(run.Rules))
	for i, r := range run.Rules {
		ids[i] = r.ID
	}
	cmd := strings.Join(run.Argv, " ")
	if cmd == "" && len(run.Rules) > 0 {
		cmd = run.Rules[0].Check
	}
	what := strings.Join(ids, ", ")
	if len(ids) > logIDs {
		what = fmt.Sprintf("%d rules: %s, ...", len(ids), strings.Join(ids[:logIDs], ", "))
	}
	fmt.Fprintf(a.Log, "run %s (%s)\n", cmd, what)
}

// logEnd reports how the invocation ended and how long it took.
func (a Adapter) logEnd(err error, took time.Duration) {
	if a.Log == nil {
		return
	}
	verdict := "ok"
	if err != nil {
		verdict = "fail"
	}
	fmt.Fprintf(a.Log, "    %s in %s\n", verdict, took.Round(10*time.Millisecond))
}
