package version

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/pkg/build"
)

// ShowVersionRequest is what the caller asks of the use case ShowVersion: the command fills it.
type ShowVersionRequest struct {
	Version string // version of the running polix
}

// ShowVersionResponse is what the use case ShowVersion gives back to the caller: the command prints it.
type ShowVersionResponse struct {
	Version string
}

// ShowVersionPort is what the shell of ShowVersion needs from the outside: nothing, since the entry passes the
// version in the request.
type ShowVersionPort interface{}

// ShowVersionCommand is the command `polix version`, the entry of the use case ShowVersion: it passes the version
// of the binary in a ShowVersionRequest, runs the shell and prints the ShowVersionResponse.
func ShowVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of polix",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			res, err := showVersion(c.Context(), ShowVersionAdapter{}, ShowVersionRequest{Version: build.Current()})
			if err != nil {
				return err
			}
			res.print(c.OutOrStdout())
			return nil
		},
	}
}

// showVersion is the shell, from request to response: it calls the logic and builds the response.
func showVersion(_ context.Context, _ ShowVersionPort, req ShowVersionRequest) (ShowVersionResponse, error) {
	out := ShowVersion(req.Version)
	return ShowVersionResponse(out), nil
}

// print writes the version in one line to stdout.
func (res ShowVersionResponse) print(stdout io.Writer) {
	fmt.Fprintln(stdout, res.Version)
}
