// Package measure is the use case MeasureDiff, the command `polix risk`: it prints the size and the risk score of
// the change, file by file, so a person calibrates the thresholds of the conditions and the weights of risk.weights.
package measure

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Request is what the caller asks of the use case MeasureDiff: the command fills it from its flags.
type Request struct {
	Dir     string   // any folder of the repository
	Changed []string // the globs the measure is narrowed to, as the condition changed takes them; none for all
	Commit  string   // the revision of a commit to measure; empty for the changes not committed yet
	Version string   // version of the running polix, which polix.yaml may require
}

// Response is what the use case MeasureDiff gives back: the command prints it.
type Response struct {
	Commit string // the short hash and the subject of the commit measured; empty for the uncommitted changes
	Lines  int    // changed_lines
	Files  int    // changed_files
	Risk   int    // risk_score
	Rows   []Row  // one per file, the highest contribution first, then by path
}

// Row is the measure of one file: its lines and its share in the risk score.
type Row struct {
	Path    string
	Added   int
	Deleted int
	Binary  bool
	Factor  float64 // its weight in the risk score
	Score   float64 // its contribution to the risk score, before rounding
}

// Port is what the shell of MeasureDiff needs from the outside; Adapter implements it.
type Port interface {
	// LoadWorkflow returns the workflow of polix.yaml for the polix of version running, the default one without the
	// file; a file that cannot be read, does not parse or is not valid is an error.
	LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, error)
	// Diff returns the size of the change of each file not committed yet, untracked files included.
	Diff(ctx context.Context) (diff.Diff, error)
	// CommitDiff returns the size of the change of each file the commit rev introduced, against its first parent
	// or the empty tree, and its short hash and subject.
	CommitDiff(ctx context.Context, rev string) (diff.Diff, string, error)
}

// Command is the command `polix risk`, the entry of the use case MeasureDiff.
func Command() *cobra.Command {
	req := Request{Version: build.Current()}
	c := &cobra.Command{
		Use:   "risk",
		Short: "Print the size and the risk score of the change, file by file",
		Long: "Print the measures the conditions changed_lines, changed_files and risk_score compare, then the share " +
			"of each file, to calibrate their thresholds and the weights of risk.weights. Without --commit it " +
			"measures the changes not committed yet; with it, the changes a commit introduced.",
		Example: "  polix risk --changed '*.go'\n  polix risk --commit HEAD~3 --changed '*.go'",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if c.Flags().Changed("commit") && req.Commit == "" {
				return errors.New("--commit needs a revision")
			}
			repo, err := git.Open(c.Context(), req.Dir)
			if err != nil {
				return err
			}
			res, err := run(c.Context(), Adapter{Repo: repo}, req)
			if err != nil {
				return err
			}
			res.print(c.OutOrStdout())
			return nil
		},
	}
	cli.DirFlag(c, &req.Dir, ".")
	c.Flags().StringArrayVar(&req.Changed, "changed", nil, "measure only the files matching this glob (repeatable)")
	c.Flags().StringVar(&req.Commit, "commit", "", "measure the changes of this commit instead of the uncommitted ones")
	return c
}

// run is the shell: it loads the weights and reads the diff through the port, the uncommitted one or
// that of the commit of the request, measures it with the logic and builds the response.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	wf, err := port.LoadWorkflow(ctx, req.Version)
	if err != nil {
		return Response{}, err
	}
	var d diff.Diff
	summary := ""
	if req.Commit != "" {
		d, summary, err = port.CommitDiff(ctx, req.Commit)
	} else {
		d, err = port.Diff(ctx)
	}
	if err != nil {
		return Response{}, err
	}
	out := Decide(d, wf.Weights, req.Changed)
	res := Response{Commit: summary, Lines: out.Lines, Files: out.Files, Risk: out.Risk}
	for i, ct := range out.Contributions {
		ch := out.Changes[i]
		res.Rows = append(res.Rows, Row{Path: ct.Path, Added: ch.Added, Deleted: ch.Deleted,
			Binary: ch.Binary, Factor: ct.Factor, Score: ct.Score})
	}
	return res, nil
}

// print writes what was measured, the totals with the names of the conditions, and one line per file: added and
// deleted lines, weight, contribution and path.
func (res Response) print(stdout io.Writer) {
	what := "uncommitted changes"
	if res.Commit != "" {
		what = "commit " + res.Commit
	}
	fmt.Fprintf(stdout, "%s\n\n", what)
	fmt.Fprintf(stdout, "changed_lines  %d\nchanged_files  %d\nrisk_score     %d\n", res.Lines, res.Files, res.Risk)
	if len(res.Rows) == 0 {
		return
	}
	fmt.Fprintf(stdout, "\n%7s %7s %7s %7s  %s\n", "added", "deleted", "weight", "score", "path")
	for _, r := range res.Rows {
		added, deleted := strconv.Itoa(r.Added), strconv.Itoa(r.Deleted)
		if r.Binary {
			added, deleted = "-", "-"
		}
		fmt.Fprintf(stdout, "%7s %7s %7s %7.2f  %s\n", added, deleted, strconv.FormatFloat(r.Factor, 'g', -1, 64),
			r.Score, r.Path)
	}
}
