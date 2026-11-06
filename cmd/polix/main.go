// Command polix connects Claude Code and Codex to the workflow and the rules of a repository through
// their hooks. It only wires the root command and the commands of each use case; see docs/architecture.md for
// the layout and docs/polix.md for the design.
package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/hook"
	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/setup"
	"github.com/mlophez/polix-ai/internal/version"
	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/cli"
)

func main() {
	root := cli.Root("polix",
		"Applies the polix.yaml and ai/rules/ of a repository to Claude Code and Codex",
		"polix applies the workflow (polix.yaml) and the rules (ai/rules/, or rules.path) of a repository to Claude Code and "+
			"Codex through their hooks: it blocks, requires, runs, injects and records what the workflow says.",
		"  # Set up this machine: the polix plugin with its skills and hooks\n"+
			"  polix install\n\n"+
			"  # Check the machine and the repository\n"+
			"  polix doctor\n\n"+
			"  # Validate the workflow and the rules\n"+
			"  polix check")
	root.AddGroup(&cobra.Group{ID: "repo", Title: "Repository"}, &cobra.Group{ID: "machine", Title: "Machine"},
		&cobra.Group{ID: "agent", Title: "Agents"})
	check := setup.CheckSetupCommand()
	check.GroupID = "repo"
	root.AddCommand(check)
	for _, c := range []*cobra.Command{setup.InstallSetupCommand(), setup.UninstallSetupCommand(),
		setup.DiagnoseSetupCommand()} {
		c.GroupID = "machine"
		root.AddCommand(c)
	}
	root.AddCommand(setup.RefreshSetupCommand())
	for _, c := range []*cobra.Command{workflow.GraphWorkflowCommand(), workflow.LogWorkflowCommand()} {
		c.GroupID = "repo"
		root.AddCommand(c)
	}
	for _, c := range []*cobra.Command{version.UpdateVersionCommand(), version.ShowVersionCommand()} {
		c.GroupID = "machine"
		root.AddCommand(c)
	}

	answer := hook.AnswerHookCommand()
	answer.GroupID = "agent"
	root.AddCommand(answer)

	rules := &cobra.Command{Use: "rules", Short: "Work with the rules of rules.path (ai/rules/)", Args: cobra.NoArgs, GroupID: "repo"}
	rules.AddCommand(rule.CheckRuleCommand())
	root.AddCommand(rules)

	os.Exit(cli.Run(root, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
