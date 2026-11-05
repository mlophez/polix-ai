package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/version"
	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/file"
	"github.com/mlophez/polix-ai/pkg/git"
	"github.com/mlophez/polix-ai/pkg/github"
	"github.com/mlophez/polix-ai/pkg/proc"
	"github.com/mlophez/polix-ai/plugins"
)

// DiagnoseSetupAdapter implements DiagnoseSetupPort on the real machine: disk, git, network and the CLI of Claude
// Code. The entry fills it and passes it as the port; there is no constructor.
type DiagnoseSetupAdapter struct {
	OS   string        // the system the checks run on (runtime.GOOS): on windows they need an interpreter
	Look proc.LookFunc // how a program is found on the PATH (exec.LookPath)
}

const (
	// diagnoseSetupReleases is the releases page of polix; POLIX_RELEASES_URL overrides it.
	diagnoseSetupReleases = "https://github.com/mlophez/polix-ai/releases"
	// diagnoseSetupLatestTimeout bounds the question for the latest release.
	diagnoseSetupLatestTimeout = 5 * time.Second
	// diagnoseSetupCLITimeout bounds the call to the CLI of Claude Code.
	diagnoseSetupCLITimeout = time.Minute
)

// Home returns the home folder of the user, or "".
func (a DiagnoseSetupAdapter) Home(_ context.Context) string {
	home, _ := os.UserHomeDir()
	return home
}

// Executable returns the real path of the running binary.
func (a DiagnoseSetupAdapter) Executable(_ context.Context) (string, error) {
	return github.Executable()
}

// LoadCache reads the cache of polix update, empty when there is none or it cannot be read.
func (a DiagnoseSetupAdapter) LoadCache(_ context.Context) version.Cache {
	path, err := file.CacheFile("POLIX_CACHE_DIR", "polix", "update.json")
	if err != nil {
		return version.Cache{}
	}
	data, _, _ := file.Read(path)
	return version.ParseCache(data)
}

// SaveCache writes the cache of polix update.
func (a DiagnoseSetupAdapter) SaveCache(_ context.Context, c version.Cache) error {
	path, err := file.CacheFile("POLIX_CACHE_DIR", "polix", "update.json")
	if err != nil {
		return err
	}
	data, err := c.Encode()
	if err != nil {
		return err
	}
	return file.Write(path, data)
}

// Latest asks the releases page (POLIX_RELEASES_URL, else the one of polix) for the latest release, within 5
// seconds.
func (a DiagnoseSetupAdapter) Latest(ctx context.Context) (string, error) {
	releases := os.Getenv("POLIX_RELEASES_URL")
	if releases == "" {
		releases = diagnoseSetupReleases
	}
	ctx, cancel := context.WithTimeout(ctx, diagnoseSetupLatestTimeout)
	defer cancel()
	return github.New(releases, "polix-update").Latest(ctx)
}

// LookPath finds name on the PATH, with its symlinks resolved.
func (a DiagnoseSetupAdapter) LookPath(_ context.Context, name string) (string, error) {
	found, err := a.Look(name)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(found); err == nil {
		found = real
	}
	return found, nil
}

// SameFile reports whether both paths exist and are the same file.
func (a DiagnoseSetupAdapter) SameFile(_ context.Context, x, y string) bool {
	ix, errX := os.Stat(x)
	iy, errY := os.Stat(y)
	return x != "" && y != "" && errX == nil && errY == nil && os.SameFile(ix, iy)
}

// OpenRepo asks git for the repository of dir.
func (a DiagnoseSetupAdapter) OpenRepo(ctx context.Context, dir string) (string, string, error) {
	repo, err := git.Open(ctx, dir)
	if err != nil {
		return "", "", err
	}
	return repo.Root, repo.GitDir, nil
}

// LoadWorkflow reads polix.yaml of root and parses it (ParseWorkflow).
func (a DiagnoseSetupAdapter) LoadWorkflow(_ context.Context, root, running string) (workflow.Workflow, bool, error) {
	data, existed, err := file.Read(filepath.Join(root, workflow.File))
	if err != nil {
		return workflow.Workflow{}, true, err
	}
	return ParseWorkflow(data, existed, running)
}

// LoadRules reads the yaml files of ai/rules/ of root and parses them (rule.ParseFilesAll).
func (a DiagnoseSetupAdapter) LoadRules(_ context.Context, root string) ([]rule.Rule, []string, error) {
	files, err := file.ReadDir(filepath.Join(root, rule.Dir), ".yaml")
	if err != nil {
		return nil, nil, err
	}
	return rule.ParseFilesAll(files)
}

// Missing reports whether the check does not exist below root.
func (a DiagnoseSetupAdapter) Missing(_ context.Context, root, check string) bool {
	return !file.Exists(filepath.Join(root, filepath.FromSlash(check)))
}

// Runnable tells why a check that exists cannot run as a program: on Windows, where a shebang does nothing, when
// its interpreter is not found; elsewhere, when it lacks the permission to execute. A missing check is left to
// Missing.
func (a DiagnoseSetupAdapter) Runnable(_ context.Context, root, check string) error {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(check)))
	switch {
	case err != nil:
		return nil
	case a.OS == "windows":
		_, err := proc.Interpreter(check, a.Look, proc.FileExists)
		return err
	case info.Mode()&0o111 == 0:
		return fmt.Errorf("check %s is not executable (chmod +x)", check)
	}
	return nil
}

// ResolveBase asks git for the first of candidates that exists, locally or on origin.
func (a DiagnoseSetupAdapter) ResolveBase(ctx context.Context, root string, candidates []string) error {
	repo, err := git.Open(ctx, root)
	if err != nil {
		return err
	}
	_, err = repo.ResolveBase(ctx, candidates)
	return err
}

// WriteState creates the folder of the state and a temporary file in it, which it removes.
func (a DiagnoseSetupAdapter) WriteState(_ context.Context, gitDir string) (string, error) {
	dir := filepath.Join(gitDir, state.DirName)
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		var f *os.File
		if f, err = os.CreateTemp(dir, "doctor-*"); err == nil {
			f.Close()
			os.Remove(f.Name())
		}
	}
	return dir, err
}

// ReadFiles reads the files of paths: the content of those that exist, and the error of those that cannot be read.
func (a DiagnoseSetupAdapter) ReadFiles(_ context.Context, paths []string) (map[string][]byte, map[string]error) {
	files, errs := map[string][]byte{}, map[string]error{}
	for _, p := range paths {
		data, existed, err := file.Read(p)
		switch {
		case err != nil:
			errs[p] = err
		case existed:
			files[p] = data
		}
	}
	return files, errs
}

// Plugin reads the plugin embedded in the binary.
func (a DiagnoseSetupAdapter) Plugin(_ context.Context) (map[string][]byte, error) {
	return plugins.Files()
}

// ClaudePlugins runs `claude plugin list --json` within a minute; ErrNoCLI without the CLI.
func (a DiagnoseSetupAdapter) ClaudePlugins(ctx context.Context) ([]byte, error) {
	if _, err := a.Look("claude"); err != nil {
		return nil, ErrNoCLI
	}
	out, err := proc.Stdout(ctx, "", []string{"claude", "plugin", "list", "--json"}, diagnoseSetupCLITimeout)
	return []byte(out), err
}

// CodexUsed reports whether codex is on the PATH, or ~/.codex or <root>/.codex is a folder.
func (a DiagnoseSetupAdapter) CodexUsed(_ context.Context, home, root string) bool {
	if _, err := a.Look("codex"); err == nil {
		return true
	}
	return file.IsDir(filepath.Join(home, ".codex")) || root != "" && file.IsDir(filepath.Join(root, ".codex"))
}
