// Package update is the use case UpdateVersion, the command `polix update`: it replaces the running binary with a
// release of polix, checked against the checksums of the release, and refreshes the plugin with the new binary.
package update

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

// Request is what the caller asks of the use case UpdateVersion, whatever the transport: the command
// fills it from its flags and the environment.
type Request struct {
	Check    bool      // only report, without installing
	Want     string    // the version to install, without its leading v; empty for the latest release
	Hook     bool      // the session start hook: once an hour, quiet, never fails, then doctor --hook
	NoUpdate bool      // POLIX_NO_UPDATE is set: updates are disabled
	Version  string    // version of the running polix
	Now      time.Time // the time of the call, against which the cache is read and written
}

// Response is what the use case UpdateVersion gives back to the caller, in the shape of the
// transport: the command serializes it to the console.
type Response struct {
	Hook          bool   // the response of the session start hook: plain lines, and it never fails
	Failure       string // why the update failed; empty when it did not
	Message       string // what the update tells; empty for nothing to say
	RefreshOut    string // what the refresh of the plugin printed to stdout
	RefreshErr    string // what the refresh of the plugin printed to stderr
	Warning       string // why the refresh of the plugin failed; empty when it did not
	DoctorOut     string // what doctor --hook printed to stdout
	DoctorErr     string // what doctor --hook printed to stderr
	DoctorFailure string // why doctor --hook failed; empty when it did not
}

// Port is what the shell of UpdateVersion needs from the outside: the running binary, the cache, the
// workflow of the repository, the releases and the new binary. It speaks in domain values, and every method takes
// the context of the caller.
type Port interface {
	// Executable returns the real path of the running binary, removing the one a previous update moved aside.
	Executable(ctx context.Context) (string, error)
	// LoadCache returns the cache of the checks, empty when there is none.
	LoadCache(ctx context.Context) version.Cache
	// SaveCache writes the cache.
	SaveCache(ctx context.Context, c version.Cache) error
	// WorkflowRejected returns why the polix of version running rejects the polix.yaml of the repository of the
	// working folder; nil when it accepts it, or there is none.
	WorkflowRejected(ctx context.Context, running string) error
	// Latest returns the version of the latest release.
	Latest(ctx context.Context) (string, error)
	// Install downloads the binary of release for this system, checks it against the checksums of its release and
	// installs it at exe.
	Install(ctx context.Context, exe, release string) error
	// Refresh runs the binary at exe to refresh the plugin (with hook, only the one installed) and returns what it
	// printed to stdout and stderr.
	Refresh(ctx context.Context, exe string, hook bool) (string, string, error)
	// Doctor runs doctor --hook with the binary at exe and returns what it printed to stdout and stderr.
	Doctor(ctx context.Context, exe string) (string, string, error)
}

// Command is the command `polix update`, the entry of the use case UpdateVersion: it turns the flags
// and POLIX_NO_UPDATE into an Request and validates them, injects the adapter with the loaders cache and
// loadWorkflow as the port, runs the shell and serializes the Response to the console.
func Command(cache version.Load, loadWorkflow workflow.Load) *cobra.Command {
	var req Request
	c := &cobra.Command{
		Use:   "update",
		Short: "Replace the binary with the latest release and refresh the plugin",
		Long: "Download the latest release (or --version V, older ones included), check it against checksums.txt and " +
			"replace the running binary atomically. Then refresh, with the new binary, the plugin of the user (installed " +
			"when missing) and, inside a repository that registers them, the hooks of polix.\n\n" +
			"With --hook it is the session start hook of the agent: it checks at most once an hour (at once when a newer " +
			"release is already known or this binary rejects the polix.yaml of the repository, which may need a newer " +
			"polix; 15 minutes after a failed update), gives up quickly without network, never fails, only prints a " +
			"line when it changed something and then runs polix doctor --hook with the binary it leaves.\n\n" +
			"POLIX_NO_UPDATE=1 disables the update, for machines where the version is pinned by other means; an older " +
			"version installed with --version is updated again by the hook unless it is set.",
		Example: "  polix update\n  polix update --check\n  polix update --version 0.2.0",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			req.Want = strings.TrimPrefix(req.Want, "v")
			req.NoUpdate = os.Getenv("POLIX_NO_UPDATE") != ""
			req.Version, req.Now = build.Current(), time.Now()
			res, err := run(c.Context(), Adapter{Dir: ".", Hook: req.Hook, Cache: cache, Workflow: loadWorkflow}, req)
			if err != nil {
				cli.NewUI(c.ErrOrStderr()).Fail("polix update: %v", err)
				return cli.Exit(1)
			}
			res.print(c.OutOrStdout(), c.ErrOrStderr())
			if !res.Hook && res.Failure != "" {
				return cli.Exit(1)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&req.Check, "check", false, "only report, without installing: whether a newer release exists, or with --version how that version compares to this binary")
	c.Flags().StringVar(&req.Want, "version", "", "install this version, older ones included, instead of the latest")
	c.Flags().BoolVar(&req.Hook, "hook", false, "for the session start hook: once an hour, quiet, never fails, then doctor --hook")
	return c
}

// run is the shell, from request to response: it reads through the port what the logic needs (the
// binary, the cache, with --hook the workflow, and the latest release when the update goes on for it), calls the
// logic once, carries out its output through the port (install, refresh, and with --hook doctor) and builds the
// response. Every failure goes to the response: the session start hook never fails.
//
//nolint:unparam // every shell returns (response, error), as docs/architecture.md says, even when it cannot fail
func run(ctx context.Context, port Port, req Request) (Response, error) {
	res := Response{Hook: req.Hook}
	exe, err := port.Executable(ctx)
	if err != nil {
		res.Failure = err.Error()
		return res, nil //nolint:nilerr // every failure goes to the response: the session start hook never fails
	}
	u := version.Update{Running: req.Version, Want: req.Want, Exe: exe, Check: req.Check, Hook: req.Hook,
		Disabled: req.NoUpdate}
	if err := u.Valid(); err != nil {
		res.Failure = err.Error()
		return res, nil //nolint:nilerr // every failure goes to the response: the session start hook never fails
	}
	cache := port.LoadCache(ctx)
	var rejected error
	if req.Hook && !req.NoUpdate {
		rejected = port.WorkflowRejected(ctx, req.Version)
	}
	// due is taken before Saw changes the cache.
	due := cache.ShouldCheck(req.Now, req.Version, rejected)
	cache, latest, err := askLatest(ctx, port, req, cache, rejected)
	out := Decide(u, due, latest, err)
	if out.Record(err) {
		_ = port.SaveCache(ctx, cache.Fail(req.Now))
	}
	res.Failure, res.Message = out.Failure, out.Message
	res, installErr := install(ctx, port, req, out, exe, cache, res)
	if out.RefreshAfter(installErr) {
		res.RefreshOut, res.RefreshErr, err = port.Refresh(ctx, exe, req.Hook)
		if err != nil {
			res.Warning = err.Error()
		}
	}
	// The doctor of the hook is the one of the binary the update left: the hooks of an event run in parallel, and
	// doctor must see the binary the update left.
	if req.Hook {
		res = doctor(ctx, port, exe, res)
	}
	return res, nil
}

// askLatest asks the latest release through the port when the update goes on for it: without POLIX_NO_UPDATE nor
// --version, and with --hook only when the cache is due (the condition asks the domain, since GOARCH-37 lets a
// condition of the shell name only the request, errors and the domain, never a local bool). What it saw is saved
// in the cache, and failing to write it only means asking again. It returns the cache, the latest release and the
// error of asking it.
func askLatest(ctx context.Context, port Port, req Request, cache version.Cache,
	rejected error) (version.Cache, string, error) {
	if !req.NoUpdate && req.Want == "" && (!req.Hook || cache.ShouldCheck(req.Now, req.Version, rejected)) {
		latest, err := port.Latest(ctx)
		if err == nil {
			cache = cache.Saw(req.Now, req.Version, latest)
			_ = port.SaveCache(ctx, cache)
		}
		return cache, latest, err
	}
	return cache, "", nil
}

// install installs through the port the release the output names, if any, over the binary at exe, and notes in
// res its failure or what it tells. The cache records a failure, or the release installed when it is the latest.
// It returns res and the error of the install, nil without one.
func install(ctx context.Context, port Port, req Request, out Output, exe string, cache version.Cache,
	res Response) (Response, error) {
	if out.Install == "" {
		return res, nil
	}
	err := port.Install(ctx, exe, out.Install)
	if err != nil {
		res.Failure = err.Error()
	}
	if out.Record(err) {
		_ = port.SaveCache(ctx, cache.Fail(req.Now))
	}
	if err == nil {
		res.Message = out.Installed
	}
	if err == nil && out.Remember {
		_ = port.SaveCache(ctx, cache.Saw(req.Now, out.Install, out.Install))
	}
	return res, err
}

// doctor runs doctor --hook through the port with the binary at exe and notes in res what it printed and its
// failure.
func doctor(ctx context.Context, port Port, exe string, res Response) Response {
	var err error
	res.DoctorOut, res.DoctorErr, err = port.Doctor(ctx, exe)
	if err != nil {
		res.DoctorFailure = err.Error()
	}
	return res
}

// print serializes the response to the console in the order things happened: the outcome of the update, what the
// refresh printed and its failure, and what doctor printed and its failure. With --hook the lines are plain, for
// the agent to read.
func (res Response) print(stdout, stderr io.Writer) {
	switch {
	case res.Hook && res.Failure != "":
		fmt.Fprintln(stderr, "polix update:", res.Failure)
	case res.Failure != "":
		cli.NewUI(stderr).Fail("polix update: %s", res.Failure)
	}
	switch {
	case res.Hook && res.Message != "":
		fmt.Fprintln(stdout, res.Message)
	case res.Message != "":
		cli.NewUI(stdout).OK("%s", res.Message)
	}
	_, _ = io.WriteString(stdout, res.RefreshOut)
	_, _ = io.WriteString(stderr, res.RefreshErr)
	switch {
	case res.Hook && res.Warning != "":
		fmt.Fprintln(stderr, "polix update:", res.Warning)
	case res.Warning != "":
		cli.NewUI(stderr).Warn("polix update: the plugin was not refreshed: %s", res.Warning)
	}
	_, _ = io.WriteString(stdout, res.DoctorOut)
	_, _ = io.WriteString(stderr, res.DoctorErr)
	if res.DoctorFailure != "" {
		fmt.Fprintln(stderr, "polix doctor:", res.DoctorFailure)
	}
}
