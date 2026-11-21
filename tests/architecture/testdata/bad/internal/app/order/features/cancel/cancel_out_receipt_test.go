// A test of a use case imports another use case (GOARCH-9), and an adapters package to read real files as cmd/ does,
// which GOARCH-9 admits.
package cancel

import (
	"example.com/app/internal/app/order/features/place"
	"example.com/app/internal/app/stock/adapters"
)

var (
	_ = place.Command
	_ = io.Log{}
)
