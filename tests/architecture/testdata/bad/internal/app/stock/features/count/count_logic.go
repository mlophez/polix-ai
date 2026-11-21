package count

import "example.com/app/internal/app/stock"

// Decide takes stock.StockPort, named as a port (GOARCH-23), returns an int, not Output (GOARCH-23), and the file
// declares no Output (GOARCH-11).
func Decide(p stock.StockPort) int { return stock.Level }
