// Command polix only wires the root command and the commands of each use case; it holds no logic.
package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/pkg/cli"
)

func main() {
	root := &cobra.Command{Use: "polix", SilenceUsage: true}

	rules := &cobra.Command{Use: "rules", Short: "Work with the rules of ai/rules/", Args: cobra.NoArgs}
	rules.AddCommand(rule.CheckRuleCommand())
	root.AddCommand(rules)

	os.Exit(cli.Run(root, os.Args[1:]))
}
