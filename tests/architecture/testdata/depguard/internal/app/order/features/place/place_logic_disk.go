// Package place: a sibling of the logic that imports os, which GOARCH-41-logic denies (GOARCH-41).
package place

import "os"

// home reads the environment, which the logic never does.
func home() string { return os.Getenv("HOME") }
