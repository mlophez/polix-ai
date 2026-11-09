// Package load is a load use case whose domain declares no loader nor its result (GOARCH-11), and whose entry
// returns a cobra command instead of the loader of its domain (GOARCH-15).
package load

import "github.com/spf13/cobra"

func Command() *cobra.Command { return nil }
