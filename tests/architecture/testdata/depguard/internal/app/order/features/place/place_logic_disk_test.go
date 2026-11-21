package place

import (
	"os"
	"testing"
)

// TestHome imports os, which a test of a sibling of the logic may: no list of depguard reports a test file.
func TestHome(t *testing.T) { _, _ = os.Getwd(); _ = home() }
