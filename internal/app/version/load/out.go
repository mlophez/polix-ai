package load

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/platform/file"
)

// Adapter implements Port on the disk. The entry passes it as the port; there is no constructor.
type Adapter struct{}

// cacheJSON is the shape of the file of the cache, with the fields of version.Cache in its order, so it converts to
// one.
type cacheJSON struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
	FailedAt  time.Time `json:"failed_at,omitzero"`
}

// ReadCache reads the file of the cache through internal/platform/file and decodes it. A missing file is an empty
// cache; one that cannot be read or decoded is the error.
func (a Adapter) ReadCache(_ context.Context, path string) (version.Cache, bool, error) {
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
