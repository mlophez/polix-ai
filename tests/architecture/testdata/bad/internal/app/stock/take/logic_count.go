package take

import "strings"

// count is a pure, unexported function of a sibling of logic.go, which exports nothing (GOARCH-4, GOARCH-23).
func count(raw string) int { return len(strings.Fields(raw)) }
