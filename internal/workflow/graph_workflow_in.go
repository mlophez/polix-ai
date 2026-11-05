package workflow

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/pkg/build"
	"github.com/mlophez/polix-ai/pkg/cli"
	"github.com/mlophez/polix-ai/pkg/git"
)

// GraphWorkflowRequest is what the caller asks of the use case GraphWorkflow: the command fills it from its flags.
type GraphWorkflowRequest struct {
	Dir     string // any folder of the repository
	Version string // version of the running polix, which polix.yaml may require
}

// GraphWorkflowResponse is what the use case GraphWorkflow gives back: the command prints it.
type GraphWorkflowResponse struct {
	Mermaid string // the flowchart
	Missing bool   // there is no polix.yaml to draw
}

// GraphWorkflowPort is what the shell of GraphWorkflow needs from the outside; GraphWorkflowAdapter implements it.
type GraphWorkflowPort interface {
	// LoadWorkflow returns the workflow of polix.yaml for the polix of version running and whether the file
	// exists; the problems of a workflow that parses are not an error.
	LoadWorkflow(ctx context.Context, running string) (Workflow, bool, error)
}

// GraphWorkflowCommand is the command `polix graph`, the entry of the use case GraphWorkflow.
func GraphWorkflowCommand() *cobra.Command {
	req := GraphWorkflowRequest{Version: build.Current()}
	c := &cobra.Command{
		Use:   "graph",
		Short: "Print the workflow as a Mermaid diagram",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := git.Open(c.Context(), req.Dir)
			if err != nil {
				return err
			}
			res, err := graphWorkflow(c.Context(), GraphWorkflowAdapter{Repo: repo}, req)
			if err != nil {
				return err
			}
			if res.Missing {
				return fmt.Errorf("%s not found", File)
			}
			res.print(c.OutOrStdout())
			return nil
		},
	}
	cli.DirFlag(c, &req.Dir, ".")
	return c
}

// graphWorkflow is the shell: it loads the workflow through the port, draws it with the logic and builds the
// response.
func graphWorkflow(ctx context.Context, port GraphWorkflowPort, req GraphWorkflowRequest) (GraphWorkflowResponse, error) {
	wf, found, err := port.LoadWorkflow(ctx, req.Version)
	if err != nil {
		return GraphWorkflowResponse{}, err
	}
	out := GraphWorkflow(wf, found)
	return GraphWorkflowResponse(out), nil
}

// print writes the flowchart to stdout.
func (res GraphWorkflowResponse) print(stdout io.Writer) {
	fmt.Fprint(stdout, res.Mermaid)
}
