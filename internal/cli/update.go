package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/setup"
	"github.com/mlophez/polix-ai/internal/update"
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
	// checkEvery spaces the checks of the session start hook.
	checkEvery = 24 * time.Hour
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
			"With --hook it is meant for the session start hook of the agent: it checks at most once a day, gives up " +
			"quickly without network, never fails and only prints a line when it changed something. " +
			"POLIX_NO_UPDATE=1 disables it, for machines where the version is pinned by other means.",
		Example: "  polix update\n  polix update --check\n  polix update --version 0.2.0",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return exit(doUpdate(check, strings.TrimPrefix(want, "v"), hook, c.OutOrStdout(), c.ErrOrStderr()))
		},
	}
	c.Flags().BoolVar(&check, "check", false, "only report whether a newer release exists")
	c.Flags().StringVar(&want, "version", "", "install this version, older ones included, instead of the latest")
	c.Flags().BoolVar(&hook, "hook", false, "for a session start hook: once a day, quiet, never fails")
	return c
}

// doUpdate replaces the running binary and refreshes the plugin. With hook it never fails.
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
	if !check && (err == nil || hook) && os.Getenv("POLIX_NO_UPDATE") == "" {
		if serr := refreshAfterUpdate(exe, hook, stdout, stderr); serr != nil {
			if hook {
				fmt.Fprintln(stderr, "polix update:", serr)
			} else {
				ue.warn("polix update: the plugin was not refreshed: %v", serr)
			}
		}
	}
	if err != nil && !hook {
		return 1
	}
	return 0
}

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
	if hook && (!update.IsRelease(Version) || time.Since(update.LoadCache().CheckedAt) < checkEvery) {
		return "", "", nil
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
			return "", "", err
		}
		_ = update.SaveCache(update.Cache{CheckedAt: time.Now(), Latest: latest})
		target = latest
		switch {
		case !update.IsRelease(Version) && check:
			return fmt.Sprintf("polix %s is a development build; the latest release is %s", Version, latest), "", nil
		case !update.IsRelease(Version):
			return "", "", fmt.Errorf("polix %s is a development build: run polix update --version %s to replace it", Version, latest)
		case update.Compare(Version, latest) >= 0:
			if hook {
				return "", "", nil
			}
			return fmt.Sprintf("polix %s is up to date", Version), "", nil
		}
	}
	if check {
		return fmt.Sprintf("polix %s can be updated to %s: run polix update", Version, target), "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	bin, err := client.Download(ctx, target)
	if err != nil {
		return "", "", err
	}
	if err := update.Replace(exe, bin); err != nil {
		return "", "", err
	}
	if hook {
		return fmt.Sprintf("polix se ha actualizado de %s a %s.", Version, target), exe, nil
	}
	return fmt.Sprintf("polix updated from %s to %s (%s)", Version, target, exe), exe, nil
}
