// A test of the domain imports a load use case (GOARCH-9): only the tests of a use case may.
package stock

import "example.com/app/internal/app/stock/load"

var _ = load.Command
