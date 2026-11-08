package workflow

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/pkg/cli"
	"github.com/mlophez/polix-ai/pkg/git"
)

// FormatWorkflowRequest is what the caller asks of the use case FormatWorkflow: the command fills it from its flags.
type FormatWorkflowRequest struct {
	Dir   string // any folder of the repository
	Check bool   // only tell whether polix.yaml is formatted, without writing it
}

// FormatWorkflowResponse is what the use case FormatWorkflow gives back: the command prints it.
type FormatWorkflowResponse struct {
	Missing bool   // there is no polix.yaml
	Problem string // why polix.yaml cannot be formatted; empty when it can
	Changed bool   // the format changes polix.yaml
	Written bool   // polix.yaml was written formatted
}

// FormatWorkflowPort is what the shell of FormatWorkflow needs from the outside; FormatWorkflowAdapter implements it.
type FormatWorkflowPort interface {
	// ReadWorkflow returns the data of polix.yaml and whether it exists; the error tells why it cannot be read.
	ReadWorkflow(ctx context.Context) ([]byte, bool, error)
	// WriteWorkflow writes data to polix.yaml; the error tells why it cannot be written.
	WriteWorkflow(ctx context.Context, data []byte) error
}

// FormatWorkflowCommand is the command `polix config fmt`, the entry of the use case FormatWorkflow.
func FormatWorkflowCommand() *cobra.Command {
	var req FormatWorkflowRequest
	c := &cobra.Command{
		Use:   "fmt",
		Short: "Format polix.yaml",
		Long: "Format polix.yaml in place: two spaces of indentation, flow collections on one line, the comments and " +
			"blank lines kept, and the keys of each section in the order of the reference, with guardrails and on " +
			"at the end. With --check it only tells whether polix.yaml is formatted, and exits with 1 when not.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := git.Open(c.Context(), req.Dir)
			if err != nil {
				return err
			}
			res, err := formatWorkflow(c.Context(), FormatWorkflowAdapter{Repo: repo}, req)
			if err != nil {
				return err
			}
			return cli.Exit(res.print(c.OutOrStdout(), c.ErrOrStderr()))
		},
	}
	cli.DirFlag(c, &req.Dir, ".")
	c.Flags().BoolVar(&req.Check, "check", false, "only tell whether polix.yaml is formatted, exiting with 1 when not")
	return c
}

// formatWorkflow is the shell: it reads polix.yaml through the port, decides with the logic, writes the formatted
// file when the logic says so and builds the response.
func formatWorkflow(ctx context.Context, port FormatWorkflowPort, req FormatWorkflowRequest) (FormatWorkflowResponse,
	error) {
	data, found, err := port.ReadWorkflow(ctx)
	if err != nil {
		return FormatWorkflowResponse{}, err
	}
	out := FormatWorkflow(data, found, req.Check)
	res := FormatWorkflowResponse{Missing: out.Missing, Problem: out.Problem, Changed: out.Changed}
	if out.Write != nil {
		if err := port.WriteWorkflow(ctx, out.Write); err != nil {
			return FormatWorkflowResponse{}, err
		}
		res.Written = true
	}
	return res, nil
}

// print serializes the response to the console and returns the exit code: without polix.yaml, with a problem or
// with a polix.yaml --check finds unformatted, the reason to stderr and 1; else what was done to stdout and 0.
func (res FormatWorkflowResponse) print(stdout, stderr io.Writer) int {
	switch {
	case res.Missing:
		cli.NewUI(stderr).Fail("%s not found", File)
		return 1
	case res.Problem != "":
		cli.NewUI(stderr).Fail("%s: %s", File, res.Problem)
		return 1
	case res.Written:
		cli.NewUI(stdout).Line("%s formatted %s", cli.OKText("ok:"), File)
	case res.Changed:
		cli.NewUI(stderr).Fail("%s is not formatted: run polix config fmt", File)
		return 1
	default:
		cli.NewUI(stdout).Line("%s %s is formatted", cli.OKText("ok:"), File)
	}
	return 0
}
