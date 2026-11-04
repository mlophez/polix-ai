// Package cli implements the commands of polix:
//
//	polix hook <claude|codex>    entry point of the hooks: event on stdin, answer on exit code and output
//	polix check                  validate polix.yaml and ai/rules/
//	polix rules check [--changed] [--base B] run the check of the deterministic rules, for CI
//	polix graph                  print the workflow as a Mermaid diagram
//	polix log [--tail N] [--json] print the last entries of the state log
//	polix doctor [--hook]        check that this machine and repository are ready for polix
//	polix install [--user]       install the skills for the user and register the hooks in the repository
//	polix uninstall [--user]     remove what install wrote
//	polix update [--check]       replace the binary with the latest release and refresh the skills
//	polix version
//
// The commands are built with cobra and fang, which give them styled help, errors and shell completions. The
// output of `polix hook` is not styled: the agents read it.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/agent"
	"github.com/mlophez/polix-ai/internal/engine"
	"github.com/mlophez/polix-ai/internal/gitx"
	"github.com/mlophez/polix-ai/internal/rules"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// exitCode is the error of a command that already told the user why it failed: it only carries the exit code,
// and fang prints nothing for it.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// exit turns the exit code of a command into its error.
func exit(code int) error {
	if code == 0 {
		return nil
	}
	return exitCode(code)
}

// Run executes polix with args (without the program name) and returns the exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if stdin == nil {
		stdin = os.Stdin
	}
	root := newRoot()
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := fang.Execute(context.Background(), root,
		fang.WithVersion(Version),
		fang.WithoutManpage(),
		fang.WithErrorHandler(printError),
	)
	if err == nil {
		return 0
	}
	if code, ok := errors.AsType[exitCode](err); ok {
		return int(code)
	}
	return 1
}

// printError prints the error of a command in one line, as the old CLI did, so scripts and agents can read it;
// an exitCode was already explained by the command.
func printError(w io.Writer, _ fang.Styles, err error) {
	if _, ok := errors.AsType[exitCode](err); ok {
		return
	}
	fmt.Fprintf(w, "%s %s\n", styleFail.Render("polix:"), err)
	if msg := err.Error(); strings.HasPrefix(msg, "unknown ") || strings.Contains(msg, "flag") || strings.Contains(msg, "arg(s)") {
		fmt.Fprintf(w, "%s\n", styleMuted.Render("Run polix --help for usage."))
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "polix",
		Short: "Applies the polix.yaml and ai/rules/ of a repository to Claude Code and Codex",
		Long: "polix applies the workflow (polix.yaml) and the rules (ai/rules/) of a repository to Claude Code and " +
			"Codex through their hooks: it blocks, requires, runs, injects and records what the workflow says.",
		Example: "  # Set up this machine: the polix plugin with its skills and hooks\n" +
			"  polix install\n\n" +
			"  # Check the machine and the repository\n" +
			"  polix doctor\n\n" +
			"  # Validate the workflow and the rules\n" +
			"  polix check",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddGroup(
		&cobra.Group{ID: "repo", Title: "Repository"},
		&cobra.Group{ID: "machine", Title: "Machine"},
		&cobra.Group{ID: "agent", Title: "Agents"},
	)
	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add("repo", checkCmd(), rulesCmd(), graphCmd(), logCmd())
	add("machine", installCmd(), uninstallCmd(), updateCmd(), doctorCmd(), versionCmd())
	root.AddCommand(refreshCmd())
	add("agent", hookCmd())
	return root
}

// dirFlag adds --dir, the repository a command works on.
func dirFlag(c *cobra.Command, dir *string, def string) {
	c.Flags().StringVarP(dir, "dir", "C", def, "repository to work on")
	_ = c.MarkFlagDirname("dir")
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of polix",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			fmt.Fprintln(c.OutOrStdout(), Version)
			return nil
		},
	}
}

func hookCmd() *cobra.Command {
	var dir string
	c := &cobra.Command{
		Use:   "hook <claude|codex>",
		Short: "Answer one hook call of an agent (event on stdin)",
		Long: "Entry point of the hooks of Claude Code and Codex: it reads the event on stdin and answers with the " +
			"exit code (2 blocks) and the output the agent expects. The polix plugin registers it; people do not " +
			"run it.",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"claude", "codex"},
		RunE: func(c *cobra.Command, args []string) error {
			return exit(hook(args[0], dir, c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr()))
		},
	}
	c.Flags().StringVarP(&dir, "dir", "C", "", "repository (default: cwd of the hook input, else the working directory)")
	return c
}

// hook answers one hook call. Failure policy: outside a git repository, or without polix.yaml, it lets
// everything through except the built-in protections. An invalid workflow fails closed on edits and commands
// (see engine.Engine.Invalid) and never blocks a stop, which would keep the agent in a loop it cannot leave.
func hook(name, dir string, stdin io.Reader, stdout, stderr io.Writer) int {
	ad, err := agent.For(name)
	if err != nil {
		fmt.Fprintln(stderr, "polix hook:", err)
		return 1
	}
	input, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "polix hook:", err)
		return agent.ExitOK
	}
	var repo *gitx.Repo
	resolver := func(p, cwd string) (string, bool) {
		if repo == nil {
			return "", false
		}
		return repo.Rel(p, cwd)
	}
	native, events, err := ad.Parse(input, resolver)
	if err != nil {
		fmt.Fprintln(stderr, "polix hook:", err)
		return agent.ExitOK
	}
	if len(events) == 0 {
		return agent.ExitOK
	}
	start := dir
	if start == "" {
		start = events[0].Cwd
	}
	if start == "" {
		start, _ = os.Getwd()
	}
	repo, err = gitx.Open(start)
	if err != nil {
		return agent.ExitOK // outside a repository there is nothing to govern
	}
	// Paths were resolved before the repository was known: parse again now that it is.
	native, events, _ = ad.Parse(input, resolver)

	wf, err := workflow.Load(repo.Root)
	if err == nil && wf != nil {
		if problems := workflow.Validate(wf); len(problems) > 0 {
			err = fmt.Errorf("%s", strings.Join(problems, "\n"))
		}
	}
	en := engine.New(repo, wf)
	if err != nil {
		en.Workflow, en.Invalid = nil, err
	}
	var res engine.Result
	for _, ev := range events {
		res.Merge(en.Handle(ev))
	}
	return ad.Render(native, res, stdout, stderr)
}

func checkCmd() *cobra.Command {
	var dir string
	c := &cobra.Command{
		Use:   "check",
		Short: "Validate polix.yaml and ai/rules/",
		Long: "Validate the workflow and the rules and print every problem: unknown events, conditions, actions and " +
			"keys, undefined variables and conditions on events where they never hold.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := gitx.Open(dir)
			if err != nil {
				return err
			}
			return exit(check(repo, c.OutOrStdout(), c.ErrOrStderr()))
		},
	}
	dirFlag(c, &dir, ".")
	return c
}

// check validates the workflow and the rules and prints every problem.
func check(repo *gitx.Repo, stdout, stderr io.Writer) int {
	var problems []string
	wf, err := workflow.Load(repo.Root)
	switch {
	case err != nil:
		problems = append(problems, err.Error())
	case wf != nil:
		problems = append(problems, workflow.Validate(wf)...)
	}
	all, err := rules.Load(repo.Root)
	if err != nil {
		problems = append(problems, err.Error())
	} else {
		problems = append(problems, rules.Validate(repo.Root, all)...)
	}
	if len(problems) > 0 {
		u := newUI(stderr)
		for _, p := range problems {
			u.fail("%s", p)
		}
		return 1
	}
	u := newUI(stdout)
	if wf == nil {
		u.line("%s no %s (only the built-in protections apply) and %d rules of %s/", styleOK.Render("ok:"), workflow.File, len(all), rules.Dir)
		return 0
	}
	u.line("%s %s and %d rules of %s/", styleOK.Render("ok:"), workflow.File, len(all), rules.Dir)
	return 0
}

func rulesCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "rules",
		Short: "Work with the rules of ai/rules/",
		Args:  cobra.NoArgs,
	}
	var dir, base string
	var changed bool
	chk := &cobra.Command{
		Use:   "check",
		Short: "Run the checks of the deterministic rules",
		Long: "Run the checks of the deterministic rules: all of them, or with --changed only those whose scope " +
			"matches the files changed by the branch. Meant for CI.",
		Example: "  polix rules check\n  polix rules check --changed --base develop",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := gitx.Open(dir)
			if err != nil {
				return err
			}
			return exit(rulesCheck(repo, changed, base, c.OutOrStdout(), c.ErrOrStderr()))
		},
	}
	dirFlag(chk, &dir, ".")
	chk.Flags().BoolVar(&changed, "changed", false, "only the rules whose scope matches the changed files")
	chk.Flags().StringVar(&base, "base", "", "branch the change merges into, for --changed (default: base of "+
		workflow.File+", else "+workflow.DefaultBase+")")
	c.AddCommand(chk)
	return c
}

// rulesCheck runs the checks of the deterministic rules: all of them, or with changed only those whose scope
// matches the files changed by the branch. The base of changed is base, else the base of polix.yaml.
func rulesCheck(repo *gitx.Repo, changed bool, base string, stdout, stderr io.Writer) int {
	all, err := rules.Load(repo.Root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	list := all
	if changed {
		if base == "" {
			wf, _ := workflow.Load(repo.Root)
			base = wf.BaseBranch()
		}
		files, err := repo.Changed(base)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		list = rules.Matching(all, files)
	}
	failures := rules.RunChecks(context.Background(), repo.Root, list)
	ran := 0
	for _, r := range list {
		if r.Deterministic && r.Check != "" {
			ran++
		}
	}
	ue := newUI(stderr)
	for _, f := range failures {
		ue.fail("FAIL %s (%s): %s", f.Rule.ID, f.Rule.Check, f.Rule.Description)
		fmt.Fprintln(stderr, f.Output)
	}
	u := newUI(stdout)
	if len(failures) > 0 {
		u.fail("%d checks, %d failed", ran, len(failures))
		return 1
	}
	u.ok("%d checks, %d failed", ran, len(failures))
	return 0
}

func graphCmd() *cobra.Command {
	var dir string
	c := &cobra.Command{
		Use:   "graph",
		Short: "Print the workflow as a Mermaid diagram",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := gitx.Open(dir)
			if err != nil {
				return err
			}
			return graph(repo, c.OutOrStdout())
		},
	}
	dirFlag(c, &dir, ".")
	return c
}

// graph prints the workflow as a Mermaid flowchart: one subgraph per event, one node per entry.
func graph(repo *gitx.Repo, stdout io.Writer) error {
	wf, err := workflow.Load(repo.Root)
	if err != nil {
		return err
	}
	if wf == nil {
		return fmt.Errorf("%s not found", workflow.File)
	}
	fmt.Fprintln(stdout, "flowchart TD")
	n := 0
	for _, ev := range workflow.Events {
		entries := wf.On[ev]
		if len(entries) == 0 {
			continue
		}
		fmt.Fprintf(stdout, "  subgraph %s\n", ev)
		prev := ""
		for _, e := range entries {
			id := fmt.Sprintf("n%d", n)
			n++
			fmt.Fprintf(stdout, "    %s[\"%s\"]\n", id, mermaidLabel(&e))
			if prev != "" {
				fmt.Fprintf(stdout, "    %s --> %s\n", prev, id)
			}
			prev = id
		}
		fmt.Fprintln(stdout, "  end")
	}
	return nil
}

func logCmd() *cobra.Command {
	var dir string
	var n int
	var raw bool
	c := &cobra.Command{
		Use:   "log",
		Short: "Print the last entries of the state log",
		Long: "Print the last entries of the state log: the sanctioned way to read it, since commands that name the " +
			"state folder and write files are denied.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := gitx.Open(dir)
			if err != nil {
				return err
			}
			return printLog(repo, n, raw, c.OutOrStdout())
		},
	}
	dirFlag(c, &dir, ".")
	c.Flags().IntVarP(&n, "tail", "n", 20, "number of entries, 0 for all")
	c.Flags().BoolVar(&raw, "json", false, "print the entries as JSON lines")
	return c
}

func printLog(repo *gitx.Repo, n int, raw bool, stdout io.Writer) error {
	entries, err := state.Open(repo.GitDir).Entries()
	if err != nil {
		return err
	}
	if n > 0 && len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	for _, e := range entries {
		if raw {
			line, _ := json.Marshal(e)
			fmt.Fprintln(stdout, string(line))
			continue
		}
		what := e.Fact
		if what == "" {
			what = strings.TrimSpace(e.Outcome + " " + e.Action + e.Rule)
		}
		reason, _, _ := strings.Cut(e.Reason, "\n")
		fmt.Fprintf(stdout, "%s  %-10s %-13s %-9s %-16s %.8s  %s\n", e.At, e.Kind, e.Event, e.Actor, what, e.Hash, reason)
	}
	return nil
}

// mermaidLabel describes an entry: its conditions and its action.
func mermaidLabel(e *workflow.Entry) string {
	var parts []string
	if c := condText(e.If); c != "" {
		parts = append(parts, "if "+c)
	}
	if c := condText(e.Unless); c != "" {
		parts = append(parts, "unless "+c)
	}
	act := e.Action()
	switch act {
	case "deny":
		act += ": " + *e.Deny
	case "require":
		act += ": " + *e.Require
	case "record":
		act += ": " + *e.Record
	case "run":
		if e.Run.Rules != "" {
			act += " rules " + e.Run.Rules
		} else {
			act += " " + strings.Join(e.Run.Command, " ")
		}
	}
	if m := e.Mode; m != "" && m != workflow.ModeEnforce {
		act += " (" + m + ")"
	}
	parts = append(parts, act)
	return strings.ReplaceAll(strings.Join(parts, "<br/>"), `"`, "'")
}

func condText(c workflow.Cond) string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%v", k, c[k]))
	}
	return strings.Join(out, ", ")
}
