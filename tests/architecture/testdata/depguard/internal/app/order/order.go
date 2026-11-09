// Package order is a domain that imports os, which GOARCH-41-domain denies, and the library git, which it does
// not allow (GOARCH-41).
package order

import (
	"os"

	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Order is the entity of the domain.
type Order struct{ ID string }

// Home reads the environment and the repository.
func Home() string { return os.Getenv("HOME") + git.Root }
