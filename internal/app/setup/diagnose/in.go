// Package diagnose is the use case DiagnoseSetup, the command `polix doctor`: it checks that the machine and the
// repository are ready for polix and reports a finding per area.
package diagnose

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mlophez/polix-ai/internal/app/setup"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

// Request is what the caller asks of the use case DiagnoseSetup, whatever the transport: the command
// fills it from its flags.
type Request struct {
	Dir     string    // any folder of the repository to check
	Hook    bool      // the session start hook: quiet when fine, never fails, no network
	Offline bool      // do not ask for the latest release
	Version string    // version of the running polix
	Now     time.Time // the time of the call, recorded in the cache when the latest release is asked
}

// Response is what the use case DiagnoseSetup gives back to the caller, in the shape of the
// transport: the command serializes it to the console.
type Response struct {
	Hook     bool            // the response of the session start hook: only the problems, and it never fails
	Findings []setup.Finding // the findings of the report, in order
	Failed   bool            // whether a finding is a failure
}

// Port is what the shell of DiagnoseSetup needs from the outside: the binary, the PATH, the
// releases, the repository with its workflow, rules and state, the files of the agents and the CLI of Claude Code.
// It speaks in domain values, and every method takes the context of the caller.
type Port interface {
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
	// file exists; err tells why it cannot be read or parsed, or is a workflow.InvalidError, and baseErr why none of
	// the base branches of the workflow exists as a branch of the repository.
	LoadWorkflow(ctx context.Context, root, running string) (wf workflow.Workflow, found bool, err, baseErr error)
	// LoadRules returns what the folder dir of root holds: every rule, ordered by id, with the problems of the
	// invalid ones; and, by the check of each rule that has one, relative to root, whether its file does not exist
	// (missing) and why it cannot run as a program (runnable), nil when it can or does not exist.
	LoadRules(ctx context.Context, root, dir string) (loaded rule.Loaded, missing map[string]bool,
		runnable map[string]error)
	// WriteState checks that the folder of the state, below gitDir, can be written, and returns it.
	WriteState(ctx context.Context, gitDir string) (string, error)
	// HooksFiles returns the hooks files of agent (setup.Claude or setup.Codex) of paths that exist, decoded: their
	// hooks, or why they cannot be read or are not JSON. An empty file is not JSON for Claude Code, whose settings
	// they are, and has no hooks for Codex.
	HooksFiles(ctx context.Context, agent string, paths []string) map[string]setup.HooksFile
	// CodexEnabled reports whether the configuration of Codex at path enables the plugin polix.
	CodexEnabled(ctx context.Context, path string) bool
	// Plugin returns the files of the plugin embedded in the binary, by path below polix/, and the plugin written
	// at the path manifest: the version of its manifest, and whether it is the plugin polix (written), false when
	// it does not exist, cannot be read or is another one.
	Plugin(ctx context.Context, manifest string) (embedded map[string][]byte, version string, written bool, err error)
	// ClaudePlugin asks `claude plugin list --json` whether the plugin polix is installed for the user, whether it
	// is enabled, and the folder Claude Code reads it from; setup.ErrNoCLI without the CLI.
	ClaudePlugin(ctx context.Context) (bool, bool, string, error)
}

// repository is what the shell reads through the port about the repository, inside one with git.
type repository struct {
	wf           workflow.Workflow
	found        bool
	wfErr        error
	requirements map[string]error
	rules        []rule.Rule
	invalid      []string
	rulesErr     error
	missing      map[string]bool
	runnable     map[string]error
	baseErr      error
	stateDir     string
	stateErr     error
}

// Command is the command `polix doctor`, the entry of the use case DiagnoseSetup: it turns the flags
// into a Request, injects the adapter with the loaders rules, cache and loadWorkflow as the port, runs the shell and
// serializes the Response to the console.
func Command(rules rule.Load, cache version.Load, loadWorkflow workflow.Load) *cobra.Command {
	var req Request
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check that this machine and repository are ready for polix",
		Long: "Check the binary and its version, the PATH the hooks use, git, the workflow, the rules and their checks, " +
			"the hooks of Claude Code, the plugin of the user, Codex only when polix is registered for it (not implemented), " +
			"and the state. Exits 1 on any failure.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			req.Version, req.Now = build.Current(), time.Now()
			res, err := run(c.Context(), Adapter{OS: runtime.GOOS, Look: exec.LookPath, Rules: rules, Cache: cache,
				Workflow: loadWorkflow}, req)
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

// run is the shell, from request to response: it reads through the port everything doctor looks at (the
// latest release only for a release build without --hook nor --offline, saving what it saw; inside a repository
// its workflow, the programs it requires, rules, checks, base and state; the hooks files and the configuration of
// the agents, decoded; the plugin; Claude Code only without --hook and with the plugin of this binary), calls the
// logic once and builds the response.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	home := port.Home(ctx)
	exe, exeErr := port.Executable(ctx)
	latest, latestErr := releases(ctx, port, req, exeErr)
	path, pathErr := port.LookPath(ctx, "polix")
	_, gitErr := port.LookPath(ctx, "git")
	root, gitDir, repoErr := port.OpenRepo(ctx, req.Dir)
	var repo repository
	if gitErr == nil && repoErr == nil {
		repo = inspectRepository(ctx, port, req, root, gitDir)
	}
	s := setup.Setup{Home: home, Root: root, Agents: []string{setup.Claude, setup.Codex}, Version: req.Version}
	if err := s.Valid(); err != nil {
		return Response{}, err
	}
	claudeHooks := port.HooksFiles(ctx, setup.Claude, s.ClaudeSettingsFiles())
	codexHooks := port.HooksFiles(ctx, setup.Codex, s.CodexHooksFiles())
	codexEnabled := port.CodexEnabled(ctx, s.CodexConfigFile())
	plugin, err := readPlugin(ctx, port, req, s)
	if err != nil {
		return Response{}, err
	}
	installed, enabled, folder, claudeErr := askClaude(ctx, port, req, s, plugin)
	i := setup.Inspection{Setup: s, Exe: exe, ExeErr: exeErr, Latest: latest, LatestErr: latestErr, Path: path,
		PathErr: pathErr, SamePath: port.SameFile(ctx, path, exe), GitErr: gitErr, RepoErr: repoErr,
		Workflow: repo.wf, Found: repo.found, WorkflowErr: repo.wfErr, Requirements: repo.requirements,
		Rules: repo.rules, Invalid: repo.invalid, RulesErr: repo.rulesErr, Missing: repo.missing,
		Runnable: repo.runnable, BaseErr: repo.baseErr,
		ClaudeHooks: claudeHooks, CodexHooks: codexHooks, CodexEnabled: codexEnabled, Plugin: plugin,
		ClaudeInstalled: installed, ClaudeEnabled: enabled, ClaudeFolder: folder, ClaudeErr: claudeErr,
		SameFolder: port.SameFile(ctx, folder, s.PluginDir()), StateDir: repo.stateDir, StateErr: repo.stateErr}
	if err := i.Valid(); err != nil {
		return Response{}, err
	}
	out := Decide(i, req.Hook)
	return Response{Hook: req.Hook, Findings: out.Findings, Failed: out.Failed}, nil
}

// releases returns the latest release, known from the cache, and asked to the releases only by a release build
// that found itself, without --hook nor --offline: then what it saw is saved, and the error of asking returned.
func releases(ctx context.Context, port Port, req Request, exeErr error) (string, error) {
	cache := port.LoadCache(ctx)
	running, versionErr := versionText(req.Version).parse()
	if exeErr == nil && !req.Hook && !req.Offline && versionErr == nil && running.IsRelease() {
		asked, err := port.Latest(ctx)
		if err != nil {
			return cache.Latest, err
		}
		// The time of the check is only saved when this binary is the latest (Cache.Saw).
		_ = port.SaveCache(ctx, cache.Saw(req.Now, req.Version, asked))
		return asked, nil
	}
	return cache.Latest, nil
}

// inspectRepository reads the repository of root: its workflow, why the PATH does not run each program it requires
// (workflow.Programs, git included; only for a workflow that can be used), its rules, whether each check exists and can run, whether a base branch
// exists, and whether the state below gitDir can be written.
func inspectRepository(ctx context.Context, port Port, req Request, root, gitDir string) repository {
	wf, found, wfErr, baseErr := port.LoadWorkflow(ctx, root, req.Version)
	r := repository{wf: wf, found: found, wfErr: wfErr, baseErr: baseErr}
	if wfErr == nil {
		r.requirements = lookRequirements(ctx, port, wf.Programs())
	}
	loaded, missing, runnable := port.LoadRules(ctx, root, r.wf.RulesPath())
	r.rules, r.invalid, r.rulesErr, r.missing, r.runnable = loaded.Rules, loaded.Problems, loaded.Err, missing, runnable
	r.stateDir, r.stateErr = port.WriteState(ctx, gitDir)
	return r
}

// lookRequirements looks each program up on the PATH, the one the hooks give polix, and returns by program why the
// PATH does not run it, nil when it does.
func lookRequirements(ctx context.Context, port Port, programs []string) map[string]error {
	why := make(map[string]error, len(programs))
	for _, p := range programs {
		_, why[p] = port.LookPath(ctx, p)
	}
	return why
}

// readPlugin returns the plugin embedded in the binary against the one written in the local marketplace of s.
func readPlugin(ctx context.Context, port Port, req Request, s setup.Setup) (setup.Plugin, error) {
	embedded, manifest, written, err := port.Plugin(ctx, s.PluginManifest())
	if err != nil {
		return setup.Plugin{}, err
	}
	p := setup.Plugin{Written: written, Manifest: manifest, Embedded: embedded, Version: req.Version}
	if err := p.Valid(); err != nil {
		return setup.Plugin{}, err
	}
	return p, nil
}

// askClaude asks Claude Code for the plugin polix (installed, enabled, the folder it reads it from), only without
// --hook, with a home, and when the plugin written is the one of this binary.
func askClaude(ctx context.Context, port Port, req Request, s setup.Setup, p setup.Plugin) (bool, bool, string, error) {
	if req.Hook || s.Home == "" || !p.Current() {
		return false, false, "", nil
	}
	return port.ClaudePlugin(ctx)
}

// print serializes the response to stdout: one line per finding; with --hook only the problems, in a paragraph
// the agent tells the user, and nothing when everything is fine.
func (res Response) print(stdout io.Writer) {
	if res.Hook {
		var lines []string
		for _, f := range res.Findings {
			if f.Problem() {
				lines = append(lines, fmt.Sprintf("- [%s] %s: %s", f.Level, f.Area, f.Message))
			}
		}
		if len(lines) > 0 {
			fmt.Fprintf(stdout, "polix doctor found problems on this machine or repository; tell the user about them and that "+
				"they can fix them by running /polix:doctor (polix doctor gives the detail):\n%s\n", strings.Join(lines, "\n"))
		}
		return
	}
	u := cli.NewUI(stdout)
	for _, f := range res.Findings {
		u.Status(f.Level, f.Area, f.Message)
	}
}
