// Package format is the use case FormatWorkflow, the command `polix config fmt`: it formats polix.yaml, or the
// workflow file its argument names, in place, or with --check only tells whether it is formatted.
package format

import (
	"context"
	"errors"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Request is what the caller asks of the use case FormatWorkflow: the command fills it from its argument and flags.
type Request struct {
	Dir   string // any folder of the repository, whose polix.yaml is formatted without File
	File  string // the workflow file to format, as the argument names it; empty for polix.yaml of the repository
	Check bool   // only tell whether the file is formatted, without writing it
}

// Response is what the use case FormatWorkflow gives back: the command prints it.
type Response struct {
	File    string // the file, as the console names it: the argument, or polix.yaml
	Fix     string // with a change, the command that formats the file
	Missing bool   // the workflow file does not exist
	Problem string // why the workflow file cannot be formatted; empty when it can
	Changed bool   // the format changes the workflow file
	Written bool   // the workflow file was written formatted
}

// Port is what the shell of FormatWorkflow needs from the outside; Adapter implements it.
type Port interface {
	// ReadWorkflow returns the data of the workflow file and whether it exists; the error tells why it cannot be read.
	ReadWorkflow(ctx context.Context) ([]byte, bool, error)
	// WriteWorkflow writes data to the workflow file; the error tells why it cannot be written.
	WriteWorkflow(ctx context.Context, data []byte) error
}

// Command is the command `polix config fmt`, the entry of the use case FormatWorkflow.
func Command() *cobra.Command {
	var req Request
	c := &cobra.Command{
		Use:   "fmt [file]",
		Short: "Format polix.yaml",
		Long: "Format polix.yaml in place: two spaces of indentation, flow collections on one line, the comments and " +
			"blank lines kept, and the keys of each section in the order of the reference, with guardrails and on " +
			"at the end. With --check it only tells whether polix.yaml is formatted, and exits with 1 when not. " +
			"With a file, relative to the working folder, it formats that workflow file instead, as " +
			"examples/polix.yaml, and needs no repository.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 1 {
				if args[0] == "" {
					return errors.New("the file cannot be empty: name a workflow file, or none for polix.yaml")
				}
				if c.Flags().Changed("dir") {
					return errors.New("a file and --dir cannot go together: name the file from the working folder")
				}
				req.File = args[0]
			}
			path := req.File
			if path == "" {
				repo, err := git.Open(c.Context(), req.Dir)
				if err != nil {
					return err
				}
				path = filepath.Join(repo.Root, workflow.File)
			}
			res, err := run(c.Context(), Adapter{Path: path}, req)
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
	out := Decide(data, found, req.Check, req.File)
	res := Response{File: out.File, Fix: out.Fix, Missing: out.Missing, Problem: out.Problem, Changed: out.Changed}
	if out.Write != nil {
		if err := port.WriteWorkflow(ctx, out.Write); err != nil {
			return Response{}, err
		}
		res.Written = true
	}
	return res, nil
}

// print serializes the response to the console, naming the file as the request did, and returns the exit code:
// without the file, with a problem or with a file --check finds unformatted, the reason to stderr and 1; else what was done to stdout and 0.
func (res Response) print(stdout, stderr io.Writer) int {
	switch {
	case res.Missing:
		cli.NewUI(stderr).Fail("%s not found", res.File)
		return 1
	case res.Problem != "":
		cli.NewUI(stderr).Fail("%s: %s", res.File, res.Problem)
		return 1
	case res.Written:
		cli.NewUI(stdout).Line("%s formatted %s", cli.OKText("ok:"), res.File)
	case res.Changed:
		cli.NewUI(stderr).Fail("%s is not formatted: run %s", res.File, res.Fix)
		return 1
	default:
		cli.NewUI(stdout).Line("%s %s is formatted", cli.OKText("ok:"), res.File)
	}
	return 0
}
