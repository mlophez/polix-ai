// Package adapters reads the cache of the checks of the latest release: CacheFile loads it (version.Load) and keeps
// what it read for the life of the process, so cmd/polix builds it once and passes its method Load to every use case
// that reads the cache. It reads, decodes and caches; what the cache means is decided by version.Cache.Loaded.
package adapters

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/platform/file"
)

// CacheFile loads the file of the cache. Its zero value works: the first Load of a file reads it, and every later
// one returns the same result.
type CacheFile struct {
	mu   sync.Mutex
	seen map[string]version.Loaded
}

// cacheJSON is the shape of the file of the cache, with the fields of version.Cache in its order, so it converts to
// one.
type cacheJSON struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
	FailedAt  time.Time `json:"failed_at,omitzero"`
}

// Load loads the cache of the file at path: it reads and decodes the file and has version.Cache.Loaded decide what
// it loads. It never fails: what stops the reading is data of its result. The first call for a file reads it, and
// every later call returns the same result.
func (c *CacheFile) Load(_ context.Context, path string) version.Loaded {
	c.mu.Lock()
	defer c.mu.Unlock()
	if loaded, ok := c.seen[path]; ok {
		return loaded
	}
	cache, exists, err := c.read(path)
	loaded := cache.Loaded(exists, err)
	if c.seen == nil {
		c.seen = map[string]version.Loaded{}
	}
	c.seen[path] = loaded
	return loaded
}

// read reads the file of the cache through internal/platform/file and decodes it. A missing file is an empty cache;
// one that cannot be read or decoded is the error.
func (c *CacheFile) read(path string) (version.Cache, bool, error) {
	data, exists, err := file.Read(path)
	if err != nil {
		return version.Cache{}, true, err
	}
	if !exists {
		return version.Cache{}, false, nil
	}
	var j cacheJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return version.Cache{}, true, err
	}
	return version.Cache(j), true, nil
}
