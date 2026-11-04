package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.2.0", "0.2.0", 0},
		{"v0.2.0", "0.2.0", 0},
		{"0.2.0", "0.10.0", -1},
		{"1.0.0", "0.99.99", 1},
		{"0.2.0-rc1", "0.2.0", -1},
		{"0.2.0", "0.2.0-rc1", 1},
		{"0.2.0-rc1", "0.2.0-rc2", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsRelease(t *testing.T) {
	for v, want := range map[string]bool{"0.2.0": true, "1.0.0-rc1": true, "dev": false, "": false, "0.0.0": false} {
		if got := IsRelease(v); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, got, want)
		}
	}
}

// releases serves a fake releases page with one release, v0.2.0, whose binary is bin.
func releases(t *testing.T, bin []byte, sum string) *httptest.Server {
	t.Helper()
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tag/v0.2.0", http.StatusFound)
	})
	mux.HandleFunc("/download/v0.2.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0000  polix_other_os\n" + sum + "  " + asset + "\n"))
	})
	// The binary is served through a redirect, as GitHub does.
	mux.HandleFunc("/download/v0.2.0/"+asset, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/assets/"+asset, http.StatusFound)
	})
	mux.HandleFunc("/assets/"+asset, func(w http.ResponseWriter, r *http.Request) { w.Write(bin) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestLatestAndDownload(t *testing.T) {
	bin := []byte("new binary")
	srv := releases(t, bin, sha(bin))
	t.Setenv("POLIX_RELEASES_URL", srv.URL)
	c := New()
	latest, err := c.Latest(context.Background())
	if err != nil || latest != "0.2.0" {
		t.Fatalf("Latest = %q, %v", latest, err)
	}
	got, err := c.Download(context.Background(), "v0.2.0")
	if err != nil || string(got) != string(bin) {
		t.Fatalf("Download = %q, %v", got, err)
	}
}

// A binary that does not match checksums.txt is never returned.
func TestDownloadRejectsTamperedBinary(t *testing.T) {
	srv := releases(t, []byte("tampered"), sha([]byte("original")))
	t.Setenv("POLIX_RELEASES_URL", srv.URL)
	if _, err := New().Download(context.Background(), "0.2.0"); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("want a checksum error, got %v", err)
	}
}

// While a repository has no release, GitHub sends releases/latest back to the releases page.
func TestLatestWithoutReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases", http.StatusFound)
	}))
	defer srv.Close()
	t.Setenv("POLIX_RELEASES_URL", srv.URL)
	if _, err := New().Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "no release") {
		t.Fatalf("want no release, got %v", err)
	}
}

func TestReplace(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "polix")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Replace(exe, []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(exe)
	info, _ := os.Stat(exe)
	if string(data) != "new" || (runtime.GOOS != "windows" && info.Mode()&0o111 == 0) {
		t.Fatalf("replaced binary: %q %v", data, info.Mode())
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) > 2 { // the binary, and on Windows the old one
		t.Fatalf("temporary files left: %v", entries)
	}
}

func TestCache(t *testing.T) {
	t.Setenv("POLIX_CACHE_DIR", t.TempDir())
	if c := LoadCache(); !c.CheckedAt.IsZero() {
		t.Fatalf("empty cache: %+v", c)
	}
	if err := SaveCache(Cache{Latest: "0.2.0"}); err != nil {
		t.Fatal(err)
	}
	if c := LoadCache(); c.Latest != "0.2.0" {
		t.Fatalf("cache: %+v", c)
	}
}
