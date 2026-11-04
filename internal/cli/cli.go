// Package cli implements the commands of polix:
//
//	polix hook <claude|codex>    entry point of the hooks: event on stdin, answer on exit code and output
//	polix check                  validate polix.yaml and ai/rules/
//	polix rules check [--changed] [--base B] run the check of the deterministic rules, for CI
//	polix graph                  print the workflow as a Mermaid diagram
//	polix log [--tail N] [--json] print the last entries of the state log
//	polix doctor [--hook]        check that this machine and repository are ready for polix
//	polix update [--check]       replace the binary with the latest release
//	polix version
//
// Every command takes --dir (default the working directory) to point at the repository.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mlophez/polix-ai/internal/agent"
	"github.com/mlophez/polix-ai/internal/engine"
	"github.com/mlophez/polix-ai/internal/gitx"
	"github.com/mlophez/polix-ai/internal/rules"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/workflow"
)

const usage = `polix: applies the polix.yaml and ai/rules/ of a repository to Claude Code and Codex.

Usage:
  polix hook <claude|codex> [--dir DIR]   entry point of the hooks (event on stdin)
  polix check [--dir DIR]                  validate polix.yaml and ai/rules/
  polix rules check [--changed] [--base BRANCH] [--dir DIR]
                                         run the checks of the deterministic rules
  polix graph [--dir DIR]                  print the workflow as a Mermaid diagram
  polix log [--tail N] [--json] [--dir DIR] print the last entries of the state log
  polix doctor [--hook] [--offline] [--dir DIR]
                                         check that this machine and repository are ready for polix
  polix update [--check] [--version V] [--hook]
                                         replace the binary with the latest release (or version V)
  polix version
`

// Run executes polix with args (without the program name) and returns the exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}
	switch args[0] {
	case "hook":
		return hook(args[1:], stdin, stdout, stderr)
	case "check":
		return check(args[1:], stdout, stderr)
	case "rules":
		if len(args) < 2 || args[1] != "check" {
			fmt.Fprint(stderr, usage)
			return 1
		}
		return rulesCheck(args[2:], stdout, stderr)
	case "graph":
		return graph(args[1:], stdout, stderr)
	case "log":
		return logCmd(args[1:], stdout, stderr)
	case "doctor":
		return doctor(args[1:], stdout, stderr)
	case "update":
		return updateCmd(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, Version)
		return 0
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "polix: unknown command %q\n\n%s", args[0], usage)
	return 1
}

// hook answers one hook call. Failure policy: outside a git repository, or without polix.yaml, it lets
// everything through except the built-in protections. An invalid workflow fails closed on edits and commands
// (see engine.Engine.Invalid) and never blocks a stop, which would keep the agent in a loop it cannot leave.
func hook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "polix hook: missing agent (claude, codex)")
		return 1
	}
	ad, err := agent.For(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "polix hook:", err)
		return 1
	}
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "", "repository (default: cwd of the hook input, else the working directory)")
	if err := fs.Parse(args[1:]); err != nil {
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
	start := *dir
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

// check validates the workflow and the rules and prints every problem.
func check(args []string, stdout, stderr io.Writer) int {
	repo, code := openRepo("check", args, nil, stderr)
	if repo == nil {
		return code
	}
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
		for _, p := range problems {
			fmt.Fprintln(stderr, p)
		}
		return 1
	}
	if wf == nil {
		fmt.Fprintf(stdout, "ok: no %s (only the built-in protections apply) and %d rules of %s/\n", workflow.File, len(all), rules.Dir)
		return 0
	}
	fmt.Fprintf(stdout, "ok: %s and %d rules of %s/\n", workflow.File, len(all), rules.Dir)
	return 0
}

// rulesCheck runs the checks of the deterministic rules: all of them, or with --changed only those whose
// scope matches the files changed by the branch. The base of --changed is --base, else the base of polix.yaml.
func rulesCheck(args []string, stdout, stderr io.Writer) int {
	var changed bool
	var base string
	repo, code := openRepo("rules check", args, func(fs *flag.FlagSet) {
		fs.BoolVar(&changed, "changed", false, "only the rules whose scope matches the changed files")
		fs.StringVar(&base, "base", "", "branch the change merges into, for --changed (default: base of "+workflow.File+", else "+workflow.DefaultBase+")")
	}, stderr)
	if repo == nil {
		return code
	}
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
	for _, f := range failures {
		fmt.Fprintf(stderr, "FAIL %s (%s): %s\n%s\n", f.Rule.ID, f.Rule.Check, f.Rule.Description, f.Output)
	}
	fmt.Fprintf(stdout, "%d checks, %d failed\n", ran, len(failures))
	if len(failures) > 0 {
		return 1
	}
	return 0
}

// graph prints the workflow as a Mermaid flowchart: one subgraph per event, one node per entry.
func graph(args []string, stdout, stderr io.Writer) int {
	repo, code := openRepo("graph", args, nil, stderr)
	if repo == nil {
		return code
	}
	wf, err := workflow.Load(repo.Root)
	if err != nil {
		fmt.Fprintln(stderr, "polix graph:", err)
		return 1
	}
	if wf == nil {
		fmt.Fprintf(stderr, "polix graph: %s not found\n", workflow.File)
		return 1
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
	return 0
}

// logCmd prints the last entries of the state log: the sanctioned way to read it, since commands that name the
// state folder and write files are denied.
func logCmd(args []string, stdout, stderr io.Writer) int {
	var n int
	var raw bool
	repo, code := openRepo("log", args, func(fs *flag.FlagSet) {
		fs.IntVar(&n, "tail", 20, "number of entries, 0 for all")
		fs.BoolVar(&raw, "json", false, "print the entries as JSON lines")
	}, stderr)
	if repo == nil {
		return code
	}
	entries, err := state.Open(repo.GitDir).Entries()
	if err != nil {
		fmt.Fprintln(stderr, "polix log:", err)
		return 1
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
	return 0
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

// openRepo parses --dir (plus extra flags) and opens the repository.
func openRepo(name string, args []string, extra func(*flag.FlagSet), stderr io.Writer) (*gitx.Repo, int) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", ".", "repository")
	if extra != nil {
		extra(fs)
	}
	if err := fs.Parse(args); err != nil {
		return nil, 1
	}
	repo, err := gitx.Open(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "polix "+name+":", err)
		return nil, 1
	}
	return repo, 0
}
