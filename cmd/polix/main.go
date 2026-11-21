// Command polix connects Claude Code to the workflow and the rules of a repository through its hooks; Codex
// support is not implemented. It only wires the root command and the commands of each use case; see
// docs/architecture.md for the layout and docs/polix.md for the design.
package main

import (
	"os"

	"github.com/spf13/cobra"

	diffmeasure "github.com/mlophez/polix-ai/internal/app/diff/features/measure"
	githookanswer "github.com/mlophez/polix-ai/internal/app/githook/features/answer"
	hookanswer "github.com/mlophez/polix-ai/internal/app/hook/features/answer"
	ruleadapters "github.com/mlophez/polix-ai/internal/app/rule/adapters"
	rulecheck "github.com/mlophez/polix-ai/internal/app/rule/features/check"
	ruleformat "github.com/mlophez/polix-ai/internal/app/rule/features/format"
	setupcheck "github.com/mlophez/polix-ai/internal/app/setup/features/check"
	setupdiagnose "github.com/mlophez/polix-ai/internal/app/setup/features/diagnose"
	setupinstall "github.com/mlophez/polix-ai/internal/app/setup/features/install"
	setuprefresh "github.com/mlophez/polix-ai/internal/app/setup/features/refresh"
	setupuninstall "github.com/mlophez/polix-ai/internal/app/setup/features/uninstall"
	stateadapters "github.com/mlophez/polix-ai/internal/app/state/adapters"
	versionadapters "github.com/mlophez/polix-ai/internal/app/version/adapters"
	versionshow "github.com/mlophez/polix-ai/internal/app/version/features/show"
	versionupdate "github.com/mlophez/polix-ai/internal/app/version/features/update"
	workflowadapters "github.com/mlophez/polix-ai/internal/app/workflow/adapters"
	workflowformat "github.com/mlophez/polix-ai/internal/app/workflow/features/format"
	workflowgraph "github.com/mlophez/polix-ai/internal/app/workflow/features/graph"
	workflowlog "github.com/mlophez/polix-ai/internal/app/workflow/features/log"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

func main() {
	root := cli.Root("polix",
		"A deterministic harness for AI agents: one polix.yaml decides what the agent may not touch, what must "+
			"pass and what runs at every step.",
		"polix applies the workflow (polix.yaml) and the rules (rules/ below path, ai/rules/ by default) of a repository to Claude Code "+
			"through its hooks: it runs, injects and blocks what the workflow says. Claude Code is the supported agent; "+
			"Codex support is not implemented.",
		"  # Set up this machine: the polix plugin with its skills and hooks\n"+
			"  polix install\n\n"+
			"  # Check the machine and the repository\n"+
			"  polix doctor\n\n"+
			"  # Validate the workflow and the rules\n"+
			"  polix config check\n\n"+
			"  # Format polix.yaml\n"+
			"  polix config fmt")
	root.AddGroup(&cobra.Group{ID: "repo", Title: "Repository"}, &cobra.Group{ID: "machine", Title: "Machine"},
		&cobra.Group{ID: "agent", Title: "Agents"})
	// The loaders of the files several use cases read, built once and passed to each of them; they read lazily.
	loadRules, loadCache, loadWorkflow := (&ruleadapters.Folder{}).Load, (&versionadapters.CacheFile{}).Load, (&workflowadapters.File{}).Load
	loadState := (&stateadapters.Log{}).Load
	config := &cobra.Command{Use: "config", Short: "Validate and format polix.yaml", Args: cobra.NoArgs, GroupID: "repo"}
	config.AddCommand(setupcheck.Command(loadRules, loadWorkflow), workflowformat.Command())
	root.AddCommand(config)
	for _, c := range []*cobra.Command{setupinstall.Command(), setupuninstall.Command(),
		setupdiagnose.Command(loadRules, loadCache, loadWorkflow)} {
		c.GroupID = "machine"
		root.AddCommand(c)
	}
	root.AddCommand(setuprefresh.Command())
	for _, c := range []*cobra.Command{workflowgraph.Command(loadWorkflow), workflowlog.Command(loadState),
		diffmeasure.Command(loadWorkflow)} {
		c.GroupID = "repo"
		root.AddCommand(c)
	}
	for _, c := range []*cobra.Command{versionupdate.Command(loadCache, loadWorkflow), versionshow.Command()} {
		c.GroupID = "machine"
		root.AddCommand(c)
	}

	answer := hookanswer.Command(loadRules, loadWorkflow, loadState)
	answer.GroupID = "agent"
	root.AddCommand(answer, githookanswer.Command(loadWorkflow))

	rules := &cobra.Command{Use: "rules", Short: "Work with the rules of rules/ below path (ai/rules/)", Args: cobra.NoArgs, GroupID: "repo"}
	rules.AddCommand(rulecheck.Command(loadRules, loadWorkflow), ruleformat.Command(loadRules, loadWorkflow))
	root.AddCommand(rules)

	os.Exit(cli.Run(root, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
