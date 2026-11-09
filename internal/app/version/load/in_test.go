package load

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/version"
)

// fakePort answers the reading of the cache with what it is given, and records each file it was asked for.
type fakePort struct {
	cache  version.Cache
	exists bool
	err    error
	asked  []string
}

func (f *fakePort) ReadCache(_ context.Context, path string) (version.Cache, bool, error) {
	f.asked = append(f.asked, path)
	return f.cache, f.exists, f.err
}

// The shell reads the file of the request through the port and builds the response from the logic.
func TestRun(t *testing.T) {
	port := &fakePort{cache: version.Cache{CheckedAt: now, Latest: "0.2.0"}, exists: true}
	res, err := run(context.Background(), port, Request{Path: "/cache/update.json"})
	if err != nil || !slices.Equal(port.asked, []string{"/cache/update.json"}) {
		t.Fatal(port.asked, err)
	}
	if res.Loaded.Cache != port.cache || !res.Loaded.Exists || res.Loaded.Err != nil {
		t.Fatalf("loaded %+v", res.Loaded)
	}
}

// A file that cannot be read is data of the response, never an error of the shell.
func TestRunNeverFails(t *testing.T) {
	boom := errors.New("boom")
	res, err := run(context.Background(), &fakePort{exists: true, err: boom}, Request{Path: "/cache/update.json"})
	if err != nil || !errors.Is(res.Loaded.Err, boom) || res.Loaded.Cache != (version.Cache{}) {
		t.Fatalf("got %+v %v", res, err)
	}
}

// The loader reads a file once for the life of the process: a second call for the same file returns the same
// result without reading again; another file is read.
func TestLoaderReadsOnce(t *testing.T) {
	port := &fakePort{cache: version.Cache{Latest: "0.2.0"}, exists: true}
	l := &loader{port: port, seen: map[Request]version.Loaded{}}
	ctx := context.Background()
	first := l.load(ctx, "/a/update.json")
	port.cache = version.Cache{}
	second := l.load(ctx, "/a/update.json")
	if first.Cache.Latest != "0.2.0" || second.Cache.Latest != "0.2.0" || len(port.asked) != 1 {
		t.Fatalf("first %+v, second %+v, asked %v", first, second, port.asked)
	}
	l.load(ctx, "/b/update.json")
	if want := []string{"/a/update.json", "/b/update.json"}; !slices.Equal(port.asked, want) {
		t.Fatalf("asked %v, want %v", port.asked, want)
	}
}

// Command returns a loader, ready to be called.
func TestCommand(t *testing.T) {
	load := Command()
	if got := load(context.Background(), filepath.Join(t.TempDir(), "update.json")); got != (version.Loaded{}) {
		t.Fatalf("no file: %+v", got)
	}
}
