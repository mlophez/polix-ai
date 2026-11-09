// Package load is the good case of a load use case: its entry returns the loader of its domain, stock.Load,
// instead of a cobra command (GOARCH-11, GOARCH-15), and its document names main in its Trigger (GOARCH-14).
package load

import (
	"context"

	"example.com/app/internal/app/stock"
)

type Request struct{ Shop string }

type Response struct{ Loaded stock.Loaded }

// Port is the port of the use case, which the adapter implements (GOARCH-5, GOARCH-12).
type Port interface {
	Read(ctx context.Context, shop string) (int, error)
}

// Command returns the loader, which runs the shell with the adapter as the port and calls neither (GOARCH-32).
func Command() stock.Load {
	return func(ctx context.Context, shop string) stock.Loaded {
		res, _ := run(ctx, Adapter{}, Request{Shop: shop})
		return res.Loaded
	}
}

// run reads through the port and calls the logic once (GOARCH-32).
func run(ctx context.Context, port Port, req Request) (Response, error) {
	count, err := port.Read(ctx, req.Shop)
	out := Decide(count, err)
	return Response{Loaded: out.Loaded}, nil
}
