// Package show is the use case ShowVersion, the command `polix version`: it prints the version of the running
// polix.
package show

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/platform/build"
)

// Request is what the caller asks of the use case ShowVersion: the command fills it.
type Request struct {
	Version string // version of the running polix
}

// Response is what the use case ShowVersion gives back to the caller: the command prints it.
type Response struct {
	Version string
}

// Port is what the shell of ShowVersion needs from the outside: nothing, since the entry passes the
// version in the request.
type Port interface{} //nolint:modernize // GOARCH-12: a port is an interface literal, and any is a named type

// Command is the command `polix version`, the entry of the use case ShowVersion: it passes the version
// of the binary in a Request, runs the shell and prints the Response.
func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of polix",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			res, err := run(c.Context(), Adapter{}, Request{Version: build.Current()})
			if err != nil {
				return err
			}
			res.print(c.OutOrStdout())
			return nil
		},
	}
}

// run is the shell, from request to response: it calls the logic and builds the response.
//
//nolint:unparam // every shell returns (response, error), as docs/architecture.md says, even when it cannot fail
func run(_ context.Context, _ Port, req Request) (Response, error) {
	out := Decide(req.Version)
	return Response(out), nil
}

// print writes the version in one line to stdout.
func (res Response) print(stdout io.Writer) {
	fmt.Fprintln(stdout, res.Version)
}
