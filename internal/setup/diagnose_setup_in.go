package setup

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/version"
	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/build"
	"github.com/mlophez/polix-ai/pkg/cli"
)

// DiagnoseSetupRequest is what the caller asks of the use case DiagnoseSetup, whatever the transport: the command
// fills it from its flags.
type DiagnoseSetupRequest struct {
	Dir     string    // any folder of the repository to check
	Hook    bool      // the session start hook: quiet when fine, never fails, no network
	Offline bool      // do not ask for the latest release
	Version string    // version of the running polix
	Now     time.Time // the time of the call, recorded in the cache when the latest release is asked
}

// DiagnoseSetupResponse is what the use case DiagnoseSetup gives back to the caller, in the shape of the
// transport: the command serializes it to the console.
type DiagnoseSetupResponse struct {
	Hook     bool      // the response of the session start hook: only the problems, and it never fails
	Findings []Finding // the findings of the report, in order
	Failed   bool      // whether a finding is a failure
}

// DiagnoseSetupPort is what the shell of DiagnoseSetup needs from the outside: the binary, the PATH, the
// releases, the repository with its workflow, rules and state, the files of the agents and the CLI of Claude Code.
// It speaks in domain values, and every method takes the context of the caller.
type DiagnoseSetupPort interface {
	// Home returns the home folder of the user; empty when unknown.
	Home(ctx context.Context) string
	// Executable returns the real path of the running binary.
	Executable(ctx context.Context) (string, error)
	// LoadCache returns the cache of the checks of the latest release, empty when there is none.
	LoadCache(ctx context.Context) version.Cache
	// SaveCache writes the cache.
	SaveCache(ctx context.Context, c version.Cache) error
	// Latest asks the releases for the latest one.
	Latest(ctx context.Context) (string, error)
	// LookPath returns the real path of the program name the PATH runs.
	LookPath(ctx context.Context, name string) (string, error)
	// SameFile reports whether the paths a and b are the same file.
	SameFile(ctx context.Context, a, b string) bool
	// OpenRepo returns the root and the git directory of the repository that holds dir.
	OpenRepo(ctx context.Context, dir string) (string, string, error)
	// LoadWorkflow returns the workflow of polix.yaml of root for the polix of version running and whether the
	// file exists; the error tells why it cannot be read or parsed, or is a workflow.Invalid.
	LoadWorkflow(ctx context.Context, root, running string) (workflow.Workflow, bool, error)
	// LoadRules returns every rule of the folder dir of root, ordered by id, and the problems of the invalid ones.
	LoadRules(ctx context.Context, root, dir string) ([]rule.Rule, []string, error)
	// Missing reports whether the file of a check, relative to root, does not exist.
	Missing(ctx context.Context, root, check string) bool
	// Runnable returns why a check, relative to root, cannot run as a program; nil when it can or does not exist.
	Runnable(ctx context.Context, root, check string) error
	// ResolveBase returns why none of candidates exists as a branch of the repository of root.
	ResolveBase(ctx context.Context, root string, candidates []string) error
	// WriteState checks that the folder of the state, below gitDir, can be written, and returns it.
	WriteState(ctx context.Context, gitDir string) (string, error)
	// ReadFiles returns the content of the files of paths that exist, and why the others that exist cannot be read.
	ReadFiles(ctx context.Context, paths []string) (map[string][]byte, map[string]error)
	// Plugin returns the files of the plugin embedded in the binary, by path below polix/.
	Plugin(ctx context.Context) (map[string][]byte, error)
	// ClaudePlugins returns what `claude plugin list --json` printed; ErrNoCLI without the CLI.
	ClaudePlugins(ctx context.Context) ([]byte, error)
}

// DiagnoseSetupCommand is the command `polix doctor`, the entry of the use case DiagnoseSetup: it turns the flags
// into a DiagnoseSetupRequest, injects the adapter as the port, runs the shell and serializes the
// DiagnoseSetupResponse to the console.
func DiagnoseSetupCommand() *cobra.Command {
	var req DiagnoseSetupRequest
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check that this machine and repository are ready for polix",
		Long: "Check the binary and its version, the PATH the hooks use, git, the workflow, the rules and their checks, " +
			"the hooks of Claude Code, the plugin of the user, Codex only when polix is registered for it (not implemented), " +
			"and the state. Exits 1 on any failure.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			req.Version, req.Now = build.Current(), time.Now()
			res, err := diagnoseSetup(c.Context(), DiagnoseSetupAdapter{OS: runtime.GOOS, Look: exec.LookPath}, req)
			if err != nil {
				return err
			}
			res.print(c.OutOrStdout())
			if !res.Hook && res.Failed {
				return cli.Exit(1)
			}
			return nil
		},
	}
	cli.DirFlag(c, &req.Dir, ".")
	c.Flags().BoolVar(&req.Hook, "hook", false, "for a session start hook: quiet when fine, never fails, no network")
	c.Flags().BoolVar(&req.Offline, "offline", false, "do not ask for the latest release")
	return c
}

// diagnoseSetup is the shell, from request to response: it reads through the port everything doctor looks at (the
// latest release only for a release build without --hook nor --offline, saving what it saw; inside a repository
// its workflow, rules, checks, base and state; Claude Code only without --hook and with the plugin of this binary), calls the logic once and builds
// the response.
func diagnoseSetup(ctx context.Context, port DiagnoseSetupPort, req DiagnoseSetupRequest) (DiagnoseSetupResponse, error) {
	home := port.Home(ctx)
	exe, exeErr := port.Executable(ctx)
	cache := port.LoadCache(ctx)
	latest, latestErr := cache.Latest, error(nil)
	running, versionErr := version.Parse(req.Version)
	if exeErr == nil && !req.Hook && !req.Offline && versionErr == nil && running.IsRelease() {
		asked, err := port.Latest(ctx)
		latestErr = err
		if err == nil {
			latest = asked
			// The time of the check is only saved when this binary is the latest (Cache.Saw).
			_ = port.SaveCache(ctx, cache.Saw(req.Now, req.Version, asked))
		}
	}
	path, pathErr := port.LookPath(ctx, "polix")
	_, gitErr := port.LookPath(ctx, "git")
	root, gitDir, repoErr := port.OpenRepo(ctx, req.Dir)
	var wf workflow.Workflow
	var found bool
	var wfErr, rulesErr, baseErr, stateErr error
	var rules []rule.Rule
	var invalid []string
	var stateDir string
	missing, runnable := map[string]bool{}, map[string]error{}
	if gitErr == nil && repoErr == nil {
		wf, found, wfErr = port.LoadWorkflow(ctx, root, req.Version)
		rules, invalid, rulesErr = port.LoadRules(ctx, root, wf.RulesPath())
		for _, r := range rules {
			if r.HasCheck() {
				missing[r.Check] = port.Missing(ctx, root, r.Check)
				runnable[r.Check] = port.Runnable(ctx, root, r.Check)
			}
		}
		baseErr = port.ResolveBase(ctx, root, wf.BaseBranches())
		stateDir, stateErr = port.WriteState(ctx, gitDir)
	}
	s := Setup{Home: home, Root: root, Agents: []string{Claude, Codex}, Version: req.Version}
	if err := s.Valid(); err != nil {
		return DiagnoseSetupResponse{}, err
	}
	paths := append(s.ClaudeSettingsFiles(), s.CodexHooksFiles()...)
	files, fileErrs := port.ReadFiles(ctx, append(paths, s.CodexConfigFile(), s.PluginManifest()))
	embedded, err := port.Plugin(ctx)
	if err != nil {
		return DiagnoseSetupResponse{}, err
	}
	var claude []byte
	var claudeErr error
	plugin := Plugin{Manifest: files[s.PluginManifest()], Embedded: embedded, Version: req.Version}
	if err := plugin.Valid(); err != nil {
		return DiagnoseSetupResponse{}, err
	}
	if !req.Hook && s.Home != "" && plugin.Current() {
		claude, claudeErr = port.ClaudePlugins(ctx)
	}
	i := Inspection{Setup: s, Exe: exe, ExeErr: exeErr, Latest: latest, LatestErr: latestErr, Path: path,
		PathErr: pathErr, SamePath: port.SameFile(ctx, path, exe), GitErr: gitErr, RepoErr: repoErr, Workflow: wf,
		Found: found, WorkflowErr: wfErr, Rules: rules, Invalid: invalid, RulesErr: rulesErr, Missing: missing,
		Runnable: runnable, BaseErr: baseErr, Files: files, FileErrs: fileErrs, Embedded: embedded, Claude: claude,
		ClaudeErr: claudeErr, SameFolder: port.SameFile(ctx, ClaudeFolder(claude), s.PluginDir()),
		StateDir: stateDir, StateErr: stateErr}
	if err := i.Valid(); err != nil {
		return DiagnoseSetupResponse{}, err
	}
	out := DiagnoseSetup(i, req.Hook)
	return DiagnoseSetupResponse{Hook: req.Hook, Findings: out.Findings, Failed: out.Failed}, nil
}

// print serializes the response to stdout: one line per finding; with --hook only the problems, in a paragraph
// in Spanish the agent tells the user, and nothing when everything is fine.
func (res DiagnoseSetupResponse) print(stdout io.Writer) {
	if res.Hook {
		var lines []string
		for _, f := range res.Findings {
			if f.Problem() {
				lines = append(lines, fmt.Sprintf("- [%s] %s: %s", f.Level, f.Area, f.Message))
			}
		}
		if len(lines) > 0 {
			fmt.Fprintf(stdout, "polix doctor ha encontrado problemas en este equipo o repositorio; cuéntaselos al usuario y "+
				"ofrécete a arreglarlos (polix doctor da el detalle, y la skill polix:doctor sabe cómo):\n%s\n", strings.Join(lines, "\n"))
		}
		return
	}
	u := cli.NewUI(stdout)
	for _, f := range res.Findings {
		u.Status(f.Level, f.Area, f.Message)
	}
}
