// Command order: the same imports with their alias <noun><verb> and <noun>adapters, which importas accepts.
package main

import (
	orderadapters "github.com/mlophez/polix-ai/internal/app/order/adapters"
	orderplace "github.com/mlophez/polix-ai/internal/app/order/features/place"
)

var _ = orderplace.Decide

var _ = orderadapters.File{}
