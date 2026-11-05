package stock

import (
	"context"
	"io"
)

// CountStockRequest is not named in the document (GOARCH-36).
type CountStockRequest struct{ Shop string }

type CountStockResponse struct{ Count int }

// CountStockPort takes and returns types that are not domain and embeds an interface (GOARCH-33), and its
// document does not name Load (GOARCH-36).
type CountStockPort interface {
	io.Closer
	Load(ctx context.Context, from io.Reader) (chan int, error)
}

// countStock calls the logic twice (GOARCH-32).
func countStock(ctx context.Context, port CountStockPort, req CountStockRequest) (CountStockResponse, error) {
	return CountStockResponse{Count: CountStock() + CountStock()}, nil
}

// countStockLoad uses the port out of the shell (GOARCH-32).
func countStockLoad(ctx context.Context, port CountStockPort) { _, _ = port.Load(ctx, nil) }
