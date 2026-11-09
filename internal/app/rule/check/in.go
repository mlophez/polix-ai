// Package check is the use case CheckRule, the command `polix rules check`: it runs the checks of the
// deterministic rules of the folder of rules, all of them or only those of the files changed by the branch.
package check

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Request is what the caller asks of the use case CheckRule, whatever the transport: the command fills
// it from its flags.
type Request struct {
	Dir         string // any folder of the repository
	OnlyChanged bool   // only the rules whose scope matches the files changed by the branch
	Base        string // branch the change merges into, with OnlyChanged; empty takes the bases of polix.yaml
	Version     string // version of the running polix, which polix.yaml may require
}

// Response is what the use case CheckRule gives back to the caller, in the shape of the transport: the
// command serializes it to the console.
type Response struct {
	Ran    int
	Failed []Failed
}

// Failed is one failed check as the caller sees it.
type Failed struct {
	ID          string
	Check       string
	Description string
	Output      string
}

// Port is what the shell of CheckRule needs from the outside. It is declared here, where it is used, as
// an interface so its implementation can change; the entry injects the adapter, and the logic never sees it. It
// speaks in domain values, and every method takes the context of the caller.
type Port interface {
	// LoadRules returns the rules of the folder dir, relative to the root, retired ones excluded, ordered by id.
	LoadRules(ctx context.Context, dir string) ([]rule.Rule, error)
	// LoadWorkflow returns the workflow of polix.yaml for the polix of version running, the default one without
	// the file.
	LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, error)
	// Changed returns the files changed against the first of candidates that exists.
	Changed(ctx context.Context, candidates []string) ([]string, error)
	// RunCheck runs one invocation of run from the root of the repository, which may decide several rules, and
	// returns its output.
	RunCheck(ctx context.Context, run rule.Run) (string, error)
}

// Command is the command `polix rules check`, the entry of the use case CheckRule: it turns the flags
// into a Request and validates them, injects the adapter with the loaders rules and loadWorkflow as the port, runs
// the shell and serializes the Response to the console.
func Command(rules rule.Load, loadWorkflow workflow.Load) *cobra.Command {
	req := Request{Version: build.Current()}
	c := &cobra.Command{
		Use:   "check",
		Short: "Run the checks of the deterministic rules",
		Long: "Run the checks of the deterministic rules: all of them, or with --changed only those whose scope " +
			"matches the files changed by the branch. Each check runs through the first runner of rules.run in " +
			workflow.File + " whose glob matches its file, or as a program; a runner with {names}, {files} or " +
			"{dirs} runs its checks together, and reruns each alone when they fail. Meant for CI.",
		Example: "  polix rules check\n  polix rules check --changed --base develop",
		Args:    cobra.NoArgs,
		PreRunE: func(*cobra.Command, []string) error { return flags(req) },
		RunE: func(c *cobra.Command, _ []string) error {
			var res Response
			repo, err := git.Open(c.Context(), req.Dir)
			if err == nil {
				res, err = run(c.Context(), Adapter{Repo: repo, Log: c.ErrOrStderr(), Rules: rules,
					Workflow: loadWorkflow}, req)
			}
			if err != nil {
				return err
			}
			res.print(c.OutOrStdout(), c.ErrOrStderr())
			if len(res.Failed) > 0 {
				return cli.Exit(1)
			}
			return nil
		},
	}
	cli.DirFlag(c, &req.Dir, ".")
	c.Flags().BoolVar(&req.OnlyChanged, "changed", false, "only the rules whose scope matches the changed files")
	c.Flags().StringVar(&req.Base, "base", "", "branch the change merges into, for --changed, locally or on origin "+
		"(default: the closest of base_branch in "+workflow.File+", else "+workflow.DefaultBase+")")
	return c
}

// flags rejects a combination of flags that means nothing: --base only applies with --changed.
func flags(req Request) error {
	if req.Base != "" && !req.OnlyChanged {
		return errors.New("--base needs --changed")
	}
	return nil
}

// run is the shell, from request to response: it reads through the port what the logic needs, branching on
// the request to know what to read, calls the logic in a loop from one call site (the step shape), carries out
// each run it asks for through the port and gives the logic its outcome, and builds the
// response. It only sequences; the decisions are in the logic and the domain.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	wf, rules, err := load(ctx, port, req)
	if err != nil {
		return Response{}, err
	}
	var changed []string
	if req.OnlyChanged {
		if changed, err = port.Changed(ctx, wf.Bases(req.Base)); err != nil {
			return Response{}, err
		}
	}
	var outcomes []rule.Outcome
	for {
		out := Decide(rules, wf, req.OnlyChanged, changed, outcomes)
		if out.Next == nil {
			return response(out.Results)
		}
		text, err := port.RunCheck(ctx, *out.Next)
		o := rule.Outcome{Output: text, Err: err}
		if err := o.Valid(); err != nil {
			return Response{}, err
		}
		outcomes = append(outcomes, o)
	}
}

// load reads through the port the workflow of polix.yaml, whose runners of rules.run must be valid, and the rules
// of its folder of rules, every one valid; the error tells which cannot be read or is not valid.
func load(ctx context.Context, port Port, req Request) (workflow.Workflow, []rule.Rule, error) {
	wf, err := port.LoadWorkflow(ctx, req.Version)
	if err != nil {
		return workflow.Workflow{}, nil, err
	}
	if err := wf.Rules.Valid(); err != nil {
		return workflow.Workflow{}, nil, err
	}
	rules, err := port.LoadRules(ctx, wf.RulesPath())
	if err != nil {
		return workflow.Workflow{}, nil, err
	}
	for _, r := range rules {
		if err := r.Valid(); err != nil {
			return workflow.Workflow{}, nil, err
		}
	}
	return wf, rules, nil
}

// response turns the results of carrying out the output, each valid, into the response the caller sees.
func response(results []rule.Result) (Response, error) {
	for _, res := range results {
		if err := res.Valid(); err != nil {
			return Response{}, err
		}
	}
	res := Response{Ran: len(results)}
	for _, f := range rule.Results(results).Failed() {
		res.Failed = append(res.Failed, Failed{
			ID:          f.Rule.ID,
			Check:       f.Rule.Check,
			Description: f.Rule.Description,
			Output:      f.Tail(),
		})
	}
	return res, nil
}

// print serializes the response to the console: each failure to stderr, and the summary to stdout, as a failure
// when a check failed.
func (res Response) print(stdout, stderr io.Writer) {
	ue := cli.NewUI(stderr)
	for _, f := range res.Failed {
		ue.Fail("FAIL %s (%s): %s", f.ID, f.Check, f.Description)
		fmt.Fprintln(stderr, f.Output)
	}
	if len(res.Failed) > 0 {
		cli.NewUI(stdout).Fail("%d checks, %d failed", res.Ran, len(res.Failed))
		return
	}
	cli.NewUI(stdout).OK("%d checks, %d failed", res.Ran, len(res.Failed))
}
