// Package stock: the use case drop_stock has no drop_stock_in.go (GOARCH-3).
package stock

type DropStockOutput struct{}

func DropStock() DropStockOutput { return DropStockOutput{} }
