package util

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/platform/git"
)

func TestRoot(t *testing.T) {
	if Root != git.Root {
		t.Fatal(Root)
	}
}
