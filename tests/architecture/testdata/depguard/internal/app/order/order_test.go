package order

import (
	"os"
	"testing"

	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// TestHome imports os and testenv, which a test may: no list of depguard reports a test file.
func TestHome(t *testing.T) { _, _ = os.Getwd(); _ = Home() + testenv.Dir }
