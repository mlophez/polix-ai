package rule

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/file"
	"github.com/mlophez/polix-ai/pkg/git"
	"github.com/mlophez/polix-ai/pkg/proc"
)

// CheckRuleAdapter implements CheckRulePort on the real disk, git and processes of a repository. The entry fills
// it and passes it as the port; there is no constructor.
type CheckRuleAdapter struct {
	Repo *git.Repo // the repository whose rules are checked
	Log  io.Writer // where RunCheck reports each invocation, its rules, outcome and duration; nil reports nothing
}

// checkRuleTimeout bounds one check, within the deadline of the caller.
const checkRuleTimeout = 5 * time.Minute

// checkRuleLogIDs is how many ids of the rules of an invocation the log names before it only counts them.
const checkRuleLogIDs = 5

// LoadRules reads the rules from the yaml files of ai/rules/ (ParseFiles), without retired.yaml, ordered by id.
// Without the folder there are no rules.
func (a CheckRuleAdapter) LoadRules(_ context.Context) ([]Rule, error) {
	files, err := file.ReadDir(filepath.Join(a.Repo.Root, Dir), ".yaml")
	if err != nil {
		return nil, err
	}
	return ParseFiles(files)
}

// LoadWorkflow reads polix.yaml with the domain of workflow, as a use case of workflow would break the vertical
// slices. Without polix.yaml it is the default workflow; a polix.yaml that cannot be read, cannot be parsed or
// requires a newer polix is an error, never a silent default. The problems of a workflow that parses (Invalid)
// are not errors here: the shell stops on those of its runners, and polix check reports the rest.
func (a CheckRuleAdapter) LoadWorkflow(_ context.Context, running string) (workflow.Workflow, error) {
	data, _, err := file.Read(filepath.Join(a.Repo.Root, workflow.File))
	if err != nil {
		return workflow.Workflow{}, err
	}
	wf, err := workflow.Parse(data, running)
	if _, invalid := errors.AsType[workflow.Invalid](err); err != nil && !invalid {
		return workflow.Workflow{}, err
	}
	return wf, nil
}

// Changed asks git for the files changed against the first of candidates that exists, locally or on origin. When
// none exists the error says how to name one; a failure of git is returned as it is.
func (a CheckRuleAdapter) Changed(ctx context.Context, candidates []string) ([]string, error) {
	ref, err := a.Repo.ResolveBase(ctx, candidates)
	if err != nil {
		return nil, fmt.Errorf("%w: set base in %s or pass --base", err, workflow.File)
	}
	return a.Repo.Changed(ctx, ref)
}

// RunCheck runs one invocation from the root, which may decide several rules: the argv of its runner, or without
// one the check of its first rule, the only one then, as a program
// (on Windows through its interpreter by extension, elsewhere through its shebang). On Windows a runner of bash,
// sh, python, python3 or py runs the program the PATH names unless it is a Store alias or, for bash and sh, the
// bash of WSL in the Windows folder; then the interpreter of its kind (proc.Command). With Log, it reports the
// command and the rules it decides before running, and whether it passed and how long it took after.
func (a CheckRuleAdapter) RunCheck(ctx context.Context, run Run) (string, error) {
	var check string
	if len(run.Rules) > 0 {
		check = filepath.Join(a.Repo.Root, filepath.FromSlash(run.Rules[0].Check))
	}
	start := time.Now()
	a.logStart(run)
	out, err := proc.RunCheck(ctx, a.Repo.Root, run.Argv, check, checkRuleTimeout)
	a.logEnd(err, time.Since(start))
	return out, err
}

// logStart reports the invocation about to run: its command, or the check run as a program, and its rules.
func (a CheckRuleAdapter) logStart(run Run) {
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
	if len(ids) > checkRuleLogIDs {
		what = fmt.Sprintf("%d rules: %s, ...", len(ids), strings.Join(ids[:checkRuleLogIDs], ", "))
	}
	fmt.Fprintf(a.Log, "run %s (%s)\n", cmd, what)
}

// logEnd reports how the invocation ended and how long it took.
func (a CheckRuleAdapter) logEnd(err error, took time.Duration) {
	if a.Log == nil {
		return
	}
	verdict := "ok"
	if err != nil {
		verdict = "fail"
	}
	fmt.Fprintf(a.Log, "    %s in %s\n", verdict, took.Round(10*time.Millisecond))
}
