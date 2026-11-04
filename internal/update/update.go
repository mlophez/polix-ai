// Package update finds, downloads and installs releases of polix from GitHub Releases.
//
// A release holds one raw binary per platform, named polix_<os>_<arch> (.exe on Windows), and checksums.txt
// with their SHA-256 in the format of sha256sum. Every download is checked against it before it replaces the
// running binary.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultReleases is the releases page of the repository; POLIX_RELEASES_URL overrides it (a mirror, or tests).
const DefaultReleases = "https://github.com/mlophez/polix-ai/releases"

// maxBinary bounds a download, so a wrong URL never fills the disk.
const maxBinary = 200 << 20

// Client talks to a releases page.
type Client struct {
	Releases string
	HTTP     *http.Client
}

// New returns a client; callers bound each request with the context they pass.
func New() *Client {
	base := os.Getenv("POLIX_RELEASES_URL")
	if base == "" {
		base = DefaultReleases
	}
	return &Client{
		Releases: strings.TrimRight(base, "/"),
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

// Download returns the binary of version for this platform, checked against the checksums of the release.
func (c *Client) Download(ctx context.Context, version string) ([]byte, error) {
	base := fmt.Sprintf("%s/download/v%s/", c.Releases, strings.TrimPrefix(version, "v"))
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	sums, err := c.fetch(ctx, base+"checksums.txt", 1<<20)
	if err != nil {
		return nil, err
	}
	want, err := checksum(sums, asset)
	if err != nil {
		return nil, err
	}
	bin, err := c.fetch(ctx, base+asset, maxBinary)
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
	req.Header.Set("User-Agent", "polix-update")
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

// AssetName is the name of the binary of a platform in a release.
func AssetName(goos, goarch string) string {
	name := "polix_" + goos + "_" + goarch
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

// Replace installs data as the binary at exe. The new binary is written next to it and renamed over it, so a
// hook that starts meanwhile runs either the old or the new one, never half of it. Windows cannot replace a
// running binary, so the old one is moved aside to <exe>.old first; Cleanup removes it on a later run.
func Replace(exe string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".polix-update-*")
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
	if runtime.GOOS == "windows" {
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return err
		}
	}
	return os.Rename(tmp.Name(), exe)
}

// Cleanup removes the binary a previous update on Windows moved aside.
func Cleanup(exe string) { _ = os.Remove(exe + ".old") }

// Compare compares two versions (1.2.3, with an optional v and -prerelease): -1, 0 or 1. A prerelease is lower
// than its release; prereleases compare as text.
func Compare(a, b string) int {
	an, ap := parse(a)
	bn, bp := parse(b)
	for i := range an {
		if an[i] != bn[i] {
			if an[i] < bn[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case ap == bp:
		return 0
	case ap == "":
		return 1
	case bp == "":
		return -1
	case ap < bp:
		return -1
	}
	return 1
}

func parse(v string) ([3]int, string) {
	v = strings.TrimPrefix(v, "v")
	v, pre, _ := strings.Cut(v, "-")
	var n [3]int
	for i, p := range strings.SplitN(v, ".", 3) {
		n[i], _ = strconv.Atoi(p)
	}
	return n, pre
}

// IsRelease reports whether v is a released version (not a development build).
func IsRelease(v string) bool {
	n, _ := parse(v)
	return v != "" && v != "dev" && n != [3]int{}
}

// Cache remembers the last check of the latest release, so the session start hook asks at most once a day.
type Cache struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

// cacheFile is <user cache dir>/polix/update.json; POLIX_CACHE_DIR overrides the folder.
func cacheFile() (string, error) {
	dir := os.Getenv("POLIX_CACHE_DIR")
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "polix")
	}
	return filepath.Join(dir, "update.json"), nil
}

// LoadCache returns the cache, empty when there is none.
func LoadCache() Cache {
	var c Cache
	if f, err := cacheFile(); err == nil {
		if data, err := os.ReadFile(f); err == nil {
			_ = json.Unmarshal(data, &c)
		}
	}
	return c
}

// SaveCache writes the cache; failing to write it only means asking again next time.
func SaveCache(c Cache) error {
	f, err := cacheFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(f, data, 0o644)
}
