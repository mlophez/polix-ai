package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/app/version"
)

// CacheFile is tested against a temporary folder: the real disk.

var now = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

// writeCache writes the file of the cache in a temporary folder and returns its path.
func writeCache(t *testing.T, data string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "update.json")
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The adapter reads the shape of the file of the cache, the failure included.
func TestCacheFileReadCache(t *testing.T) {
	c := version.Cache{CheckedAt: now, Latest: "0.2.0"}
	for _, want := range []version.Cache{c, c.Fail(now)} {
		data, err := json.Marshal(cacheJSON(want))
		if err != nil {
			t.Fatal(err)
		}
		got, exists, err := (&CacheFile{}).read(writeCache(t, string(data)))
		if err != nil || !exists || !got.CheckedAt.Equal(want.CheckedAt) || got.Latest != "0.2.0" ||
			!got.FailedAt.Equal(want.FailedAt) {
			t.Fatalf("round trip of %s: %+v %v %v", data, got, exists, err)
		}
	}
}

// A missing file is an empty cache and no error.
func TestCacheFileReadCacheWithoutFile(t *testing.T) {
	got, exists, err := (&CacheFile{}).read(filepath.Join(t.TempDir(), "update.json"))
	if got != (version.Cache{}) || exists || err != nil {
		t.Fatalf("got %+v %v %v", got, exists, err)
	}
}

// A file that cannot be decoded is the error; a cache that decodes but is not valid is read as it is, for the
// logic to reject.
func TestCacheFileReadCacheBroken(t *testing.T) {
	for _, bad := range []string{"", "{"} {
		if got, exists, err := (&CacheFile{}).read(writeCache(t, bad)); err == nil || !exists ||
			got != (version.Cache{}) {
			t.Errorf("ReadCache(%q) = %+v %v %v, want the error", bad, got, exists, err)
		}
	}
	got, _, err := (&CacheFile{}).read(writeCache(t, `{"latest":"nightly"}`))
	if err != nil || got.Latest != "nightly" {
		t.Fatalf("got %+v %v", got, err)
	}
}

// A file that cannot be read is the error.
func TestCacheFileReadCacheUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("the permissions of a file do not stop reads on Windows nor for root")
	}
	p := writeCache(t, `{"latest":"0.2.0"}`)
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if got, exists, err := (&CacheFile{}).read(p); err == nil || !exists ||
		got != (version.Cache{}) {
		t.Fatalf("got %+v %v %v", got, exists, err)
	}
}

// A second call of Load for the same file returns the first result without reading it again: the file changed after
// the first call is not seen; another file is read.
func TestCacheFileLoadReadsOnce(t *testing.T) {
	p := writeCache(t, `{"latest":"0.2.0"}`)
	c := &CacheFile{}
	ctx := context.Background()
	first := c.Load(ctx, p)
	if err := os.WriteFile(p, []byte(`{"latest":"0.3.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	second := c.Load(ctx, p)
	if first.Cache.Latest != "0.2.0" || second.Cache.Latest != "0.2.0" || !second.Exists || second.Err != nil {
		t.Fatalf("first %+v, second %+v", first, second)
	}
	if got := c.Load(ctx, writeCache(t, `{"latest":"0.4.0"}`)); got.Cache.Latest != "0.4.0" {
		t.Fatalf("another file is read: %+v", got)
	}
	if got := (&CacheFile{}).Load(ctx, p); got.Cache.Latest != "0.3.0" {
		t.Fatalf("a new CacheFile reads again: %+v", got)
	}
}

// Load never fails: a file that cannot be decoded, holds a cache that is not valid or is missing is an empty cache,
// with Err set unless it is missing, which only means asking again.
func TestCacheFileLoadNeverFails(t *testing.T) {
	load := (&CacheFile{}).Load
	for _, bad := range []string{"", "{", `{"latest":"nightly"}`} {
		if got := load(context.Background(), writeCache(t, bad)); got.Cache != (version.Cache{}) || got.Err == nil {
			t.Errorf("load of %q = %+v, want an empty cache with its error", bad, got)
		}
	}
	if got := load(context.Background(), filepath.Join(t.TempDir(), "update.json")); got != (version.Loaded{}) {
		t.Fatalf("no file: %+v", got)
	}
	t.Run("unreadable", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("the permissions of a file do not stop reads on Windows nor for root")
		}
		p := writeCache(t, `{"latest":"0.2.0"}`)
		if err := os.Chmod(p, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
		got := (&CacheFile{}).Load(context.Background(), p)
		if got.Err == nil || got.Cache != (version.Cache{}) {
			t.Fatalf("unreadable file: %+v", got)
		}
	})
}
