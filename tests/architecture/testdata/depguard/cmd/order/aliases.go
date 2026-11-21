// Command order: imports of a use case and of an adapters package without their alias, which importas reports.
package main

import (
	"github.com/mlophez/polix-ai/internal/app/order/adapters"
	"github.com/mlophez/polix-ai/internal/app/order/features/place"
)

var _ = place.Decide

var _ = adapters.File{}
