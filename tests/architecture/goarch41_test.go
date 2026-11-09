package architecture

import "testing"

// GOARCH-41: the domain and the logic import only what the lists GOARCH-41-domain and GOARCH-41-logic of depguard
// in .golangci.yml allow: the standard library but what touches the outside, the randomness and the logs (and
// context for the logic), internal/app/, the pure libraries of internal/platform/, yaml and the templates.
func TestGOARCH41(t *testing.T) {
	checkDepguard(t, "GOARCH-41",
		"internal/app/order/order.go:6: import 'os' is not allowed from list 'goarch-41-domain': GOARCH-41:",
		"internal/app/order/order.go:8: import 'github.com/mlophez/polix-ai/internal/platform/git' is not allowed "+
			"from list 'goarch-41-domain'",
		"internal/app/order/place/logic.go:4: import 'context' is not allowed from list 'goarch-41-logic': GOARCH-41:")
}
