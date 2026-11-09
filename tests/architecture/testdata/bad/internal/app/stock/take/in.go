// Package take is a use case that grows into a sibling of each kind and keeps the rules of the files of a use
// case: the good cases of GOARCH-1, 3, 4, 5, 12, 15, 23, 24, 32 and 37.
package take

import (
	"context"

	"github.com/spf13/cobra"
)

type Request struct{ Shop string }

type Response struct{ Count int }

// Port is the port of the use case, which the adapter implements in a sibling of out.go (GOARCH-5).
type Port interface {
	Load(ctx context.Context, shop string) (string, error)
}

// Command is a cobra command (GOARCH-15) that passes the adapter as the port, and calls neither (GOARCH-32).
func Command() *cobra.Command {
	_, _ = run(context.Background(), Adapter{}, Request{})
	return nil
}

// run reads through a function of a sibling of in.go and calls the logic once (GOARCH-32).
func run(ctx context.Context, port Port, req Request) (Response, error) {
	raw, err := load(ctx, port, req)
	if err != nil {
		return Response{}, err
	}
	out := Decide(raw)
	return Response{Count: out.Count}, nil
}
