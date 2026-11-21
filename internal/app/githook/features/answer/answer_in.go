// Package answer is the use case AnswerGithook, the command `polix githook <pre-commit|commit-msg> [file]`: it
// answers one hook call of git, evaluating the entries of the githooks event of polix.yaml, and lets the commit go
// on or aborts it, whoever makes it.
package answer

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/githook"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

// Request is what the caller asks of the use case AnswerGithook: the command fills it from its arguments and flag.
type Request struct {
	Event string // the event of githooks the call of git runs: workflow.PreCommit or workflow.CommitMsg
	// File is the file of the message git passes to commit-msg, relative to the working directory; empty on
	// pre-commit, which takes none.
	File    string
	Dir     string // any folder of the repository; empty for the working directory, the root where git runs a hook
	Version string // version of the running polix, which polix.yaml may require
}

// Response is what the use case AnswerGithook gives back: the command prints it to stderr and exits with its code.
type Response struct {
	Reasons []string // why the commit is aborted, each line printed prefixed with `polix: `
	Code    int      // 0 lets the commit go on, 1 aborts it
}

// Port is what the shell of AnswerGithook needs from the outside: the repository, its workflow, its facts and the
// processes. It speaks in domain values, and every method takes the context of the caller.
type Port interface {
	// Open opens the repository that contains dir; the error tells that dir is not inside one.
	Open(ctx context.Context, dir string) error
	// LoadWorkflow reads polix.yaml for the polix of version running: whether it exists, and the error of a file
	// that cannot be read, parsed or used.
	LoadWorkflow(ctx context.Context, running string) (wf workflow.Workflow, present bool, err error)
	// Branch returns the current branch, "" on a detached HEAD.
	Branch(ctx context.Context) string
	// Staged returns the files staged for the commit, relative to the root with slashes; the error tells why git
	// cannot list them.
	Staged(ctx context.Context) ([]string, error)
	// Message returns the content of the message file git passed to commit-msg; the error tells why it cannot be
	// read.
	Message(ctx context.Context, file string) (string, error)
	// CommentChar returns the string that starts a comment line of the message, # unless git is told otherwise.
	CommentChar(ctx context.Context) string
	// Run runs argv from the root, with its output live to the terminal of the commit and no time limit, and
	// returns why it failed, nil when it passed.
	Run(ctx context.Context, argv []string) error
}

// events maps the name git gives each hook to the event of githooks it runs.
var events = map[string]string{"pre-commit": workflow.PreCommit, "commit-msg": workflow.CommitMsg}

// Command is the command `polix githook <pre-commit|commit-msg> [file]`, the entry of the use case AnswerGithook,
// which git runs from the hooks polix install declares in the configuration of the repository; hidden, since
// people do not run it. It turns the arguments and the flag into a Request, injects the adapter with the loader
// loadWorkflow as the port, runs the shell, prints the reasons of the response to stderr and exits with its code.
// An unknown event, or an error of the shell, exits with 1 and the error, so git aborts the commit.
func Command(loadWorkflow workflow.Load) *cobra.Command {
	req := Request{Version: build.Current()}
	c := &cobra.Command{
		Use:   "githook <pre-commit|commit-msg> [file]",
		Short: "Answer one hook call of git (pre-commit or commit-msg)",
		Long: "Entry point of the hooks of git that polix install declares in the configuration of the repository: " +
			"it evaluates the githooks section of polix.yaml for the event and exits with 1 to abort the commit, " +
			"with the reasons on stderr. Git runs it; people do not.",
		Hidden:    true,
		Args:      cobra.RangeArgs(1, 2),
		ValidArgs: []string{"pre-commit", "commit-msg"},
		RunE: func(c *cobra.Command, args []string) error {
			stderr := c.ErrOrStderr()
			ev, ok := events[args[0]]
			if !ok {
				fmt.Fprintf(stderr, "polix githook: unknown event %q (events: pre-commit, commit-msg)\n", args[0])
				return cli.Exit(1)
			}
			switch {
			case ev == workflow.CommitMsg && len(args) < 2:
				fmt.Fprintln(stderr, "polix githook: commit-msg needs the file of the message")
				return cli.Exit(1)
			case ev == workflow.PreCommit && len(args) > 1:
				fmt.Fprintln(stderr, "polix githook: pre-commit takes no file")
				return cli.Exit(1)
			case len(args) > 1:
				req.File = args[1]
			}
			req.Event = ev
			res, err := run(c.Context(), &Adapter{Workflow: loadWorkflow, Stdout: c.OutOrStdout(), Stderr: stderr}, req)
			if err != nil {
				fmt.Fprintln(stderr, "polix githook:", err)
				return cli.Exit(1)
			}
			res.print(stderr)
			return cli.Exit(res.Code)
		},
	}
	cli.DirFlag(c, &req.Dir, "")
	return c
}

// run is the shell, from request to response: it opens the repository and loads polix.yaml through the port, reads
// the branch, the staged files and, with the file of a message (commit_msg), the message and the comment string,
// and loops: it asks the logic for the next run, carries it out through the port and feeds its outcome back to the
// call, until the logic answers. Outside a repository the commit goes on. It only sequences; the decisions are in
// the logic and the domain.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	g := githook.Githook{Event: req.Event}
	if err := g.Valid(); err != nil {
		return Response{}, err
	}
	if err := port.Open(ctx, req.Dir); err != nil {
		return Response{}, nil //nolint:nilerr // outside a repository there is nothing to gate
	}
	wf, present, problem := port.LoadWorkflow(ctx, req.Version)
	branch := port.Branch(ctx)
	staged, err := port.Staged(ctx)
	if err != nil {
		return Response{}, err
	}
	var message, comment string
	if req.File != "" {
		if message, err = port.Message(ctx, req.File); err != nil {
			return Response{}, err
		}
		comment = port.CommentChar(ctx)
	}
	g = githook.Githook{Event: req.Event, Branch: branch, Staged: staged, Message: message, Comment: comment}
	if err := g.Valid(); err != nil {
		return Response{}, err
	}
	var out Output
	for {
		out = Decide(g, wf, present, problem)
		if out.Run == nil {
			break
		}
		g = g.Ran(port.Run(ctx, out.Run))
	}
	res := Response{Reasons: out.Reasons}
	if out.Block {
		res.Code = 1
	}
	return res, nil
}

// print writes each line of each reason to stderr prefixed with `polix: `, without the last line break of a
// reason, so the reasons of a commit read apart from the output of git.
func (res Response) print(stderr io.Writer) {
	for _, reason := range res.Reasons {
		for line := range strings.SplitSeq(strings.TrimRight(reason, "\n"), "\n") {
			fmt.Fprintln(stderr, strings.TrimRight("polix: "+line, " "))
		}
	}
}
