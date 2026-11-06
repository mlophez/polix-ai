package hook

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mlophez/polix-ai/internal/diff"
	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/file"
	"github.com/mlophez/polix-ai/pkg/git"
	"github.com/mlophez/polix-ai/pkg/glob"
	"github.com/mlophez/polix-ai/pkg/jsonl"
	"github.com/mlophez/polix-ai/pkg/proc"
)

// AnswerHookAdapter implements AnswerHookPort on the real disk, git and processes of a repository. The entry
// passes an empty one as the port; Open fills it with the repository and the deadline of the call.
type AnswerHookAdapter struct {
	Repo     *git.Repo // the repository of the call, once open
	Deadline time.Time // end of the budget of every run of the call together, once open
}

// answerHookCheckTimeout bounds the check of one rule, within the budget of the call.
const answerHookCheckTimeout = 5 * time.Minute

// Open opens the repository that contains dir (the working directory when empty) and starts the budget of the
// runs of the call.
func (a *AnswerHookAdapter) Open(ctx context.Context, dir string) (string, string, error) {
	repo, err := git.Open(ctx, dir)
	if err != nil {
		return "", "", err
	}
	a.Repo, a.Deadline = repo, time.Now().Add(RunBudget)
	return repo.Root, repo.GitDir, nil
}

// LoadWorkflow reads polix.yaml with the domain of workflow. Without the file there is no workflow and no error;
// a file that cannot be read, cannot be parsed, has problems or requires a newer polix exists, with its error.
func (a *AnswerHookAdapter) LoadWorkflow(_ context.Context, running string) (workflow.Workflow, bool, error) {
	data, existed, err := file.Read(filepath.Join(a.Repo.Root, filepath.FromSlash(workflow.File)))
	if err != nil {
		return workflow.Workflow{}, true, err
	}
	if !existed {
		return workflow.Workflow{}, false, nil
	}
	if data == nil {
		data = []byte{}
	}
	wf, err := workflow.Parse(data, running)
	return wf, true, err
}

// Locate turns a path given by the agent from cwd into one relative to the root, symlinks resolved.
func (a *AnswerHookAdapter) Locate(_ context.Context, path, cwd string) (string, bool) {
	return a.Repo.Rel(path, cwd)
}

// Branch returns the current branch.
func (a *AnswerHookAdapter) Branch(ctx context.Context) string { return a.Repo.Branch(ctx) }

// Hash returns the content hash, computed with a scratch index in the folder of the state.
func (a *AnswerHookAdapter) Hash(ctx context.Context) (string, error) {
	return a.Repo.ContentHash(ctx, filepath.Join(a.Repo.GitDir, state.DirName))
}

// Changed returns the files not committed yet; a failure of git reads as no files. It swallows the error, so it
// runs detached from the cancellation of ctx, bounded by the timeout of pkg/git.
func (a *AnswerHookAdapter) Changed(ctx context.Context) []string {
	files, _ := a.Repo.Changed(context.WithoutCancel(ctx), "")
	return files
}

// Diff returns the size of the change of each file not committed yet, from git, validated; a failure of git or an
// invalid diff is the error. Like Changed, it runs detached from the cancellation of ctx.
func (a *AnswerHookAdapter) Diff(ctx context.Context) (diff.Diff, error) {
	stats, err := a.Repo.Stats(context.WithoutCancel(ctx), "")
	if err != nil {
		return diff.Diff{}, err
	}
	changes := make([]diff.Change, 0, len(stats))
	for _, s := range stats {
		c := diff.Change{Path: s.Path, Added: s.Added, Deleted: s.Deleted, Binary: s.Binary}
		if err := c.Valid(); err != nil {
			return diff.Diff{}, err
		}
		changes = append(changes, c)
	}
	d := diff.Diff{Changes: changes}
	if err := d.Valid(); err != nil {
		return diff.Diff{}, err
	}
	return d, nil
}

// Files returns the files of the working tree that git does not ignore; a failure of git reads as no files. Like
// Changed, it runs detached from the cancellation of ctx.
func (a *AnswerHookAdapter) Files(ctx context.Context) []string {
	files, _ := a.Repo.Files(context.WithoutCancel(ctx))
	return files
}

// LoadRules reads the rules from the yaml files of the folder dir (rule.ParseFilesAll), without retired.yaml,
// ordered by id, invalid ones included. Without the folder there are no rules.
func (a *AnswerHookAdapter) LoadRules(_ context.Context, dir string) ([]rule.Rule, error) {
	files, err := file.ReadDir(filepath.Join(a.Repo.Root, filepath.FromSlash(dir)), ".yaml")
	if err != nil {
		return nil, err
	}
	rules, _, err := rule.ParseFilesAll(dir, files)
	return rules, err
}

// LoadState reads the log through pkg/jsonl: the lines over state.MaxLine are skipped and counted. A log that does
// not exist is empty; one that cannot be read is an error.
func (a *AnswerHookAdapter) LoadState(_ context.Context) (state.State, error) {
	lines, skipped, err := jsonl.Read(a.logPath(), state.MaxLine)
	if err != nil {
		return state.State{}, err
	}
	return state.Parse(lines, skipped)
}

// Ignored reports whether git ignores the path.
func (a *AnswerHookAdapter) Ignored(ctx context.Context, path string) bool {
	return a.Repo.Ignored(ctx, path)
}

// Real returns the real path of each path, symlinks of its longest existing prefix resolved.
func (a *AnswerHookAdapter) Real(_ context.Context, paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = git.RealPath(p)
	}
	return out
}

// Read returns the content of a file of the repository.
func (a *AnswerHookAdapter) Read(_ context.Context, path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(a.Repo.Root, filepath.FromSlash(path)))
	return string(data), err
}

// Run runs argv from the root within the budget of the call.
func (a *AnswerHookAdapter) Run(ctx context.Context, argv []string) (string, error) {
	ctx, cancel := context.WithDeadline(ctx, a.Deadline)
	defer cancel()
	return proc.Run(ctx, a.Repo.Root, argv, RunBudget)
}

// RunCheck runs one invocation of run from the root within the budget of the call, which may decide several
// rules: the argv of its runner, or without one the check of its first rule, the only one then, as a program (on Windows through its interpreter by extension, elsewhere through its shebang).
// On Windows a runner of bash, sh, python, python3 or py runs the program the PATH names unless it is a Store
// alias or, for bash and sh, the bash of WSL in the Windows folder; then the interpreter of its kind (proc.Command).
func (a *AnswerHookAdapter) RunCheck(ctx context.Context, run rule.Run) (string, error) {
	ctx, cancel := context.WithDeadline(ctx, a.Deadline)
	defer cancel()
	var check string
	if len(run.Rules) > 0 {
		check = filepath.Join(a.Repo.Root, filepath.FromSlash(run.Rules[0].Check))
	}
	return proc.RunCheck(ctx, a.Repo.Root, run.Argv, check, answerHookCheckTimeout)
}

// Append writes e as one line at the end of the log, stamped with the time when it has none.
func (a *AnswerHookAdapter) Append(_ context.Context, e state.Entry) error {
	at := e.At
	if at == "" {
		at = time.Now().Format(time.RFC3339)
	}
	stamped := state.Entry{At: at, Kind: e.Kind, Agent: e.Agent, Session: e.Session, Event: e.Event, Actor: e.Actor,
		Branch: e.Branch, Hash: e.Hash, Fact: e.Fact, Rule: e.Rule, Action: e.Action, Line: e.Line,
		Outcome: e.Outcome, Reason: e.Reason}
	if err := stamped.Valid(); err != nil {
		return err
	}
	line, err := state.EncodeLine(stamped)
	if err != nil {
		return fmt.Errorf("log entry: %w", err)
	}
	return jsonl.Append(a.logPath(), line)
}

// logPath is the path of the log of the repository.
func (a *AnswerHookAdapter) logPath() string {
	return filepath.Join(a.Repo.GitDir, state.DirName, state.LogName)
}

// Contains reports whether a file git does not ignore, matching one of the patterns, holds text; a file that
// cannot be read does not.
func (a *AnswerHookAdapter) Contains(ctx context.Context, patterns []string, text string) bool {
	for _, f := range a.Files(ctx) {
		if !glob.AnyPath(patterns, f) {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(a.Repo.Root, filepath.FromSlash(f))); err == nil &&
			bytes.Contains(data, []byte(text)) {
			return true
		}
	}
	return false
}
