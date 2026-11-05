package stock

// CountStock is the logic of a use case, not domain; it takes StockPort, named as a port (GOARCH-23), returns an
// int, not CountStockOutput (GOARCH-23), and the file declares no CountStockOutput (GOARCH-11).
func CountStock(p StockPort) int { return Level }
