package update

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/github"
	"github.com/mlophez/polix-ai/internal/platform/proc"
)

// Adapter implements Port on the real disk, network and processes. The entry fills it
// and passes it as the port; there is no constructor.
type Adapter struct {
	Dir      string        // the working folder, whose repository holds the workflow and where refresh and doctor run
	Hook     bool          // the session start hook: the latest release is asked with a short timeout
	Cache    version.Load  // the loader of the cache (internal/app/version/load), which cmd/polix builds once
	Workflow workflow.Load // the loader of polix.yaml (internal/app/workflow/load), which cmd/polix builds once too
}

// cacheJSON is the shape of the file of the cache, with the fields of version.Cache in its order, so one converts to
// it; LoadVersion (internal/app/version/load) decodes the same shape.
type cacheJSON struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
	FailedAt  time.Time `json:"failed_at,omitzero"`
}

const (
	// releasesPage is the releases page of polix; POLIX_RELEASES_URL overrides it (a mirror, or tests).
	releasesPage = "https://github.com/mlophez/polix-ai/releases"
	// agent is the User-Agent of the requests to the releases.
	agent = "polix-update"
	// temp is the prefix of the temporary files written next to the binary.
	temp = ".polix-update-"
	// hookTimeout bounds the question for the latest release of the hook, which gives up quickly.
	hookTimeout = 5 * time.Second
	// timeout bounds the question for the latest release and the download, which a person waits for.
	timeout = 2 * time.Minute
)

// Executable returns the real path of the running binary, removing the binary a previous update on Windows moved
// aside.
func (a Adapter) Executable(_ context.Context) (string, error) {
	exe, err := github.Executable()
	if err != nil {
		return "", err
	}
	github.Cleanup(exe)
	return exe, nil
}

// LoadCache loads the cache through the loader of the cache, empty when there is none or it cannot be read.
func (a Adapter) LoadCache(ctx context.Context) version.Cache {
	file, err := a.cacheFile()
	if err != nil {
		return version.Cache{}
	}
	return a.Cache(ctx, file).Cache
}

// SaveCache writes the cache, encoded with cacheJSON, creating its folder.
func (a Adapter) SaveCache(_ context.Context, c version.Cache) error {
	file, err := a.cacheFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cacheJSON(c))
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o644)
}

// WorkflowRejected loads the polix.yaml of the repository of the working folder for the polix of version running,
// through the loader of polix.yaml, and returns why it rejects it: it cannot be read, it is not valid or it requires
// a newer polix. Outside a repository, or without polix.yaml, there is nothing to reject.
func (a Adapter) WorkflowRejected(ctx context.Context, running string) error {
	repo, err := git.Open(ctx, a.Dir)
	if err != nil {
		return nil //nolint:nilerr // outside a repository there is no workflow to reject
	}
	return a.Workflow(ctx, filepath.Join(repo.Root, workflow.File), running).Err
}

// Latest asks the releases page for the latest release, within the timeout of the hook or of a person.
func (a Adapter) Latest(ctx context.Context) (string, error) {
	limit := timeout
	if a.Hook {
		limit = hookTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	return a.client().Latest(ctx)
}

// Install checks that exe can be replaced before downloading anything, downloads the binary of release for this
// system checked against the checksums of its release, and replaces exe with it. A replace that fails is a
// version.ErrReplace.
func (a Adapter) Install(ctx context.Context, exe, release string) error {
	if err := github.Writable(exe, temp); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	bin, err := a.client().Download(ctx, release, github.AssetName("polix", runtime.GOOS, runtime.GOARCH))
	if err != nil {
		return err
	}
	if err := github.Replace(exe, temp, bin); err != nil {
		return fmt.Errorf("%w: %w", version.ErrReplace, err)
	}
	return nil
}

// Refresh runs the binary at exe to refresh the plugin: with hook `__refresh --quiet`, which only refreshes the
// plugin already installed; else `install --skills --yes --quiet`, which installs it when missing, and then
// `__refresh --hooks`, which also refreshes the hooks the repository registers.
func (a Adapter) Refresh(ctx context.Context, exe string, hook bool) (string, string, error) {
	if hook {
		return a.run(ctx, exe, "__refresh", "--quiet")
	}
	stdout, stderr, err := a.run(ctx, exe, "install", "--skills", "--yes", "--quiet")
	if err != nil {
		return stdout, stderr, err
	}
	out, errOut, err := a.run(ctx, exe, "__refresh", "--hooks")
	return stdout + out, stderr + errOut, err
}

// Doctor runs `doctor --hook` with the binary at exe.
func (a Adapter) Doctor(ctx context.Context, exe string) (string, string, error) {
	return a.run(ctx, exe, "doctor", "--hook")
}

// client is the client of the releases page, POLIX_RELEASES_URL or else the one of polix.
func (a Adapter) client() *github.Client {
	releases := os.Getenv("POLIX_RELEASES_URL")
	if releases == "" {
		releases = releasesPage
	}
	return github.New(releases, agent)
}

// cacheFile is <user cache dir>/polix/update.json; POLIX_CACHE_DIR overrides the folder.
func (a Adapter) cacheFile() (string, error) {
	return file.CacheFile("POLIX_CACHE_DIR", "polix", "update.json")
}

// run runs exe with args from the working folder and returns what it printed to stdout and stderr.
func (a Adapter) run(ctx context.Context, exe string, args ...string) (string, string, error) {
	return proc.Output(ctx, a.Dir, append([]string{exe}, args...), 0)
}
