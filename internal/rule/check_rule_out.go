package rule

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
}

// checkRuleTimeout bounds one check, within the deadline of the caller.
const checkRuleTimeout = 5 * time.Minute

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

// RunCheck runs the check of run from the root: the argv of its runner, or without one the file as a program
// (on Windows through its interpreter by extension, elsewhere through its shebang). On Windows a runner of bash,
// sh, python, python3 or py runs the program the PATH names unless it is a Store alias or, for bash and sh, the
// bash of WSL in the Windows folder; then the interpreter of its kind (proc.Command).
func (a CheckRuleAdapter) RunCheck(ctx context.Context, run Run) (string, error) {
	check := filepath.Join(a.Repo.Root, filepath.FromSlash(run.Rule.Check))
	return proc.RunCheck(ctx, a.Repo.Root, run.Argv, check, checkRuleTimeout)
}
