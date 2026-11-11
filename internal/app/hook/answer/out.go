package answer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/glob"
	"github.com/mlophez/polix-ai/internal/platform/proc"
)

// Adapter implements Port on the real disk, git and processes of a repository. The entry
// passes one with the loaders of the rules, of polix.yaml and of the log as the port; Open fills it with the
// repository and the deadline of the call.
type Adapter struct {
	Repo     *git.Repo     // the repository of the call, once open
	Deadline time.Time     // end of the budget of every run of the call together, once open
	Rules    rule.Load     // the loader of the rules (internal/app/rule/load), which cmd/polix builds once
	Workflow workflow.Load // the loader of polix.yaml (internal/app/workflow/load), which cmd/polix builds once too
	State    state.Load    // the loader of the log (internal/app/state/load), which cmd/polix builds once too
	Look     proc.LookFunc // how a program is found on the PATH (exec.LookPath)
}

// checkTimeout bounds the check of one rule, within the budget of the call.
const checkTimeout = 5 * time.Minute

// Open opens the repository that contains dir (the working directory when empty) and starts the budget of the
// runs of the call.
func (a *Adapter) Open(ctx context.Context, dir string) (string, string, error) {
	repo, err := git.Open(ctx, dir)
	if err != nil {
		return "", "", err
	}
	a.Repo, a.Deadline = repo, time.Now().Add(hook.RunBudget)
	return repo.Root, repo.GitDir, nil
}

// LoadWorkflow loads polix.yaml of the repository through the loader of polix.yaml. Without the file there is no
// workflow and no error; a file that cannot be read, cannot be parsed, has problems or requires a newer polix
// exists, with its error.
func (a *Adapter) LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, bool, error) {
	loaded := a.Workflow(ctx, filepath.Join(a.Repo.Root, workflow.File), running)
	if !loaded.Exists {
		return workflow.Workflow{}, false, nil
	}
	return loaded.Workflow, true, loaded.Err
}

// Locate turns a path given by the agent from cwd into one relative to the root, symlinks resolved.
func (a *Adapter) Locate(_ context.Context, path, cwd string) (string, bool) {
	return a.Repo.Rel(path, cwd)
}

// Head returns the current branch and the commit HEAD points to.
func (a *Adapter) Head(ctx context.Context) (branch, commit string) { return a.Repo.Head(ctx) }

// Hash returns the content hash, computed with a scratch index in the folder of the state.
func (a *Adapter) Hash(ctx context.Context) (string, error) {
	return a.Repo.ContentHash(ctx, filepath.Join(a.Repo.GitDir, state.DirName))
}

// Refs returns what git moves without moving HEAD, through git.
func (a *Adapter) Refs(ctx context.Context) string { return a.Repo.Refs(ctx) }

// TreeDiff returns the paths that differ between two content hashes and those the second no longer holds, through
// git; a tree git no longer has is the error.
func (a *Adapter) TreeDiff(ctx context.Context, from, to string) (paths, removed []string, err error) {
	return a.Repo.TreeDiff(ctx, from, to)
}

// Changed returns the files not committed yet; a failure of git reads as no files. It swallows the error, so it
// runs detached from the cancellation of ctx, bounded by the timeout of internal/platform/git.
func (a *Adapter) Changed(ctx context.Context) []string {
	files, _ := a.Repo.Changed(context.WithoutCancel(ctx), "")
	return files
}

// Diff returns the size of the change of each file not committed yet, from git, validated; a failure of git or an
// invalid diff is the error. Like Changed, it runs detached from the cancellation of ctx.
func (a *Adapter) Diff(ctx context.Context) (diff.Diff, error) {
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
func (a *Adapter) Files(ctx context.Context) []string {
	files, _ := a.Repo.Files(context.WithoutCancel(ctx))
	return files
}

// LoadRules loads the rules of the folder dir of the repository through the loader of the rules: every rule but
// those of retired.yaml, ordered by id, invalid ones included, or the error of a file that cannot be read or is not
// yaml. Without the folder there are no rules.
func (a *Adapter) LoadRules(ctx context.Context, dir string) ([]rule.Rule, error) {
	loaded := a.Rules(ctx, a.Repo.Root, dir)
	return loaded.Rules, loaded.Err
}

// Ignored reports whether git ignores the path.
func (a *Adapter) Ignored(ctx context.Context, path string) bool {
	return a.Repo.Ignored(ctx, path)
}

// Real returns the real path of each path, symlinks of its longest existing prefix resolved.
func (a *Adapter) Real(_ context.Context, paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = git.RealPath(p)
	}
	return out
}

// Foreign reports whether the absolute folder dir is inside an existing git repository other than this one.
func (a *Adapter) Foreign(ctx context.Context, dir string) bool {
	return a.Repo.Foreign(ctx, dir)
}

// Missing returns the programs the lookup does not find, in their order. exec.LookPath searches the PATH the hooks
// give polix, as a command of run is found; on Windows it tries the extensions of PATHEXT, so a name without
// extension finds its .exe. A program found only in the current folder (exec.ErrDot) is missing.
func (a *Adapter) Missing(_ context.Context, programs []string) []string {
	var missing []string
	for _, p := range programs {
		if _, err := a.Look(p); err != nil {
			missing = append(missing, p)
		}
	}
	return missing
}

// Read returns the content of a file of the repository.
func (a *Adapter) Read(_ context.Context, path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(a.Repo.Root, filepath.FromSlash(path)))
	return string(data), err
}

// Run runs argv from the root within the budget of the call.
func (a *Adapter) Run(ctx context.Context, argv []string) (string, error) {
	ctx, cancel := context.WithDeadline(ctx, a.Deadline)
	defer cancel()
	return proc.Run(ctx, a.Repo.Root, argv, hook.RunBudget)
}

// RunCheck runs one invocation of run from the root within the budget of the call, which may decide several
// rules: the argv of its runner, or without one the check of its first rule, the only one then, as a program (on Windows through its interpreter by extension, elsewhere through its shebang).
// On Windows a runner of bash, sh, python, python3 or py runs the program the PATH names unless it is a Store
// alias or, for bash and sh, the bash of WSL in the Windows folder; then the interpreter of its kind (proc.Command).
func (a *Adapter) RunCheck(ctx context.Context, run rule.Run) (string, error) {
	ctx, cancel := context.WithDeadline(ctx, a.Deadline)
	defer cancel()
	var check string
	if len(run.Rules) > 0 {
		check = filepath.Join(a.Repo.Root, filepath.FromSlash(run.Rules[0].Check))
	}
	return proc.RunCheck(ctx, a.Repo.Root, run.Argv, check, checkTimeout)
}

// Restore puts the paths back in the working tree as the tree source holds them, removing those it does not hold,
// through git with a scratch index in the folder of the state, as Hash, so the real index is never touched.
func (a *Adapter) Restore(ctx context.Context, source string, paths []string) error {
	return a.Repo.Restore(ctx, filepath.Join(a.Repo.GitDir, state.DirName), source, paths)
}

// Contains reports whether a file git does not ignore, matching one of the patterns, holds text; a file that
// cannot be read does not.
func (a *Adapter) Contains(ctx context.Context, patterns []string, text string) bool {
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
