// Package adapters is the good case of an adapters package: its struct File keeps a cache and its mutex unexported, its
// decoding type is unexported and its exported method UnmarshalYAML is admitted (GOARCH-25, GOARCH-46).
package adapters

import "sync"

// File reads the order file once per path.
type File struct {
	mu    sync.Mutex
	cache map[string]int
}

// fileYAML decodes the format of the file; yaml.v3 needs the exported method UnmarshalYAML.
type fileYAML struct{ Items []string }

// Load returns the items of the file at path, from the cache after the first call.
func (f *File) Load(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := f.cache[path]; ok {
		return n
	}
	n := count(nil)
	f.cache[path] = n
	return n
}

// UnmarshalYAML decodes the items.
func (y *fileYAML) UnmarshalYAML(decode func(any) error) error { return decode(&y.Items) }

// maxItems is an unexported constant.
const maxItems = 8

// count is an unexported helper.
func count(items []string) int { return min(len(items), maxItems) }
