// Package cart: a domain file that imports the library of the module that reads and writes files (GOARCH-4).
package cart

import (
	// GOARCH-4: pkg/file touches the disk; the domain never imports it.
	"example.com/app/pkg/file"
)

var _ = file.Read
