// Package check is the use case CheckSetup, the command `polix config check`: it validates the workflow and the
// rules of a repository, or another workflow file with those rules, and prints every problem.
package check

import (
	"context"
	"errors"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Request is what the caller asks of the use case CheckSetup, whatever the transport: the command fills
// it from its flags.
type Request struct {
	Dir     string // any folder of the repository
	File    string // the workflow file to check in place of polix.yaml, as the argument names it; empty for none
	Version string // version of the running polix, which polix.yaml may require
}

// Response is what the use case CheckSetup gives back to the caller, in the shape of the transport: the
// command serializes it to the console.
type Response struct {
	File      string   // the workflow file, as the console names it: the argument, or polix.yaml
	Problems  []string // every problem of the workflow file and of the rules
	Hint      string   // the hint that the problems may come from a newer polix; empty for none
	Warning   string   // the warning that rules check --changed would fail; empty for none
	Found     bool     // whether the workflow file exists
	Rules     int      // how many rules the folder of rules holds
	RulesPath string   // the folder of the rules, relative to the root
}

// Port is what the shell of CheckSetup needs from the outside: the workflow, the rules, their checks and
// the base branches of the repository. It speaks in domain values, and every method takes the context of the
// caller.
type Port interface {
	// LoadWorkflow returns the workflow of polix.yaml for the polix of version running and whether the file exists:
	// the default workflow without it. The error tells why it cannot be read or parsed, or is a workflow.InvalidError
	// with the problems of a workflow that parses, returned with it.
	LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, bool, error)
	// LoadRules returns every rule of the folder dir, relative to the root, retired ones excluded, ordered by id,
	// and the problems of the invalid ones; the error tells why a file cannot be read or parsed.
	LoadRules(ctx context.Context, dir string) ([]rule.Rule, []string, error)
	// Missing reports whether the file of a check, relative to the root, does not exist.
	Missing(ctx context.Context, check string) bool
	// ResolveBase returns why none of candidates exists as a branch, locally or on origin; nil when one does.
	ResolveBase(ctx context.Context, candidates []string) error
}

// Command is the command `polix config check`, a subcommand of `polix config` and the entry of the use
// case CheckSetup: it turns the flags into a Request, injects the adapter with the loaders rules and loadWorkflow as
// the port, runs the shell and serializes the Response to the console.
func Command(rules rule.Load, loadWorkflow workflow.Load) *cobra.Command {
	req := Request{Version: build.Current()}
	c := &cobra.Command{
		Use:   "check [file]",
		Short: "Validate polix.yaml and the rules",
		Long: "Validate the workflow and the rules and print every problem: unknown events, conditions, actions and " +
			"keys, undefined variables and conditions on events where they never hold. With a file, relative to " +
			"the working folder, it validates that workflow file in place of polix.yaml, as examples/polix.yaml, " +
			"with the rules and branches of the repository.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			repo, err := git.Open(c.Context(), req.Dir)
			if err != nil {
				return err
			}
			path := filepath.Join(repo.Root, workflow.File)
			if len(args) == 1 {
				if args[0] == "" {
					return errors.New("the file cannot be empty: name a workflow file, or none for polix.yaml")
				}
				req.File, path = args[0], args[0]
			}
			res, err := run(c.Context(), Adapter{Repo: repo, Path: path, Rules: rules, Workflow: loadWorkflow}, req)
			if err != nil {
				return err
			}
			return cli.Exit(res.print(c.OutOrStdout(), c.ErrOrStderr()))
		},
	}
	cli.DirFlag(c, &req.Dir, ".")
	return c
}

// run is the shell, from request to response: it reads through the port the workflow, the rules, whether
// each check exists and whether a base branch exists, builds the inspection of the setup of the running version,
// calls the logic once and builds the response.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	wf, found, wfErr := port.LoadWorkflow(ctx, req.Version)
	rules, invalid, rulesErr := port.LoadRules(ctx, wf.RulesPath())
	missing := map[string]bool{}
	for _, r := range rules {
		if r.HasCheck() {
			missing[r.Check] = port.Missing(ctx, r.Check)
		}
	}
	s := setup.Setup{Version: req.Version}
	if err := s.Valid(); err != nil {
		return Response{}, err
	}
	i := setup.Inspection{Setup: s, Workflow: wf, Found: found, WorkflowErr: wfErr, Rules: rules, Invalid: invalid,
		RulesErr: rulesErr, Missing: missing, BaseErr: port.ResolveBase(ctx, wf.BaseBranches())}
	if err := i.Valid(); err != nil {
		return Response{}, err
	}
	return Response(Decide(i, req.File)), nil
}

// print serializes the response to the console and returns the exit code: every problem to stderr, with the hint
// of a newer polix, and 1; else the warning to stderr, the summary to stdout and 0.
func (res Response) print(stdout, stderr io.Writer) int {
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
		u.Line("%s no %s (only the built-in protections apply) and %d rules of %s/", cli.OKText("ok:"), res.File,
			res.Rules, res.RulesPath)
		return 0
	}
	u.Line("%s %s and %d rules of %s/", cli.OKText("ok:"), res.File, res.Rules, res.RulesPath)
	return 0
}
