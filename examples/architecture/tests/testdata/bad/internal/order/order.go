package order

import (
	"os"

	"example.com/app/internal/stock"
)

// Total uses a name of a use case and of a use case of stock (GOARCH-9) and comes before a type (GOARCH-13).
func Total() int { _ = os.Args; return placeOrderLimit + stock.Level + stock.CountStock() }

// Item is not the entity Order (GOARCH-2).
type Item struct{ ID string }
