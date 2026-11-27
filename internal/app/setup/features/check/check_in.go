// Package check is the use case CheckSetup, the command `polix config check`: it validates the workflow of a
// repository, or another workflow file, and prints every problem. It never reads the rules.
package check

import (
	"context"
	"errors"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Request is what the caller asks of the use case CheckSetup, whatever the transport: the command fills
// it from its flags.
type Request struct {
	Dir     string // any folder of the repository
	File    string // the workflow file to check in place of polix.yaml, as the argument names it; empty for none
	Version string // version of the running polix, which polix.yaml may require
}

// Response is what the use case CheckSetup gives back to the caller, in the shape of the transport: the
// command serializes it to the console.
type Response struct {
	File             string   // the workflow file, as the console names it: the argument, or polix.yaml
	WorkflowProblems []string // every problem of the workflow file
	Hint             string   // the hint that the problems of the workflow may come from a newer polix; empty for none
	Found            bool     // whether the workflow file exists
}

// Port is what the shell of CheckSetup needs from the outside: the workflow. It speaks in domain values, and every
// method takes the context of the caller.
type Port interface {
	// LoadWorkflow returns the workflow of polix.yaml for the polix of version running and whether the file exists:
	// the default workflow without it. The error tells why it cannot be read or parsed, or is a workflow.InvalidError
	// with the problems of a workflow that parses, returned with it.
	LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, bool, error)
}

// Command is the command `polix config check`, a subcommand of `polix config` and the entry of the use
// case CheckSetup: it turns the flags into a Request, injects the adapter with the loader loadWorkflow as the port,
// runs the shell and serializes the Response to the console.
func Command(loadWorkflow workflow.Load) *cobra.Command {
	req := Request{Version: build.Current()}
	c := &cobra.Command{
		Use:   "check [file]",
		Short: "Validate polix.yaml",
		Long: "Validate the workflow and print every problem: unknown events, conditions, actions and " +
			"keys, undefined variables and conditions on events where they never hold. With a file, relative to " +
			"the working folder, it validates that workflow file in place of polix.yaml, as examples/polix.yaml. " +
			"The rules are validated by polix rules check.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			repo, err := git.Open(c.Context(), req.Dir)
			if err != nil {
				return err
			}
			path := filepath.Join(repo.Root, workflow.File)
			if len(args) == 1 {
				if args[0] == "" {
					return errors.New("the file cannot be empty: name a workflow file, or none for polix.yaml")
				}
				req.File, path = args[0], args[0]
			}
			res, err := run(c.Context(), Adapter{Path: path, Workflow: loadWorkflow}, req)
			if err != nil {
				return err
			}
			return cli.Exit(res.print(c.OutOrStdout(), c.ErrOrStderr()))
		},
	}
	cli.DirFlag(c, &req.Dir, ".")
	return c
}

// run is the shell, from request to response: it reads the workflow through the port, builds the inspection of
// the setup of the running version, calls the logic once and builds the response.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	wf, found, wfErr := port.LoadWorkflow(ctx, req.Version)
	s := setup.Setup{Version: req.Version}
	if err := s.Valid(); err != nil {
		return Response{}, err
	}
	i := setup.Inspection{Setup: s, Workflow: wf, Found: found, WorkflowErr: wfErr}
	if err := i.Valid(); err != nil {
		return Response{}, err
	}
	return Response(Decide(i, req.File)), nil
}

// print serializes the response to the console and returns the exit code: with problems, a report to stderr with a
// section Workflow, its problems as fail rows and the hint of a newer polix as the fix of the last problem, and 1;
// else the summary to stdout and 0.
func (res Response) print(stdout, stderr io.Writer) int {
	if len(res.WorkflowProblems) > 0 {
		workflowRows := rows(res.WorkflowProblems)
		workflowRows[len(workflowRows)-1].Fix = res.Hint
		cli.NewUI(stderr).Report(cli.Report{Sections: []cli.Section{
			{Title: "Workflow", Detail: res.File, Rows: workflowRows},
		}})
		return 1
	}
	u := cli.NewUI(stdout)
	if !res.Found {
		u.Line("%s no %s (only the built-in protections apply)", cli.OKText("ok:"), res.File)
		return 0
	}
	u.Line("%s %s", cli.OKText("ok:"), res.File)
	return 0
}

// rows returns each problem as a fail row without area.
func rows(problems []string) []cli.Row {
	out := make([]cli.Row, 0, len(problems))
	for _, p := range problems {
		out = append(out, cli.Row{Level: cli.LevelFail, Message: p})
	}
	return out
}
