package hook

import (
	"context"
	"fmt"
	"io"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/agent"
	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/build"
	"github.com/mlophez/polix-ai/pkg/cli"
)

// AnswerHookRequest is what the caller asks of the use case AnswerHook, whatever the transport: the command
// fills it from its argument, its flag and its stdin.
type AnswerHookRequest struct {
	Agent   string // the agent behind the call: claude or codex
	Dir     string // the repository; empty takes the cwd of the call, else the working directory
	Input   []byte // the input of the hook, as the agent wrote it on stdin
	Version string // version of the running polix, which polix.yaml may require
	System  string // the system polix runs on (GOOS)
}

// AnswerHookResponse is what the use case AnswerHook gives back to the caller, in the shape the agent reads:
// the command writes it as it is.
type AnswerHookResponse struct {
	Stdout string
	Stderr string
	Code   int // 0 lets the action go on, 2 blocks it
}

// AnswerHookPort is what the shell of AnswerHook needs from the outside: the repository, its workflow, its
// facts, the processes and the log. It speaks in domain values, and every method takes the context of the
// caller.
type AnswerHookPort interface {
	// Open opens the repository that contains dir and returns its root and its git directory.
	Open(ctx context.Context, dir string) (root, gitDir string, err error)
	// LoadWorkflow reads polix.yaml for the polix of version running: whether it exists, and the error of a file
	// that cannot be read, parsed or used, which is data of the answer.
	LoadWorkflow(ctx context.Context, running string) (wf workflow.Workflow, present bool, err error)
	// Locate turns a path given by the agent from cwd into one relative to the root; inside is false outside it.
	Locate(ctx context.Context, path, cwd string) (rel string, inside bool)
	// Branch returns the current branch, "" on a detached HEAD.
	Branch(ctx context.Context) string
	// Hash returns the hash of the current content of the working tree.
	Hash(ctx context.Context) (string, error)
	// Changed returns the files not committed yet: changed since HEAD, or untracked.
	Changed(ctx context.Context) []string
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
	// Append appends an entry to the log, stamped with the time.
	Append(ctx context.Context, e state.Entry) error
}

// AnswerHookCommand is the command `polix hook <claude|codex>`, the entry of the use case AnswerHook: it turns
// the argument, the flag and stdin into an AnswerHookRequest, injects the adapter as the port, runs the shell and
// writes the AnswerHookResponse as the agent expects it. Its failure policy: an unknown agent exits with 1; an
// input that cannot be read or parsed lets the action go on, with the error on stderr.
func AnswerHookCommand() *cobra.Command {
	req := AnswerHookRequest{Version: build.Current(), System: runtime.GOOS}
	c := &cobra.Command{
		Use:   "hook <claude|codex>",
		Short: "Answer one hook call of an agent (event on stdin)",
		Long: "Entry point of the hooks of Claude Code and Codex: it reads the event on stdin and answers with the " +
			"exit code (2 blocks) and the output the agent expects. The polix plugin registers it; people do not " +
			"run it.",
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
			res, err := answerHook(c.Context(), &AnswerHookAdapter{}, req)
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

// answerHook is the shell, from request to response: it parses the input with the format of the agent, opens the
// repository and loads the workflow through the port, and for each event locates its path and loops: it asks the
// logic for the next step, carries it out through the port and feeds the result back to the evaluation, until
// there is none; then it merges the answers of the events and renders them for the agent. Outside a repository,
// or without events, it lets everything through. It only sequences; the decisions are in the logic and the
// domain.
func answerHook(ctx context.Context, port AnswerHookPort, req AnswerHookRequest) (AnswerHookResponse, error) {
	ag := agent.Agent{Name: req.Agent}
	if err := ag.Valid(); err != nil {
		return AnswerHookResponse{}, err
	}
	native, events, err := ag.Parse(req.Input)
	if err != nil {
		return AnswerHookResponse{}, err
	}
	h := Hook{Agent: ag, Native: native, Events: events}
	if err := h.Valid(); err != nil {
		return AnswerHookResponse{}, err
	}
	if len(h.Events) == 0 {
		return AnswerHookResponse{}, nil
	}
	root, gitDir, err := port.Open(ctx, h.Folder(req.Dir))
	if err != nil {
		return AnswerHookResponse{}, nil // outside a repository there is nothing to govern
	}
	wf, present, problem := port.LoadWorkflow(ctx, req.Version)
	invalid := Unusable(problem, req.Version)
	var facts Facts
	var answer agent.Answer
	for _, e := range h.Events {
		if e.Edits() {
			e = e.Locate(port.Locate(ctx, e.AbsPath, e.Cwd))
		}
		ev := Evaluation{Workflow: wf, Present: present, Invalid: invalid, Root: root, GitDir: gitDir,
			System: req.System, Event: e, Facts: facts}
		if err := ev.Valid(); err != nil {
			return AnswerHookResponse{}, err
		}
		var out AnswerHookOutput
		for {
			out = AnswerHook(ev)
			s := out.Step
			switch {
			case s.None():
			case s.Branch:
				ev = ev.KnowBranch(port.Branch(ctx))
			case s.Hash:
				hash, err := port.Hash(ctx)
				ev = ev.KnowHash(s.Epoch, hash, err)
			case s.Changed:
				ev = ev.KnowChanged(port.Changed(ctx))
			case s.Files:
				ev = ev.KnowFiles(port.Files(ctx))
			case s.Rules:
				ev = ev.KnowRules(port.LoadRules(ctx, wf.RulesPath()))
			case s.State:
				ev = ev.KnowState(port.LoadState(ctx))
			case s.Ignored != "":
				ev = ev.KnowIgnored(s.Ignored, port.Ignored(ctx, s.Ignored))
			case s.Real != nil:
				ev = ev.KnowReal(s.Real, port.Real(ctx, s.Real))
			case s.Contains != nil:
				ev = ev.KnowFound(s.Epoch, s.Contains, s.Text, port.Contains(ctx, s.Contains, s.Text))
			case s.Read != "":
				data, err := port.Read(ctx, s.Read)
				ev = ev.KnowContent(s.Epoch, s.Read, data, err)
			case s.Run != nil:
				ev = ev.Note(port.Run(ctx, s.Run))
			case s.Check != nil:
				ev = ev.Note(port.RunCheck(ctx, *s.Check))
			case s.Append != nil:
				ev = ev.Note("", port.Append(ctx, *s.Append))
			}
			if s.None() {
				break
			}
		}
		answer = answer.Merge(out.Answer)
		facts = out.Facts
	}
	stdout, stderr, code := ag.Render(h.Native, answer)
	return AnswerHookResponse{Stdout: stdout, Stderr: stderr, Code: code}, nil
}
