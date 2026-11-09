package architecture

import "testing"

// GOARCH-42: internal/platform/ never imports internal/app/, as the list GOARCH-42 of depguard in .golangci.yml
// denies.
func TestGOARCH42(t *testing.T) {
	checkDepguard(t, "GOARCH-42",
		"internal/platform/util/util.go:4: import 'github.com/mlophez/polix-ai/internal/app/order' is not allowed "+
			"from list 'goarch-42': GOARCH-42:")
}
