// Package format is the use case FormatWorkflow, the command `polix config fmt`: it formats polix.yaml in place, or
// with --check only tells whether it is formatted.
package format

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Request is what the caller asks of the use case FormatWorkflow: the command fills it from its flags.
type Request struct {
	Dir   string // any folder of the repository
	Check bool   // only tell whether polix.yaml is formatted, without writing it
}

// Response is what the use case FormatWorkflow gives back: the command prints it.
type Response struct {
	Missing bool   // there is no polix.yaml
	Problem string // why polix.yaml cannot be formatted; empty when it can
	Changed bool   // the format changes polix.yaml
	Written bool   // polix.yaml was written formatted
}

// Port is what the shell of FormatWorkflow needs from the outside; Adapter implements it.
type Port interface {
	// ReadWorkflow returns the data of polix.yaml and whether it exists; the error tells why it cannot be read.
	ReadWorkflow(ctx context.Context) ([]byte, bool, error)
	// WriteWorkflow writes data to polix.yaml; the error tells why it cannot be written.
	WriteWorkflow(ctx context.Context, data []byte) error
}

// Command is the command `polix config fmt`, the entry of the use case FormatWorkflow.
func Command() *cobra.Command {
	var req Request
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
			res, err := run(c.Context(), Adapter{Repo: repo}, req)
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

// run is the shell: it reads polix.yaml through the port, decides with the logic, writes the formatted file when the
// logic says so and builds the response.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	data, found, err := port.ReadWorkflow(ctx)
	if err != nil {
		return Response{}, err
	}
	out := Decide(data, found, req.Check)
	res := Response{Missing: out.Missing, Problem: out.Problem, Changed: out.Changed}
	if out.Write != nil {
		if err := port.WriteWorkflow(ctx, out.Write); err != nil {
			return Response{}, err
		}
		res.Written = true
	}
	return res, nil
}

// print serializes the response to the console and returns the exit code: without polix.yaml, with a problem or
// with a polix.yaml --check finds unformatted, the reason to stderr and 1; else what was done to stdout and 0.
func (res Response) print(stdout, stderr io.Writer) int {
	switch {
	case res.Missing:
		cli.NewUI(stderr).Fail("%s not found", workflow.File)
		return 1
	case res.Problem != "":
		cli.NewUI(stderr).Fail("%s: %s", workflow.File, res.Problem)
		return 1
	case res.Written:
		cli.NewUI(stdout).Line("%s formatted %s", cli.OKText("ok:"), workflow.File)
	case res.Changed:
		cli.NewUI(stderr).Fail("%s is not formatted: run polix config fmt", workflow.File)
		return 1
	default:
		cli.NewUI(stdout).Line("%s %s is formatted", cli.OKText("ok:"), workflow.File)
	}
	return 0
}
