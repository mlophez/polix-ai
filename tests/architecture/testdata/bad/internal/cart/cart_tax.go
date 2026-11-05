// Package cart: a domain file that imports a library of the module that touches the outside (GOARCH-4).
package cart

import (
	// GOARCH-4: pkg/proc runs processes; the domain never imports it.
	"example.com/app/pkg/proc"
)

var _ = proc.Run
