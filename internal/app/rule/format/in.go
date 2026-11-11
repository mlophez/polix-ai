// Package format is the use case FormatRule, the command `polix rules fmt`: it formats every rules file of the
// folder of rules in place, or with --check only tells whether they are formatted.
package format

import (
	"context"
	"io"
	"maps"
	"slices"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Request is what the caller asks of the use case FormatRule: the command fills it from its flags.
type Request struct {
	Dir     string // any folder of the repository
	Check   bool   // only tell whether the rules files are formatted, without writing them
	Version string // version of the running polix, which polix.yaml may require
}

// Response is what the use case FormatRule gives back: the command prints it.
type Response struct {
	Dir      string   // the folder of rules, relative to the root, with slashes
	Empty    bool     // the folder does not exist or holds no yaml file: nothing to format
	Problems []string // why each file cannot be formatted, by name, then each id repeated or the loader error
	Changed  []string // the files the format changes, as <dir>/<name>, sorted by name
	Written  []string // the files written formatted, as <dir>/<name>, in the order of their names
	Fix      string   // with a change, the command that formats the files
}

// Port is what the shell of FormatRule needs from the outside; Adapter implements it. It speaks in domain values,
// and every method takes the context of the caller.
type Port interface {
	// LoadWorkflow returns the workflow of polix.yaml for the polix of version running, the default one without the
	// file; the error tells why polix.yaml cannot be read, parsed or requires a newer polix.
	LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, error)
	// ReadRuleFiles returns the data of every yaml file of the folder dir, relative to the root, by name; none without
	// the folder. The error tells why the folder or a file of it cannot be read.
	ReadRuleFiles(ctx context.Context, dir string) (map[string][]byte, error)
	// WriteRuleFile writes data to the file name of the folder dir; the error tells why it cannot be written.
	WriteRuleFile(ctx context.Context, dir, name string, data []byte) error
	// RepeatedIDs returns the problem of each id repeated among the rules of the folder dir, retired ones included,
	// as the loader of the rules words it (rule.Loaded.Repeated). When the loader stops at a file that cannot be read,
	// is not yaml or does not decode as rules, it returns that error alone, since no repeat can be ruled out.
	RepeatedIDs(ctx context.Context, dir string) []string
}

// Command is the command `polix rules fmt`, the entry of the use case FormatRule: it turns the flags into a Request,
// injects the adapter on the repository of --dir with the loaders loadRules and loadWorkflow as the port, runs the
// shell and prints the Response.
func Command(loadRules rule.Load, loadWorkflow workflow.Load) *cobra.Command {
	req := Request{Version: build.Current()}
	c := &cobra.Command{
		Use:   "fmt",
		Short: "Format the rules files",
		Long: "Format every yaml file of the folder of rules (rules.path of " + workflow.File + ", " +
			workflow.DefaultRulesPath + " by default) in place, retired.yaml included: two spaces of indentation, flow " +
			"collections on one line, the comments and blank lines kept, rules first and the keys of each rule in the " +
			"order id, description, scope, deterministic, check, reason, and one blank line between two rules. When a " +
			"file cannot be formatted or does not decode as rules, or two rules share an id, no file is written. " +
			"With --check it only tells whether the files are formatted, and exits with 1 when not.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := git.Open(c.Context(), req.Dir)
			if err != nil {
				return err
			}
			res, err := run(c.Context(), Adapter{Root: repo.Root, Rules: loadRules, Workflow: loadWorkflow}, req)
			if err != nil {
				return err
			}
			return cli.Exit(res.print(c.OutOrStdout(), c.ErrOrStderr()))
		},
	}
	cli.DirFlag(c, &req.Dir, ".")
	c.Flags().BoolVar(&req.Check, "check", false, "only tell whether the rules files are formatted, exiting with 1 "+
		"when not")
	return c
}

// run is the shell: it loads the workflow and checks its rules.path, reads the rules files of that folder and then
// the ids repeated among their rules through the port, decides with the logic, writes each formatted file the logic
// gives in the order of their names, and builds the response.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	wf, err := port.LoadWorkflow(ctx, req.Version)
	if err != nil {
		return Response{}, err
	}
	if err := wf.Rules.ValidPath(); err != nil {
		return Response{}, err
	}
	files, err := port.ReadRuleFiles(ctx, wf.RulesPath())
	if err != nil {
		return Response{}, err
	}
	repeated := port.RepeatedIDs(ctx, wf.RulesPath())
	out := Decide(wf.RulesPath(), files, repeated, req.Check)
	res := Response{Dir: out.Dir, Empty: out.Empty, Problems: out.Problems, Changed: out.Changed, Fix: out.Fix}
	for _, name := range slices.Sorted(maps.Keys(out.Write)) {
		if err := port.WriteRuleFile(ctx, out.Dir, name, out.Write[name]); err != nil {
			return Response{}, err
		}
		res.Written = append(res.Written, out.Dir+"/"+name)
	}
	return res, nil
}

// print serializes the response to the console and returns the exit code: the problems and the files --check finds
// unformatted to stderr and 1; else what was done to stdout and 0.
func (res Response) print(stdout, stderr io.Writer) int {
	ui, ue := cli.NewUI(stdout), cli.NewUI(stderr)
	switch {
	case len(res.Problems) > 0:
		for _, p := range res.Problems {
			ue.Fail("%s", p)
		}
		res.notFormatted(ue)
		return 1
	case res.Empty:
		ui.Line("%s no rules to format in %s", cli.OKText("ok:"), res.Dir)
	case len(res.Written) > 0:
		for _, f := range res.Written {
			ui.Line("%s formatted %s", cli.OKText("ok:"), f)
		}
	case len(res.Changed) > 0:
		res.notFormatted(ue)
		return 1
	default:
		ui.Line("%s the rules are formatted", cli.OKText("ok:"))
	}
	return 0
}

// notFormatted prints each changed file as not formatted, with the command that formats it.
func (res Response) notFormatted(ue *cli.UI) {
	for _, f := range res.Changed {
		ue.Fail("%s is not formatted: run %s", f, res.Fix)
	}
}
