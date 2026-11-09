package load

import "github.com/mlophez/polix-ai/internal/app/version"

// Output is the decision of LoadVersion: what the file of the cache loads.
type Output struct {
	Loaded version.Loaded
}

// Decide validates the cache read from its file with the domain (Loaded.Valid, which asks Cache.Valid): a file
// that cannot be read or decoded, or a cache that is not valid, loads an empty cache with why, which only means
// asking again.
func Decide(cache version.Cache, exists bool, err error) Output {
	loaded := version.Loaded{Cache: cache, Exists: exists}
	if err != nil {
		return Output{Loaded: loaded.Fail(err)}
	}
	if err := loaded.Valid(); err != nil {
		return Output{Loaded: loaded.Fail(err)}
	}
	return Output{Loaded: loaded}
}
