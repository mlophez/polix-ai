package architecture

import "testing"

// GOARCH-43: only the _test.go files import internal/platform/testenv, as the list GOARCH-43 of depguard in
// .golangci.yml denies to the rest.
func TestGOARCH43(t *testing.T) {
	checkDepguard(t, "GOARCH-43",
		"cmd/order/main.go:4: import 'github.com/mlophez/polix-ai/internal/platform/testenv' is not allowed "+
			"from list 'goarch-43': GOARCH-43:")
}
