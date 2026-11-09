package count

import (
	"context"
	"io"

	"example.com/app/internal/app/order"
	"example.com/app/internal/app/order/place"
	"example.com/app/internal/app/stock"
)

// Request has a field the document does not name (GOARCH-36).
type Request struct{ Shop string }

// Response and the Shelf it holds have fields the document does not name (GOARCH-36).
type Response struct {
	Count   int
	Shelves []Shelf
}

type Shelf struct{ Aisle int }

// Port takes and returns types that are not domain, returns the output of another use case (GOARCH-9 too),
// embeds an interface (GOARCH-33), and its document does not name Load (GOARCH-36).
type Port interface {
	io.Closer
	Load(ctx context.Context, from io.Reader) (chan int, error)
	Plan(ctx context.Context) (place.Output, error)
	Open(ctx context.Context, p stock.StockPort) error
}

// box embeds the port, so its methods are promoted.
type box struct{ Port }

// run calls the logic twice (GOARCH-32) and decides on the request with a literal and an ordering (GOARCH-37).
func run(ctx context.Context, port Port, req Request) (Response, error) {
	if len(req.Shop) > 3 {
		return Response{}, nil
	}
	if req.Shop == "main" {
		return Response{}, nil
	}
	var l order.Line
	if l.Rename("x").Valid() != nil { // a literal argument of a method of the domain (GOARCH-37)
		return Response{}, nil
	}
	return Response{Count: Decide(stock.StockPort{}) + Decide(stock.StockPort{})}, nil
}

// load uses the port out of the shell (GOARCH-32).
func load(ctx context.Context, port Port) { _, _ = port.Load(ctx, nil) }

// closeAll uses a method of the port promoted through an embedded field (GOARCH-32).
func closeAll(b box) { _ = b.Close() }

// pick decides with arithmetic, a constant of the domain and a type switch (GOARCH-37).
func pick(n int, v any) {
	if n%2 == 0 {
		return
	}
	if stock.Limit {
		return
	}
	switch v.(type) {
	}
}
