package load

import (
	"errors"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/app/version"
)

var now = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

// A valid cache is loaded as it is; without the file, an empty cache.
func TestDecide(t *testing.T) {
	c := version.Cache{CheckedAt: now, Latest: "0.2.0", FailedAt: now}
	if got := Decide(c, true, nil).Loaded; got.Cache != c || !got.Exists || got.Err != nil {
		t.Fatalf("got %+v", got)
	}
	if got := Decide(version.Cache{}, false, nil).Loaded; got != (version.Loaded{}) {
		t.Fatalf("no file: %+v", got)
	}
}

// A file that cannot be read or decoded, or a cache that is not valid, is an empty cache with why.
func TestDecideEmptiesABrokenCache(t *testing.T) {
	boom := errors.New("unexpected end of JSON input")
	if got := Decide(version.Cache{}, true, boom).Loaded; got.Cache != (version.Cache{}) || !errors.Is(got.Err, boom) ||
		!got.Exists {
		t.Fatalf("a broken file: %+v", got)
	}
	if got := Decide(version.Cache{Latest: "nightly"}, true, nil).Loaded; got.Cache != (version.Cache{}) ||
		got.Err == nil {
		t.Fatalf("a latest release that is not a version: %+v", got)
	}
}
