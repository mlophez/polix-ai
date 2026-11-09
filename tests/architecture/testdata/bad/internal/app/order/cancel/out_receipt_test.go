// A test of a use case imports another use case (GOARCH-9), and a load use case to build its loader as cmd/ does,
// which GOARCH-9 admits.
package cancel

import (
	"example.com/app/internal/app/order/place"
	"example.com/app/internal/app/stock/load"
)

var (
	_ = place.Command
	_ = load.Command
)
