package stock

import (
	"context"
	"io"

	"example.com/app/internal/order"
)

// CountStockRequest has a field the document does not name (GOARCH-36).
type CountStockRequest struct{ Shop string }

// CountStockResponse and the CountStockShelf it holds have fields the document does not name (GOARCH-36).
type CountStockResponse struct {
	Count   int
	Shelves []CountStockShelf
}

type CountStockShelf struct{ Aisle int }

// CountStockPort takes and returns types that are not domain, returns the output of a use case of another
// package, embeds an interface (GOARCH-33), and its document does not name Load (GOARCH-36).
type CountStockPort interface {
	io.Closer
	Load(ctx context.Context, from io.Reader) (chan int, error)
	Plan(ctx context.Context) (order.PlaceOrderOutput, error)
	Open(ctx context.Context, p StockPort) error
}

// countStockBox embeds the port, so its methods are promoted.
type countStockBox struct{ CountStockPort }

// countStock calls the logic twice (GOARCH-32) and decides on the request with a literal and an ordering
// (GOARCH-37).
func countStock(ctx context.Context, port CountStockPort, req CountStockRequest) (CountStockResponse, error) {
	if len(req.Shop) > 3 {
		return CountStockResponse{}, nil
	}
	if req.Shop == "main" {
		return CountStockResponse{}, nil
	}
	var l order.Line
	if l.Rename("x").Valid() != nil { // a literal argument of a method of the domain (GOARCH-37)
		return CountStockResponse{}, nil
	}
	return CountStockResponse{Count: CountStock() + CountStock()}, nil
}

// countStockLoad uses the port out of the shell (GOARCH-32).
func countStockLoad(ctx context.Context, port CountStockPort) { _, _ = port.Load(ctx, nil) }

// countStockClose uses a method of the port promoted through an embedded field (GOARCH-32).
func countStockClose(b countStockBox) { _ = b.Close() }

// countStockPick decides with arithmetic, a constant and a type switch (GOARCH-37).
func countStockPick(n int, v any) {
	if n%2 == 0 {
		return
	}
	if stockLimit {
		return
	}
	switch v.(type) {
	}
}
