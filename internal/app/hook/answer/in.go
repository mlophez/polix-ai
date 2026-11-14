// Package answer is the use case AnswerHook, the command `polix hook <claude|codex>`: it answers one hook call of
// Claude Code or Codex, evaluating the protections and the workflow of polix.yaml for each of its events.
package answer

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

// Request is what the caller asks of the use case AnswerHook, whatever the transport: the command
// fills it from its argument, its flag and its stdin.
type Request struct {
	Agent   string // the agent behind the call: claude or codex
	Dir     string // the repository; empty takes the cwd of the call, else the working directory
	Input   []byte // the input of the hook, as the agent wrote it on stdin
	Version string // version of the running polix, which polix.yaml may require
	System  string // the system polix runs on (GOOS)
}

// Response is what the use case AnswerHook gives back to the caller, in the shape the agent reads:
// the command writes it as it is.
type Response struct {
	Stdout string
	Stderr string
	Code   int // 0 lets the action go on, 2 blocks it
}

// Port is what the shell of AnswerHook needs from the outside: the repository, its workflow, its
// facts, the processes and the log. It speaks in domain values, and every method takes the context of the
// caller.
//
//nolint:interfacebloat // the hook reads every fact through this one port, and the step shape needs one method per fact
type Port interface {
	// Open opens the repository that contains dir and returns its root and its git directory.
	Open(ctx context.Context, dir string) (root, gitDir string, err error)
	// LoadWorkflow reads polix.yaml for the polix of version running: whether it exists, and the error of a file
	// that cannot be read, parsed or used, which is data of the answer.
	LoadWorkflow(ctx context.Context, running string) (wf workflow.Workflow, present bool, err error)
	// Locate turns a path given by the agent from cwd into one relative to the root; inside is false outside it.
	Locate(ctx context.Context, path, cwd string) (rel string, inside bool)
	// Head returns the current branch, "" on a detached HEAD, and the commit HEAD points to, "" when it points to
	// none, as on a repository without commits; both are "" when git fails.
	Head(ctx context.Context) (branch, commit string)
	// Hash returns the hash of the current content of the working tree.
	Hash(ctx context.Context) (string, error)
	// Refs returns what git moves without moving HEAD: the stash and an operation stopped halfway.
	Refs(ctx context.Context) string
	// TreeDiff returns the paths that differ between two content hashes and those of them the second no longer
	// holds, or why they cannot be compared.
	TreeDiff(ctx context.Context, from, to string) (paths, removed []string, err error)
	// Changed returns the files not committed yet: changed since HEAD, or untracked.
	Changed(ctx context.Context) []string
	// Diff returns the size of the change of each file not committed yet, the same files as Changed, or why it
	// could not be measured.
	Diff(ctx context.Context) (diff.Diff, error)
	// Commit returns the commit HEAD points to as post_commit sees it: the size of its change against its first
	// parent, or the empty tree for a root commit, its short hash and its subject, or why it could not be read.
	Commit(ctx context.Context) (hook.Commit, error)
	// Files returns the files of the working tree that git does not ignore.
	Files(ctx context.Context) []string
	// LoadRules returns the rules of the folder dir, relative to the root, retired ones excluded, ordered by id.
	LoadRules(ctx context.Context, dir string) ([]rule.Rule, error)
	// LoadState reads the log of polix.
	LoadState(ctx context.Context) (state.State, error)
	// Ignored reports whether git ignores the path, relative to the root.
	Ignored(ctx context.Context, path string) bool
	// Real returns the real path of each of paths, symlinks resolved, in the same order.
	Real(ctx context.Context, paths []string) []string
	// Foreign reports whether the absolute folder dir is inside an existing git repository other than this one, a
	// linked worktree of this one excluded.
	Foreign(ctx context.Context, dir string) bool
	// Missing returns the programs, of programs, that the PATH does not run, in the same order.
	Missing(ctx context.Context, programs []string) []string
	// Contains reports whether a file of the working tree git does not ignore, matching one of the patterns, holds
	// text.
	Contains(ctx context.Context, patterns []string, text string) bool
	// Read returns the content of a file, relative to the root with slashes.
	Read(ctx context.Context, path string) (string, error)
	// Run runs a command from the root within the budget of the call and returns its output.
	Run(ctx context.Context, argv []string) (string, error)
	// RunCheck runs one invocation of the checks of one or several rules from the root within the budget of the
	// call and returns its output.
	RunCheck(ctx context.Context, run rule.Run) (string, error)
	// Restore puts the paths, relative to the root with slashes, back in the working tree as the tree source holds
	// them, removing those it does not hold, without touching the index; the error names the first path that failed.
	Restore(ctx context.Context, source string, paths []string) error
	// Append appends an entry to the log, stamped with the time.
	Append(ctx context.Context, e state.Entry) error
}

// format is the format of the hooks of one agent, the transport the entry reads and writes: claude in
// in_claude.go and codex in in_codex.go. Every format is pure, bytes in and bytes out.
type format interface {
	// parse turns the input of a hook call into the native name of its event, needed to answer, and the events
	// polix evaluates: none when the call is not relevant, such as a Read. The paths of the edits are not located
	// in the repository yet: agent.Event.Locate does it.
	parse(input []byte) (native string, events []agent.Event, err error)
	// render turns an answer into what the agent expects for its native event: the text of stdout and stderr and
	// the exit code.
	render(native string, ans agent.Answer) (stdout, stderr string, code int)
}

// formats are the formats of the agents, by the name of the agent.
var formats = map[string]format{agent.Claude: claude{}, agent.Codex: codex{}}

// Command is the command `polix hook <claude|codex>`, the entry of the use case AnswerHook: it turns
// the argument, the flag and stdin into a Request, injects the adapter with the loaders rules, loadWorkflow and
// loadState and the lookup exec.LookPath as the port, runs the shell and writes the Response as the agent expects
// it. Its failure policy: an unknown agent exits with 1; an input that cannot be read or parsed lets the action go
// on, with the error on stderr.
func Command(rules rule.Load, loadWorkflow workflow.Load, loadState state.Load) *cobra.Command {
	req := Request{Version: build.Current(), System: runtime.GOOS}
	c := &cobra.Command{
		Use:   "hook <claude|codex>",
		Short: "Answer one hook call of an agent (event on stdin)",
		Long: "Entry point of the hooks of Claude Code: it reads the event on stdin and answers with the exit code " +
			"(2 blocks) and the output the agent expects. The polix plugin registers it; people do not run it. " +
			"The agent codex is accepted, but Codex support is not implemented and untested.",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{agent.Claude, agent.Codex},
		RunE: func(c *cobra.Command, args []string) error {
			stdout, stderr := c.OutOrStdout(), c.ErrOrStderr()
			ag := agent.Agent{Name: args[0]}
			if err := ag.Valid(); err != nil {
				fmt.Fprintln(stderr, "polix hook:", err)
				return cli.Exit(1)
			}
			req.Agent = ag.Name
			input, err := io.ReadAll(c.InOrStdin())
			if err != nil {
				fmt.Fprintln(stderr, "polix hook:", err)
				return nil
			}
			req.Input = input
			res, err := run(c.Context(), &Adapter{Rules: rules, Workflow: loadWorkflow, State: loadState,
				Look: exec.LookPath}, req)
			if err != nil {
				fmt.Fprintln(stderr, "polix hook:", err)
				return nil
			}
			fmt.Fprint(stdout, res.Stdout)
			fmt.Fprint(stderr, res.Stderr)
			return cli.Exit(res.Code)
		},
	}
	c.Flags().StringVarP(&req.Dir, "dir", "C", "", "repository (default: cwd of the hook input, else the working directory)")
	return c
}

// run is the shell, from request to response: it parses the input with the format of the agent, opens the
// repository and loads the workflow through the port, and evaluates the events from a queue, those of the hook
// first: for each it locates its path and loops: it asks the logic for the next step, carries it out through the
// port and feeds the result back to the evaluation, until there is none; the events the output follows with, the
// edits a command made, join the queue. Then it merges the answers of the events and renders them with the format
// of the agent. Outside a repository, or without events, it lets everything through. It only sequences; the
// decisions are in the logic and the domain.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	ag := agent.Agent{Name: req.Agent}
	if err := ag.Valid(); err != nil {
		return Response{}, err
	}
	f := formats[ag.Name]
	native, events, err := f.parse(req.Input)
	if err != nil {
		return Response{}, err
	}
	h := hook.Hook{Agent: ag, Native: native, Events: events}
	if err := h.Valid(); err != nil {
		return Response{}, err
	}
	if len(h.Events) == 0 {
		return Response{}, nil
	}
	root, gitDir, err := port.Open(ctx, h.Folder(req.Dir))
	if err != nil {
		return Response{}, nil //nolint:nilerr // outside a repository there is nothing to govern
	}
	wf, present, problem := port.LoadWorkflow(ctx, req.Version)
	var facts hook.Facts
	var answer agent.Answer
	for queue := h.Events; len(queue) != 0; {
		e := queue[0]
		queue = queue[1:]
		if e.Edits() {
			e = e.Locate(port.Locate(ctx, e.AbsPath, e.Cwd))
		}
		ev := hook.Evaluation{Workflow: wf, Present: present, Invalid: problem, Root: root, GitDir: gitDir,
			System: req.System, Event: e, Remaining: len(queue), Facts: facts}
		if err := ev.Valid(); err != nil {
			return Response{}, err
		}
		ev = ev.Running(req.Version)
		var out Output
		for {
			out = Decide(ev)
			if out.Step.None() {
				break
			}
			ev = carry(ctx, port, ev, out.Step)
		}
		answer = answer.Merge(out.Answer)
		facts = out.Facts
		queue = append(queue, out.Follow...)
	}
	stdout, stderr, code := f.render(h.Native, answer)
	return Response{Stdout: stdout, Stderr: stderr, Code: code}, nil
}

// carry carries out the step s of the evaluation ev through the port and feeds its result back to the
// evaluation: the fact it asks for, of git here and of a path or a program through lookup, or, for a step that acts,
// a restore of files among them, its outcome (act).
func carry(ctx context.Context, port Port, ev hook.Evaluation, s hook.Step) hook.Evaluation {
	switch {
	case s.Branch:
		return ev.KnowBranch(port.Head(ctx))
	case s.Refs:
		return ev.KnowRefs(port.Refs(ctx))
	case s.Hash:
		hash, err := port.Hash(ctx)
		return ev.KnowHash(s.Epoch, hash, err)
	case s.Compare != "":
		paths, removed, err := port.TreeDiff(ctx, s.Compare, s.Tree)
		return ev.KnowTreeDiff(s.Compare, s.Tree, paths, removed, err)
	case s.Changed:
		return ev.KnowChanged(port.Changed(ctx))
	case s.Diff:
		return ev.KnowDiff(port.Diff(ctx))
	case s.Commit:
		return ev.KnowCommit(port.Commit(ctx))
	case s.Files:
		return ev.KnowFiles(port.Files(ctx))
	case s.Rules:
		return ev.KnowRules(port.LoadRules(ctx, ev.Workflow.RulesPath()))
	case s.State:
		return ev.KnowState(port.LoadState(ctx))
	}
	return lookup(ctx, port, ev, s)
}

// lookup carries out the step s that asks for a fact of a path, a folder, the programs or the files holding a text,
// through the port, and feeds it back to the evaluation ev; any other step is one that acts (act).
func lookup(ctx context.Context, port Port, ev hook.Evaluation, s hook.Step) hook.Evaluation {
	switch {
	case s.Ignored != "":
		return ev.KnowIgnored(s.Ignored, port.Ignored(ctx, s.Ignored))
	case s.Real != nil:
		return ev.KnowReal(s.Real, port.Real(ctx, s.Real))
	case s.Foreign != "":
		return ev.KnowForeign(s.Foreign, port.Foreign(ctx, s.Foreign))
	case s.Programs != nil:
		return ev.KnowMissing(port.Missing(ctx, s.Programs))
	case s.Contains != nil:
		return ev.KnowFound(s.Epoch, s.Contains, s.Text, port.Contains(ctx, s.Contains, s.Text))
	case s.Read != "":
		data, err := port.Read(ctx, s.Read)
		return ev.KnowContent(s.Epoch, s.Read, data, err)
	}
	return act(ctx, port, ev, s)
}

// act carries out the step s that acts, a run, a check, a restore of files or an append to the log, through the
// port and notes its outcome in the evaluation ev.
func act(ctx context.Context, port Port, ev hook.Evaluation, s hook.Step) hook.Evaluation {
	switch {
	case s.Run != nil:
		return ev.Note(port.Run(ctx, s.Run))
	case s.Check != nil:
		return ev.Note(port.RunCheck(ctx, *s.Check))
	case s.Restore != nil:
		return ev.Note("", port.Restore(ctx, s.Source, s.Restore))
	case s.Append != nil:
		return ev.Note("", port.Append(ctx, *s.Append))
	}
	return ev
}
