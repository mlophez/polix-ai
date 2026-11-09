package load

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/version"
)

// The adapter is tested against a temporary folder: the real disk, as the port promises.

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
func TestAdapterReadCache(t *testing.T) {
	c := version.Cache{CheckedAt: now, Latest: "0.2.0"}
	for _, want := range []version.Cache{c, c.Fail(now)} {
		data, err := json.Marshal(cacheJSON(want))
		if err != nil {
			t.Fatal(err)
		}
		got, exists, err := Adapter{}.ReadCache(context.Background(), writeCache(t, string(data)))
		if err != nil || !exists || !got.CheckedAt.Equal(want.CheckedAt) || got.Latest != "0.2.0" ||
			!got.FailedAt.Equal(want.FailedAt) {
			t.Fatalf("round trip of %s: %+v %v %v", data, got, exists, err)
		}
	}
}

// A missing file is an empty cache and no error.
func TestAdapterReadCacheWithoutFile(t *testing.T) {
	got, exists, err := Adapter{}.ReadCache(context.Background(), filepath.Join(t.TempDir(), "update.json"))
	if got != (version.Cache{}) || exists || err != nil {
		t.Fatalf("got %+v %v %v", got, exists, err)
	}
}

// A file that cannot be decoded is the error; a cache that decodes but is not valid is read as it is, for the
// logic to reject.
func TestAdapterReadCacheBroken(t *testing.T) {
	for _, bad := range []string{"", "{"} {
		if got, exists, err := (Adapter{}).ReadCache(context.Background(), writeCache(t, bad)); err == nil || !exists ||
			got != (version.Cache{}) {
			t.Errorf("ReadCache(%q) = %+v %v %v, want the error", bad, got, exists, err)
		}
	}
	got, _, err := Adapter{}.ReadCache(context.Background(), writeCache(t, `{"latest":"nightly"}`))
	if err != nil || got.Latest != "nightly" {
		t.Fatalf("got %+v %v", got, err)
	}
}

// A file that cannot be read is the error.
func TestAdapterReadCacheUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("the permissions of a file do not stop reads on Windows nor for root")
	}
	p := writeCache(t, `{"latest":"0.2.0"}`)
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if got, exists, err := (Adapter{}).ReadCache(context.Background(), p); err == nil || !exists ||
		got != (version.Cache{}) {
		t.Fatalf("got %+v %v %v", got, exists, err)
	}
}

// The loader on the real disk loads a broken or invalid cache as an empty one, which only means asking again.
func TestLoaderOnTheDisk(t *testing.T) {
	load := Command()
	for _, bad := range []string{"", "{", `{"latest":"nightly"}`} {
		if got := load(context.Background(), writeCache(t, bad)); got.Cache != (version.Cache{}) || got.Err == nil {
			t.Errorf("load of %q = %+v, want an empty cache", bad, got)
		}
	}
}
