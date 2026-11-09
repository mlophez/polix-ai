// Package graph is the use case GraphWorkflow, the command `polix graph`: it prints the workflow of polix.yaml as a
// Mermaid flowchart.
package graph

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Request is what the caller asks of the use case GraphWorkflow: the command fills it from its flags.
type Request struct {
	Dir     string // any folder of the repository
	Version string // version of the running polix, which polix.yaml may require
}

// Response is what the use case GraphWorkflow gives back: the command prints it.
type Response struct {
	Mermaid string // the flowchart
	Missing bool   // there is no polix.yaml to draw
}

// Port is what the shell of GraphWorkflow needs from the outside; Adapter implements it.
type Port interface {
	// LoadWorkflow returns the workflow of polix.yaml for the polix of version running and whether the file
	// exists; the problems of a workflow that parses are not an error.
	LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, bool, error)
}

// Command is the command `polix graph`, the entry of the use case GraphWorkflow.
func Command() *cobra.Command {
	req := Request{Version: build.Current()}
	c := &cobra.Command{
		Use:   "graph",
		Short: "Print the workflow as a Mermaid diagram",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := git.Open(c.Context(), req.Dir)
			if err != nil {
				return err
			}
			res, err := run(c.Context(), Adapter{Repo: repo}, req)
			if err != nil {
				return err
			}
			if res.Missing {
				return fmt.Errorf("%s not found", workflow.File)
			}
			res.print(c.OutOrStdout())
			return nil
		},
	}
	cli.DirFlag(c, &req.Dir, ".")
	return c
}

// run is the shell: it loads the workflow through the port, draws it with the logic and builds the response.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	wf, found, err := port.LoadWorkflow(ctx, req.Version)
	if err != nil {
		return Response{}, err
	}
	out := Decide(wf, found)
	return Response(out), nil
}

// print writes the flowchart to stdout.
func (res Response) print(stdout io.Writer) {
	fmt.Fprint(stdout, res.Mermaid)
}
