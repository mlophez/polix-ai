package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/gitx"
	"github.com/mlophez/polix-ai/internal/setup"
	"github.com/mlophez/polix-ai/internal/update"
	"github.com/mlophez/polix-ai/internal/version"
)

// Version of polix. Releases set it at build time with -ldflags "-X github.com/mlophez/polix-ai/internal/cli.Version=...";
// `go install ...@vX.Y.Z` gets it from the build info; any other build is "dev".
var Version = "dev"

// releasedModule matches a module version of a tag, not a pseudo-version of an untagged commit.
var releasedModule = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`)

var pseudoVersion = regexp.MustCompile(`\d{14}-[0-9a-f]{12}`)

func init() {
	if Version != "dev" {
		return
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; releasedModule.MatchString(v) && !pseudoVersion.MatchString(v) {
			Version = strings.TrimPrefix(v, "v")
		}
	}
}

// Timeouts of the update: the hook asks quickly and gives up, a person waits for the download.
const (
	hookLatestTimeout = 5 * time.Second
	downloadTimeout   = 2 * time.Minute
	// checkEvery spaces the checks of the session start hook (see hookShouldCheck).
	checkEvery = time.Hour
	// retryAfter spaces the attempts of the session start hook after an update that failed.
	retryAfter = 15 * time.Minute
)

func updateCmd() *cobra.Command {
	var check, hook bool
	var want string
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
			return exit(doUpdate(check, strings.TrimPrefix(want, "v"), hook, c.OutOrStdout(), c.ErrOrStderr()))
		},
	}
	c.Flags().BoolVar(&check, "check", false, "only report, without installing: whether a newer release exists, or with --version how that version compares to this binary")
	c.Flags().StringVar(&want, "version", "", "install this version, older ones included, instead of the latest")
	c.Flags().BoolVar(&hook, "hook", false, "for the session start hook: once an hour, quiet, never fails, then doctor --hook")
	return c
}

// doUpdate replaces the running binary and refreshes the plugin. With hook it never fails and ends with the
// doctor of the binary it leaves: one command does both, since the hooks of an event run in parallel and doctor
// must see the binary the update left, and a single command needs no separator that only some shells know.
func doUpdate(check bool, want string, hook bool, stdout, stderr io.Writer) int {
	msg, exe, err := runUpdate(check, want, hook)
	u, ue := newUI(stdout), newUI(stderr)
	if err != nil {
		if hook {
			fmt.Fprintln(stderr, "polix update:", err)
		} else {
			ue.fail("polix update: %v", err)
		}
	}
	if msg != "" {
		if hook {
			fmt.Fprintln(stdout, msg)
		} else {
			u.ok("%s", msg)
		}
	}
	// After a replace that failed another process may have updated the binary: this one must not rewrite the
	// plugin of the new one with its own.
	if !check && (err == nil || hook && !errors.Is(err, errReplace)) && os.Getenv("POLIX_NO_UPDATE") == "" {
		if serr := refreshAfterUpdate(exe, hook, stdout, stderr); serr != nil {
			if hook {
				fmt.Fprintln(stderr, "polix update:", serr)
			} else {
				ue.warn("polix update: the plugin was not refreshed: %v", serr)
			}
		}
	}
	if hook {
		hookDoctor(exe, stdout, stderr)
		return 0
	}
	if err != nil {
		return 1
	}
	return 0
}

// hookDoctor runs doctor --hook on the working folder with the binary the update left: the new one when it was
// replaced, so it checks itself, else this one.
func hookDoctor(newExe string, stdout, stderr io.Writer) {
	if newExe == "" {
		doctor(".", true, false, stdout)
		return
	}
	cmd := exec.Command(newExe, "doctor", "--hook")
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(stderr, "polix doctor:", err)
	}
}

// workflowAhead reports whether this binary rejects the workflow of the repository of the working folder in a way
// a newer polix may not (see mayNeedNewer), so the session start hook looks for one at once.
func workflowAhead() bool {
	repo, err := gitx.Open(".")
	if err != nil {
		return false
	}
	_, err = loadValid(repo.Root)
	return err != nil && mayNeedNewer(err.Error())
}

// within reports whether t is less than d ago. A time in the future (saved with a clock that was ahead) is not
// within: it must not stop the checks until the clock reaches it.
func within(t time.Time, d time.Duration) bool {
	since := time.Since(t)
	return since >= 0 && since < d
}

// hookShouldCheck decides whether the session start hook asks for the latest release. A release build asks once
// an hour, at once when the cache already knows a newer release (a polix doctor or --check saw it) or this binary
// rejects the workflow, and never within retryAfter of an update that failed, so a failure that repeats does not
// download the binary at every session start. A development build never asks.
func hookShouldCheck(c update.Cache) bool {
	switch {
	case !version.IsRelease(Version), within(c.FailedAt, retryAfter):
		return false
	case !within(c.CheckedAt, checkEvery):
		return true
	case version.IsRelease(c.Latest) && version.Compare(Version, c.Latest) < 0:
		return true
	}
	return workflowAhead()
}

// errReplace marks an update whose binary could not take the place of the running one.
var errReplace = errors.New("cannot replace the binary")

// refreshAfterUpdate brings the plugin in line with the binary. When the binary was replaced, the new one does it,
// since the plugin is embedded in it; otherwise this one. A person also gets the plugin of the user installed when
// it is missing, and the hooks the repository of the working folder registers refreshed; the session start hook
// only refreshes the plugin already installed, and never touches the files of the repository.
func refreshAfterUpdate(newExe string, hook bool, stdout, stderr io.Writer) error {
	if newExe != "" {
		run := func(args ...string) error {
			cmd := exec.Command(newExe, args...)
			cmd.Stdout, cmd.Stderr = stdout, stderr
			return cmd.Run()
		}
		if hook {
			return run("__refresh", "--quiet")
		}
		if err := run("install", "--user", "--yes", "--quiet"); err != nil {
			return err
		}
		return run("__refresh", "--hooks")
	}
	if !hook {
		o, err := (&installFlags{user: true}).options()
		if err != nil {
			return err
		}
		agents, err := resolveAgents(nil, o)
		if err != nil {
			return err
		}
		changes, err := setup.Install(o, agents)
		printChanges(newUI(stdout), changes)
		if err != nil {
			return err
		}
	}
	return refreshInstalled(".", stdout, hook, !hook)
}

// refreshCmd is the hidden command the new binary runs after an update: it refreshes the plugin when polix
// installed it and, with --hooks, the hooks of polix of the repository of the working folder.
func refreshCmd() *cobra.Command {
	var quiet, hooks bool
	c := &cobra.Command{
		Use:    "__refresh",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return refreshInstalled(".", c.OutOrStdout(), quiet, hooks)
		},
	}
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "one line in Spanish when something changed, for the session start hook")
	c.Flags().BoolVar(&hooks, "hooks", false, "also refresh the hooks of polix of the repository")
	return c
}

// runUpdate does the update and returns what to tell; in hook mode an empty message means nothing to say.
func runUpdate(check bool, want string, hook bool) (msg, replaced string, err error) {
	if os.Getenv("POLIX_NO_UPDATE") != "" {
		if hook {
			return "", "", nil
		}
		return "", "", fmt.Errorf("updates are disabled by POLIX_NO_UPDATE")
	}
	exe, err := executable()
	if err != nil {
		return "", "", err
	}
	update.Cleanup(exe)
	cache := update.LoadCache()
	if hook && !hookShouldCheck(cache) {
		return "", "", nil
	}
	// fail records a failure of the hook, retried after retryAfter.
	fail := func(err error) (string, string, error) {
		if hook {
			cache.FailedAt = time.Now()
			_ = update.SaveCache(cache)
		}
		return "", "", err
	}
	client := update.New()
	target := want
	if target == "" {
		timeout := downloadTimeout
		if hook {
			timeout = hookLatestTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		latest, err := client.Latest(ctx)
		cancel()
		if err != nil {
			return fail(err)
		}
		// The time of the check is only saved once this binary is the latest: a check that leaves it behind (a
		// failed download, --check) is repeated at the next session start.
		cache.Latest = latest
		_ = update.SaveCache(cache)
		target = latest
		switch {
		case !version.IsRelease(Version) && check:
			return fmt.Sprintf("polix %s is a development build; the latest release is %s", Version, latest), "", nil
		case !version.IsRelease(Version):
			return "", "", fmt.Errorf("polix %s is a development build: run polix update --version %s to replace it", Version, latest)
		case version.Compare(Version, latest) >= 0:
			_ = update.SaveCache(update.Cache{CheckedAt: time.Now(), Latest: latest})
			if hook {
				return "", "", nil
			}
			return fmt.Sprintf("polix %s is up to date", Version), "", nil
		}
	}
	if check && want != "" {
		switch c := version.Compare(want, Version); {
		case !version.IsRelease(Version):
			return fmt.Sprintf("polix %s is a development build: run polix update --version %s to install %s", Version, want, want), "", nil
		case c > 0:
			return fmt.Sprintf("polix %s is newer than this binary (%s): run polix update --version %s", want, Version, want), "", nil
		case c == 0:
			return fmt.Sprintf("polix %s is already this binary", want), "", nil
		default:
			return fmt.Sprintf("polix %s is older than this binary (%s): installing it would be a downgrade", want, Version), "", nil
		}
	}
	if check {
		return fmt.Sprintf("polix %s can be updated to %s: run polix update", Version, target), "", nil
	}
	// A binary that cannot be replaced (a folder of the system) fails before the download.
	if err := update.Writable(exe); err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	bin, err := client.Download(ctx, target)
	if err != nil {
		return fail(err)
	}
	if err := update.Replace(exe, bin); err != nil {
		return fail(fmt.Errorf("%w: %w", errReplace, err))
	}
	if want == "" {
		_ = update.SaveCache(update.Cache{CheckedAt: time.Now(), Latest: target})
	}
	if hook {
		return fmt.Sprintf("polix se ha actualizado de %s a %s.", Version, target), exe, nil
	}
	return fmt.Sprintf("polix updated from %s to %s (%s)", Version, target, exe), exe, nil
}
