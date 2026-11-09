package measure

import (
	"context"
	"path/filepath"

	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Adapter implements Port on the disk and the git of a repository. The entry fills it; there is no constructor.
type Adapter struct {
	Repo *git.Repo // the repository whose change is measured
}

// LoadWorkflow reads polix.yaml: without the file, the default workflow, which has no weights; a file that cannot
// be read, does not parse, is not valid or requires a newer polix is an error.
func (a Adapter) LoadWorkflow(_ context.Context, running string) (workflow.Workflow, error) {
	data, existed, err := file.Read(filepath.Join(a.Repo.Root, workflow.File))
	if err != nil {
		return workflow.Workflow{}, err
	}
	if !existed {
		data = nil
	} else if data == nil {
		data = []byte{}
	}
	return workflow.Parse(data, running)
}

// Diff reads the size of the change of each file not committed yet with git (Repo.Stats) and validates it.
func (a Adapter) Diff(ctx context.Context) (diff.Diff, error) {
	stats, err := a.Repo.Stats(ctx, "")
	if err != nil {
		return diff.Diff{}, err
	}
	return a.diff(stats)
}

// CommitDiff reads the size of the change of each file of the commit rev with git (Repo.CommitStats), validates it
// and returns it with the short hash and the subject of the commit.
func (a Adapter) CommitDiff(ctx context.Context, rev string) (diff.Diff, string, error) {
	stats, err := a.Repo.CommitStats(ctx, rev)
	if err != nil {
		return diff.Diff{}, "", err
	}
	d, err := a.diff(stats)
	if err != nil {
		return diff.Diff{}, "", err
	}
	summary, err := a.Repo.Summary(ctx, rev)
	if err != nil {
		return diff.Diff{}, "", err
	}
	return d, summary, nil
}

// diff builds the domain value of the stats of git and validates it.
func (a Adapter) diff(stats []git.Stat) (diff.Diff, error) {
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
