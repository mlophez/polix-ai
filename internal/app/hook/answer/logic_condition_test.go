package answer

import "testing"

// branch holds on a match of its expression, never on one that does not compile.
func TestBranch(t *testing.T) {
	if !branchMatches("^main$", "main") || branchMatches("^main$", "dev") || branchMatches("(", "(") {
		t.Fatal("branchMatches")
	}
}
