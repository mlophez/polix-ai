// Package github talks to the releases page of a GitHub repository: the tag of the latest release and the
// download of an asset checked against the checksums.txt of its release. It also installs a downloaded binary
// over a running one. Every request is bounded by the context the caller passes.
package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Client talks to a releases page, https://github.com/<owner>/<repo>/releases or a mirror.
type Client struct {
	Releases  string
	HTTP      *http.Client
	UserAgent string // sent with every request
}

// maxAsset bounds a download, so a wrong URL never fills the disk.
const maxAsset = 200 << 20

// Seams for the tests: the rename of Replace, and whether the old binary is moved aside first, as Windows needs.
var (
	rename    = os.Rename
	moveAside = runtime.GOOS == "windows"
)

// New returns a client of the releases page that identifies itself as userAgent; callers bound each request
// with the context they pass.
func New(releases, userAgent string) *Client {
	return &Client{
		Releases:  strings.TrimRight(releases, "/"),
		UserAgent: userAgent,
		HTTP: &http.Client{
			// Latest reads the redirect of releases/latest instead of following it.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Latest returns the version of the latest release, without the leading v. It reads the redirect of
// <releases>/latest to <releases>/tag/<tag>, which, unlike the API, has no rate limit for anonymous calls.
func (c *Client) Latest(ctx context.Context) (string, error) {
	resp, err := c.get(ctx, c.Releases+"/latest")
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode >= 300 && resp.StatusCode < 400 && !strings.Contains(loc, "/tag/") {
		// GitHub sends releases/latest back to the releases page while there is none.
		return "", fmt.Errorf("no release published yet at %s", c.Releases)
	}
	if resp.StatusCode < 300 || resp.StatusCode >= 400 || !strings.Contains(loc, "/tag/") {
		return "", fmt.Errorf("latest release: unexpected answer %s (location %q)", resp.Status, loc)
	}
	return strings.TrimPrefix(path.Base(loc), "v"), nil
}

// Download returns the asset of the release of version, checked against the checksums.txt of the release.
func (c *Client) Download(ctx context.Context, version, asset string) ([]byte, error) {
	base := fmt.Sprintf("%s/download/v%s/", c.Releases, strings.TrimPrefix(version, "v"))
	sums, err := c.fetch(ctx, base+"checksums.txt", 1<<20)
	if err != nil {
		return nil, err
	}
	want, err := checksum(sums, asset)
	if err != nil {
		return nil, err
	}
	bin, err := c.fetch(ctx, base+asset, maxAsset)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(bin)
	if got := hex.EncodeToString(sum[:]); got != want {
		return nil, fmt.Errorf("%s: checksum %s does not match %s of checksums.txt", asset, got, want)
	}
	return bin, nil
}

func (c *Client) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	return c.HTTP.Do(req)
}

// fetch downloads url, following redirects (GitHub serves assets from another host).
func (c *Client) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := c.get(ctx, url)
	for hops := 0; err == nil && resp.StatusCode >= 300 && resp.StatusCode < 400 && hops < 5; hops++ {
		resp.Body.Close()
		next, perr := resp.Request.URL.Parse(resp.Header.Get("Location")) // a Location may be relative
		if perr != nil {
			return nil, perr
		}
		resp, err = c.get(ctx, next.String())
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: larger than %d bytes", url, limit)
	}
	return data, nil
}

// checksum finds the SHA-256 of asset in a sha256sum listing ("<hash>  <name>" or "<hash> *<name>").
func checksum(sums []byte, asset string) (string, error) {
	for line := range strings.SplitSeq(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", asset)
}

// AssetName is the name of the binary of program for a platform in a release: <program>_<os>_<arch>, with .exe
// on Windows.
func AssetName(program, goos, goarch string) string {
	name := program + "_" + goos + "_" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// Executable returns the real path of the running binary.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// Writable reports whether Replace can write next to exe, with a temporary file named prefix*, so an update that
// could never be installed fails before downloading the binary.
func Writable(exe, prefix string) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), prefix+"*")
	if err != nil {
		return fmt.Errorf("cannot write next to %s: %w", exe, err)
	}
	tmp.Close()
	return os.Remove(tmp.Name())
}

// Replace installs data as the binary at exe. The new binary is written next to it and renamed over it, so a
// program that starts meanwhile runs either the old or the new one, never half of it. Windows cannot replace a
// running binary, so the old one is moved aside to <exe>.old first, and moved back when the new one cannot take
// its place (an antivirus may hold the new file); Cleanup removes it on a later run.
// Between the two renames there is no binary at exe.
//
// The new binary is first written to a temporary file named prefix*, next to exe.
func Replace(exe, prefix string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), prefix+"*")
	if err != nil {
		return fmt.Errorf("cannot write next to %s: %w", exe, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if !moveAside {
		return rename(tmp.Name(), exe)
	}
	old := exe + ".old"
	_ = os.Remove(old)
	if err := rename(exe, old); err != nil {
		return err
	}
	if err := rename(tmp.Name(), exe); err != nil {
		if rerr := rename(old, exe); rerr != nil {
			//nolint:errorlint // only the failed update is wrapped: errors.Is must not match the failed restore
			return fmt.Errorf("%w; and the old binary could not be put back from %s: %v", err, old, rerr)
		}
		return err
	}
	return nil
}

// Cleanup removes the binary a previous Replace on Windows moved aside.
func Cleanup(exe string) { _ = os.Remove(exe + ".old") }
