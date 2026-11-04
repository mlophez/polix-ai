package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

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

// updateCmd replaces the running binary with the latest release, or with --version a given one.
//
// With --hook it is meant for the session start hook of the agent: it checks at most once a day, gives up
// quickly without network, never fails (exit 0) and only prints a line when it updated. POLIX_NO_UPDATE=1
// disables it, for machines where the version is pinned by other means.
func updateCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	check := fs.Bool("check", false, "only report whether a newer release exists")
	want := fs.String("version", "", "install this version, older ones included, instead of the latest")
	hook := fs.Bool("hook", false, "for a session start hook: once a day, quiet, never fails")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	msg, err := runUpdate(*check, strings.TrimPrefix(*want, "v"), *hook)
	if *hook {
		if err != nil {
			fmt.Fprintln(stderr, "polix update:", err)
		}
		if msg != "" {
			fmt.Fprintln(stdout, msg)
		}
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "polix update:", err)
		return 1
	}
	fmt.Fprintln(stdout, msg)
	return 0
}

// runUpdate does the update and returns what to tell; in hook mode an empty message means nothing to say.
func runUpdate(check bool, want string, hook bool) (string, error) {
	if os.Getenv("POLIX_NO_UPDATE") != "" {
		if hook {
			return "", nil
		}
		return "", fmt.Errorf("updates are disabled by POLIX_NO_UPDATE")
	}
	exe, err := executable()
	if err != nil {
		return "", err
	}
	update.Cleanup(exe)
	if hook && (!update.IsRelease(Version) || time.Since(update.LoadCache().CheckedAt) < checkEvery) {
		return "", nil
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
			return "", err
		}
		_ = update.SaveCache(update.Cache{CheckedAt: time.Now(), Latest: latest})
		target = latest
		switch {
		case !update.IsRelease(Version) && check:
			return fmt.Sprintf("polix %s is a development build; the latest release is %s", Version, latest), nil
		case !update.IsRelease(Version):
			return "", fmt.Errorf("polix %s is a development build: run polix update --version %s to replace it", Version, latest)
		case update.Compare(Version, latest) >= 0:
			if hook {
				return "", nil
			}
			return fmt.Sprintf("polix %s is up to date", Version), nil
		}
	}
	if check {
		return fmt.Sprintf("polix %s can be updated to %s: run polix update", Version, target), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	bin, err := client.Download(ctx, target)
	if err != nil {
		return "", err
	}
	if err := update.Replace(exe, bin); err != nil {
		return "", err
	}
	if hook {
		return fmt.Sprintf("polix se ha actualizado de %s a %s.", Version, target), nil
	}
	return fmt.Sprintf("polix updated from %s to %s (%s)", Version, target, exe), nil
}
