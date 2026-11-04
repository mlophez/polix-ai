// Command polix connects Claude Code and Codex to the workflow and the rules of a repository through
// their hooks. See internal/cli for the commands and docs/polix.md for the design.
package main

import (
	"os"

	"github.com/mlophez/polix-ai/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
