package rule

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/cli"
	"github.com/mlophez/polix-ai/pkg/git"
)

// CheckRuleRequest is what the caller asks of the use case CheckRule, whatever the transport: the command fills
// it from its flags.
type CheckRuleRequest struct {
	Dir         string // any folder of the repository
	OnlyChanged bool   // only the rules whose scope matches the files changed by the branch
	Base        string // branch the change merges into; empty takes the base of polix.yaml
}

// CheckRuleResponse is what the use case CheckRule gives back to the caller, in the shape of the transport: the
// command serializes it to the console.
type CheckRuleResponse struct {
	Ran    int
	Failed []CheckRuleFailed
}

// CheckRuleFailed is one failed check as the caller sees it.
type CheckRuleFailed struct {
	ID          string
	Check       string
	Description string
	Output      string
}

// CheckRulePort is what the shell of CheckRule needs from the outside. It is declared here, where it is used, as
// an interface so its implementation can change; the entry injects the adapter, and the logic never sees it. It
// speaks in domain values, and every method takes the context of the caller.
type CheckRulePort interface {
	// LoadRules returns the rules of the repository, retired ones excluded, ordered by id.
	LoadRules(ctx context.Context) ([]Rule, error)
	// BaseBranches returns the base branches of polix.yaml, closest first.
	BaseBranches(ctx context.Context) ([]string, error)
	// Changed returns the files changed against the first of candidates that exists.
	Changed(ctx context.Context, candidates []string) ([]string, error)
	// RunScript runs one check script from the root of the repository and returns its output.
	RunScript(ctx context.Context, script string) (string, error)
}

// CheckRuleCommand is the command `polix rules check`, the entry of the use case CheckRule: it turns the flags
// into a CheckRuleRequest, injects the adapter as the port, runs the shell and serializes the CheckRuleResponse
// to the console.
func CheckRuleCommand() *cobra.Command {
	var req CheckRuleRequest
	c := &cobra.Command{
		Use:   "check",
		Short: "Run the checks of the deterministic rules",
		Long: "Run the checks of the deterministic rules: all of them, or with --changed only those whose scope " +
			"matches the files changed by the branch. Meant for CI.",
		Example: "  polix rules check\n  polix rules check --changed --base develop",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			var res CheckRuleResponse
			repo, err := git.Open(req.Dir)
			if err == nil {
				res, err = checkRule(c.Context(), CheckRuleAdapter{Repo: repo}, req)
			}
			if err != nil {
				cli.NewUI(c.ErrOrStderr()).Fail("%v", err)
				return cli.Exit(1)
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
	c.Flags().StringVar(&req.Base, "base", "", "branch the change merges into, for --changed "+
		"(default: the closest of base in "+workflow.File+", else "+workflow.DefaultBase+")")
	return c
}

// checkRule is the shell, from request to response: it reads through the port and turns the request into domain
// values, calls the logic once, carries out its output through the port and builds the response. It only
// sequences; the decisions are in the logic and the domain.
func checkRule(ctx context.Context, port CheckRulePort, req CheckRuleRequest) (CheckRuleResponse, error) {
	rules, err := port.LoadRules(ctx)
	if err != nil {
		return CheckRuleResponse{}, err
	}
	var changed []string
	if req.OnlyChanged {
		bases, err := port.BaseBranches(ctx)
		if err != nil {
			return CheckRuleResponse{}, err
		}
		if changed, err = port.Changed(ctx, workflow.Bases(req.Base, bases)); err != nil {
			return CheckRuleResponse{}, fmt.Errorf("%w: set base in %s or pass --base", err, workflow.File)
		}
	}
	out := CheckRule(rules, req.OnlyChanged, changed)
	var results []Result
	for _, r := range out.Run {
		text, err := port.RunScript(ctx, r.Check)
		res := Result{Rule: r, Output: text, Err: err}
		if err := res.Valid(); err != nil {
			return CheckRuleResponse{}, err
		}
		results = append(results, res)
	}
	return checkRuleResponse(results), nil
}

// checkRuleResponse turns the results of carrying out the output into the response the caller sees.
func checkRuleResponse(results []Result) CheckRuleResponse {
	res := CheckRuleResponse{Ran: len(results)}
	for _, f := range Failed(results) {
		res.Failed = append(res.Failed, CheckRuleFailed{
			ID:          f.Rule.ID,
			Check:       f.Rule.Check,
			Description: f.Rule.Description,
			Output:      f.Tail(),
		})
	}
	return res
}

// print serializes the response to the console: each failure to stderr, and the summary to stdout, or to stderr
// when a check failed.
func (res CheckRuleResponse) print(stdout, stderr io.Writer) {
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
