package setup

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/build"
	"github.com/mlophez/polix-ai/pkg/cli"
	"github.com/mlophez/polix-ai/pkg/git"
)

// CheckSetupRequest is what the caller asks of the use case CheckSetup, whatever the transport: the command fills
// it from its flags.
type CheckSetupRequest struct {
	Dir     string // any folder of the repository
	Version string // version of the running polix, which polix.yaml may require
}

// CheckSetupResponse is what the use case CheckSetup gives back to the caller, in the shape of the transport: the
// command serializes it to the console.
type CheckSetupResponse struct {
	Problems []string // every problem of polix.yaml and of ai/rules/
	Hint     string   // the hint that the problems may come from a newer polix; empty for none
	Warning  string   // the warning that rules check --changed would fail; empty for none
	Found    bool     // whether polix.yaml exists
	Rules    int      // how many rules ai/rules/ holds
}

// CheckSetupPort is what the shell of CheckSetup needs from the outside: the workflow, the rules, their checks and
// the base branches of the repository. It speaks in domain values, and every method takes the context of the
// caller.
type CheckSetupPort interface {
	// LoadWorkflow returns the workflow of polix.yaml for the polix of version running and whether the file exists:
	// the default workflow without it. The error tells why it cannot be read or parsed, or is a workflow.Invalid
	// with the problems of a workflow that parses, returned with it.
	LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, bool, error)
	// LoadRules returns every rule of ai/rules/, retired ones excluded, ordered by id, and the problems of the
	// invalid ones; the error tells why a file cannot be read or parsed.
	LoadRules(ctx context.Context) ([]rule.Rule, []string, error)
	// Missing reports whether the file of a check, relative to the root, does not exist.
	Missing(ctx context.Context, check string) bool
	// ResolveBase returns why none of candidates exists as a branch, locally or on origin; nil when one does.
	ResolveBase(ctx context.Context, candidates []string) error
}

// CheckSetupCommand is the command `polix check`, the entry of the use case CheckSetup: it turns the flags into a
// CheckSetupRequest, injects the adapter as the port, runs the shell and serializes the CheckSetupResponse to the
// console.
func CheckSetupCommand() *cobra.Command {
	req := CheckSetupRequest{Version: build.Current()}
	c := &cobra.Command{
		Use:   "check",
		Short: "Validate polix.yaml and ai/rules/",
		Long: "Validate the workflow and the rules and print every problem: unknown events, conditions, actions and " +
			"keys, undefined variables and conditions on events where they never hold.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := git.Open(c.Context(), req.Dir)
			if err != nil {
				return err
			}
			res, err := checkSetup(c.Context(), CheckSetupAdapter{Repo: repo}, req)
			if err != nil {
				return err
			}
			return cli.Exit(res.print(c.OutOrStdout(), c.ErrOrStderr()))
		},
	}
	cli.DirFlag(c, &req.Dir, ".")
	return c
}

// checkSetup is the shell, from request to response: it reads through the port the workflow, the rules, whether
// each check exists and whether a base branch exists, calls the logic once and builds the response.
func checkSetup(ctx context.Context, port CheckSetupPort, req CheckSetupRequest) (CheckSetupResponse, error) {
	wf, found, wfErr := port.LoadWorkflow(ctx, req.Version)
	rules, invalid, rulesErr := port.LoadRules(ctx)
	missing := map[string]bool{}
	for _, r := range rules {
		if r.HasCheck() {
			missing[r.Check] = port.Missing(ctx, r.Check)
		}
	}
	baseErr := port.ResolveBase(ctx, wf.BaseBranches())
	out := CheckSetup(req.Version, found, wfErr, rules, invalid, rulesErr, missing, baseErr)
	return CheckSetupResponse(out), nil
}

// print serializes the response to the console and returns the exit code: every problem to stderr, with the hint
// of a newer polix, and 1; else the warning to stderr, the summary to stdout and 0.
func (res CheckSetupResponse) print(stdout, stderr io.Writer) int {
	if len(res.Problems) > 0 {
		u := cli.NewUI(stderr)
		for _, p := range res.Problems {
			u.Fail("%s", p)
		}
		if res.Hint != "" {
			u.Warn("%s", res.Hint)
		}
		return 1
	}
	if res.Warning != "" {
		cli.NewUI(stderr).Warn("%s", res.Warning)
	}
	u := cli.NewUI(stdout)
	if !res.Found {
		u.Line("%s no %s (only the built-in protections apply) and %d rules of %s/", cli.OKText("ok:"), workflow.File,
			res.Rules, rule.Dir)
		return 0
	}
	u.Line("%s %s and %d rules of %s/", cli.OKText("ok:"), workflow.File, res.Rules, rule.Dir)
	return 0
}
