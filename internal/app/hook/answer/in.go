// Package answer is the use case AnswerHook, the command `polix hook <claude|codex>`: it answers one hook call of
// Claude Code or Codex, evaluating the protections and the workflow of polix.yaml for each of its events.
package answer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
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
	// TreeDiff returns the paths that differ between two content hashes, or why they cannot be compared.
	TreeDiff(ctx context.Context, from, to string) ([]string, error)
	// Changed returns the files not committed yet: changed since HEAD, or untracked.
	Changed(ctx context.Context) []string
	// Diff returns the size of the change of each file not committed yet, the same files as Changed, or why it
	// could not be measured.
	Diff(ctx context.Context) (diff.Diff, error)
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

// hookInput is the part of the input of a hook both agents share.
type hookInput struct {
	HookEventName string          `json:"hook_event_name"`
	SessionID     string          `json:"session_id"`
	Cwd           string          `json:"cwd"`
	ToolName      string          `json:"tool_name"`
	ToolInput     map[string]any  `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
	Prompt        string          `json:"prompt"`
	AgentType     string          `json:"agent_type"`
}

// formats are the formats of the agents, by the name of the agent.
var formats = map[string]format{agent.Claude: claude{}, agent.Codex: codex{}}

// gitCommit recognises a git commit anywhere in a command line (`cd x && git commit -m ...`), after global
// options of git, those with a separate value included (`git -C dir commit`, `git -c k=v commit`), and inside a
// quoted command (`bash -c "git commit ..."`). It errs on the side of a gate: the text of a commit inside quotes
// also counts.
var gitCommit = regexp.MustCompile(`(^|[;&|(\s'"])git(\s+(-[cC]|--git-dir|--work-tree|--namespace|--config-env)\s+\S+|\s+-\S+)*\s+commit(\s|$|['"])`)

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
// repository and loads the workflow through the port, and for each event locates its path and loops: it asks the
// logic for the next step, carries it out through the port and feeds the result back to the evaluation, until
// there is none; then it merges the answers of the events and renders them with the format of the agent. Outside a
// repository, or without events, it lets everything through. It only sequences; the decisions are in the logic and
// the domain.
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
	for i, e := range h.Events {
		if e.Edits() {
			e = e.Locate(port.Locate(ctx, e.AbsPath, e.Cwd))
		}
		ev := hook.Evaluation{Workflow: wf, Present: present, Invalid: problem, Root: root, GitDir: gitDir,
			System: req.System, Event: e, Remaining: h.Remaining(i), Facts: facts}
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
	}
	stdout, stderr, code := f.render(h.Native, answer)
	return Response{Stdout: stdout, Stderr: stderr, Code: code}, nil
}

// carry carries out the step s of the evaluation ev through the port and feeds its result back to the
// evaluation: the fact it asks for, or, for a step that acts, its outcome (act).
func carry(ctx context.Context, port Port, ev hook.Evaluation, s hook.Step) hook.Evaluation {
	switch {
	case s.Branch:
		return ev.KnowBranch(port.Head(ctx))
	case s.Hash:
		hash, err := port.Hash(ctx)
		return ev.KnowHash(s.Epoch, hash, err)
	case s.Compare != "":
		paths, err := port.TreeDiff(ctx, s.Compare, s.Tree)
		return ev.KnowTreeDiff(s.Compare, s.Tree, paths, err)
	case s.Changed:
		return ev.KnowChanged(port.Changed(ctx))
	case s.Diff:
		return ev.KnowDiff(port.Diff(ctx))
	case s.Files:
		return ev.KnowFiles(port.Files(ctx))
	case s.Rules:
		return ev.KnowRules(port.LoadRules(ctx, ev.Workflow.RulesPath()))
	case s.State:
		return ev.KnowState(port.LoadState(ctx))
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

// act carries out the step s that acts, a run, a check or an append to the log, through the port and notes its
// outcome in the evaluation ev.
func act(ctx context.Context, port Port, ev hook.Evaluation, s hook.Step) hook.Evaluation {
	switch {
	case s.Run != nil:
		return ev.Note(port.Run(ctx, s.Run))
	case s.Check != nil:
		return ev.Note(port.RunCheck(ctx, *s.Check))
	case s.Append != nil:
		return ev.Note("", port.Append(ctx, *s.Append))
	}
	return ev
}

// actor is the subagent type when the call comes from a subagent, the main session otherwise.
func (in hookInput) actor() string {
	if in.AgentType != "" {
		return in.AgentType
	}
	return agent.ActorSession
}

// str returns the text of the input of the tool under key, "" when it has none.
func (in hookInput) str(key string) string {
	s, _ := in.ToolInput[key].(string)
	return s
}

// exitCode reads the exit code of a command from the tool response, 0 when it does not say.
func (in hookInput) exitCode() int {
	var resp map[string]any
	if json.Unmarshal(in.ToolResponse, &resp) != nil {
		return 0
	}
	for _, k := range []string{"exit_code", "exitCode", "returncode"} {
		if n, ok := resp[k].(float64); ok {
			return int(n)
		}
	}
	return 0
}

// event returns the event name of the agent ag for the input, with abs, the path of an edit, and with command and
// code, a command line and its exit code; the error tells why it is not valid. The stop of a turn (stop) is
// always of the main session: a subagent stops with subagent_stop.
func (in hookInput) event(ag, name, abs, command string, code int) (agent.Event, error) {
	actor := in.actor()
	if name == workflow.Stop {
		actor = agent.ActorSession
	}
	ev := agent.Event{Name: name, Agent: ag, Session: in.SessionID, Actor: actor, AbsPath: abs, Command: command,
		ExitCode: code, Prompt: in.Prompt, Cwd: in.Cwd}
	return ev, ev.Valid()
}

// single returns the one event name of the agent ag for an input that is neither an edit nor a command.
func (in hookInput) single(ag, name string) ([]agent.Event, error) {
	ev, err := in.event(ag, name, "", "", 0)
	if err != nil {
		return nil, err
	}
	return []agent.Event{ev}, nil
}

// edits returns one event name of the agent ag per edited path. A relative path (apply_patch gives them) is taken
// from the cwd of the call, so AbsPath is absolute whenever the cwd is known.
func (in hookInput) edits(ag, name string, paths []string) ([]agent.Event, error) {
	var events []agent.Event
	for _, p := range paths {
		if !filepath.IsAbs(p) && in.Cwd != "" {
			p = filepath.Join(in.Cwd, p)
		}
		ev, err := in.event(ag, name, p, "", 0)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

// commands returns the event name of the agent ag of the command line command: after it ran (post_command), with
// the exit code of its response, 1 when the agent says it failed without one; about to run (pre_command), plus
// pre_commit when it commits.
func (in hookInput) commands(ag, name, command string, failed bool) ([]agent.Event, error) {
	code := 0
	if name == workflow.PostCommand {
		code = in.exitCode()
		if failed && code == 0 {
			code = 1
		}
	}
	names := []string{name}
	if name == workflow.PreCommand && gitCommit.MatchString(command) {
		names = append(names, workflow.PreCommit)
	}
	var events []agent.Event
	for _, n := range names {
		ev, err := in.event(ag, n, "", command, code)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}
