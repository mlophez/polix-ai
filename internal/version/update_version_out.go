package version

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/file"
	"github.com/mlophez/polix-ai/pkg/git"
	"github.com/mlophez/polix-ai/pkg/github"
	"github.com/mlophez/polix-ai/pkg/proc"
)

// UpdateVersionAdapter implements UpdateVersionPort on the real disk, network and processes. The entry fills it
// and passes it as the port; there is no constructor.
type UpdateVersionAdapter struct {
	Dir  string // the working folder, whose repository holds the workflow and where refresh and doctor run
	Hook bool   // the session start hook: the latest release is asked with a short timeout
}

const (
	// updateVersionReleases is the releases page of polix; POLIX_RELEASES_URL overrides it (a mirror, or tests).
	updateVersionReleases = "https://github.com/mlophez/polix-ai/releases"
	// updateVersionAgent is the User-Agent of the requests to the releases.
	updateVersionAgent = "polix-update"
	// updateVersionTemp is the prefix of the temporary files written next to the binary.
	updateVersionTemp = ".polix-update-"
	// updateVersionHookTimeout bounds the question for the latest release of the hook, which gives up quickly.
	updateVersionHookTimeout = 5 * time.Second
	// updateVersionTimeout bounds the question for the latest release and the download, which a person waits for.
	updateVersionTimeout = 2 * time.Minute
)

// Executable returns the real path of the running binary, removing the binary a previous update on Windows moved
// aside.
func (a UpdateVersionAdapter) Executable(_ context.Context) (string, error) {
	exe, err := github.Executable()
	if err != nil {
		return "", err
	}
	github.Cleanup(exe)
	return exe, nil
}

// LoadCache reads the cache, empty when there is none or it cannot be read.
func (a UpdateVersionAdapter) LoadCache(_ context.Context) Cache {
	file, err := a.cacheFile()
	if err != nil {
		return Cache{}
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return Cache{}
	}
	return ParseCache(data)
}

// SaveCache writes the cache, creating its folder.
func (a UpdateVersionAdapter) SaveCache(_ context.Context, c Cache) error {
	file, err := a.cacheFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	data, err := c.Encode()
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o644)
}

// WorkflowRejected reads the polix.yaml of the repository of the working folder for the polix of version running
// and returns why it rejects it: it cannot be read, it is not valid or it requires a newer polix. Outside a
// repository, or without polix.yaml, there is nothing to reject.
func (a UpdateVersionAdapter) WorkflowRejected(ctx context.Context, running string) error {
	repo, err := git.Open(ctx, a.Dir)
	if err != nil {
		return nil //nolint:nilerr // outside a repository there is no workflow to reject
	}
	data, existed, err := file.Read(filepath.Join(repo.Root, workflow.File))
	if err != nil || !existed {
		return err
	}
	_, err = workflow.Parse(data, running)
	return err
}

// Latest asks the releases page for the latest release, within the timeout of the hook or of a person.
func (a UpdateVersionAdapter) Latest(ctx context.Context) (string, error) {
	timeout := updateVersionTimeout
	if a.Hook {
		timeout = updateVersionHookTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return a.client().Latest(ctx)
}

// Install checks that exe can be replaced before downloading anything, downloads the binary of version for this
// system checked against the checksums of its release, and replaces exe with it. A replace that fails is an
// ErrReplace.
func (a UpdateVersionAdapter) Install(ctx context.Context, exe, version string) error {
	if err := github.Writable(exe, updateVersionTemp); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, updateVersionTimeout)
	defer cancel()
	bin, err := a.client().Download(ctx, version, github.AssetName("polix", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		return err
	}
	if err := github.Replace(exe, updateVersionTemp, bin); err != nil {
		return fmt.Errorf("%w: %w", ErrReplace, err)
	}
	return nil
}

// Refresh runs the binary at exe to refresh the plugin: with hook `__refresh --quiet`, which only refreshes the
// plugin already installed; else `install --user --yes --quiet`, which installs it when missing, and then
// `__refresh --hooks`, which also refreshes the hooks the repository registers.
func (a UpdateVersionAdapter) Refresh(ctx context.Context, exe string, hook bool) (string, string, error) {
	if hook {
		return a.run(ctx, exe, "__refresh", "--quiet")
	}
	stdout, stderr, err := a.run(ctx, exe, "install", "--user", "--yes", "--quiet")
	if err != nil {
		return stdout, stderr, err
	}
	out, errOut, err := a.run(ctx, exe, "__refresh", "--hooks")
	return stdout + out, stderr + errOut, err
}

// Doctor runs `doctor --hook` with the binary at exe.
func (a UpdateVersionAdapter) Doctor(ctx context.Context, exe string) (string, string, error) {
	return a.run(ctx, exe, "doctor", "--hook")
}

// client is the client of the releases page, POLIX_RELEASES_URL or else the one of polix.
func (a UpdateVersionAdapter) client() *github.Client {
	releases := os.Getenv("POLIX_RELEASES_URL")
	if releases == "" {
		releases = updateVersionReleases
	}
	return github.New(releases, updateVersionAgent)
}

// cacheFile is <user cache dir>/polix/update.json; POLIX_CACHE_DIR overrides the folder.
func (a UpdateVersionAdapter) cacheFile() (string, error) {
	return file.CacheFile("POLIX_CACHE_DIR", "polix", "update.json")
}

// run runs exe with args from the working folder and returns what it printed to stdout and stderr.
func (a UpdateVersionAdapter) run(ctx context.Context, exe string, args ...string) (string, string, error) {
	return proc.Output(ctx, a.Dir, append([]string{exe}, args...), 0)
}
