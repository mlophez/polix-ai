package architecture

import "testing"

// GOARCH-48: a library of internal/platform/ never imports another one, as the list GOARCH-48 of depguard in
// .golangci.yml denies.
func TestGOARCH48(t *testing.T) {
	checkDepguard(t, "GOARCH-48",
		"internal/platform/util/util_git.go:3: import 'github.com/mlophez/polix-ai/internal/platform/git' is not "+
			"allowed from list 'goarch-48': GOARCH-48:")
}
