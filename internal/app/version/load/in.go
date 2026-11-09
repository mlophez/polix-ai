// Package load is the use case LoadVersion, the loader of the cache of the checks of the latest release
// (version.Load): cmd/polix builds it once and passes it to every use case that reads the cache, which call it lazily
// with the file they resolved.
package load

import (
	"context"
	"sync"

	"github.com/mlophez/polix-ai/internal/app/version"
)

// Request is what a caller of the loader asks of the use case LoadVersion: the argument of version.Load.
type Request struct {
	Path string // the file of the cache, with the separators of the system
}

// Response is what the use case LoadVersion gives back to the caller of the loader.
type Response struct {
	Loaded version.Loaded // what the loader returns
}

// Port is what the shell of LoadVersion needs from the outside: the file of the cache. It speaks in domain values,
// and every method takes the context of the caller.
type Port interface {
	// ReadCache returns the cache of the file at path, not validated, and whether the file exists; the error tells
	// why it cannot be read or decoded. A missing file is an empty cache and no error.
	ReadCache(ctx context.Context, path string) (version.Cache, bool, error)
}

// loader keeps what the shell loaded for each request for the life of the process, so a file is read once.
type loader struct {
	port Port
	mu   sync.Mutex
	seen map[Request]version.Loaded
}

// Command is the entry of the use case LoadVersion: it is not a cobra command but the loader of the cache, which
// cmd/polix builds once and injects into the use cases that read it. The loader turns its argument into a Request,
// runs the shell with the adapter as the port the first time it is asked for a file, and returns the same result
// every later time.
func Command() version.Load {
	l := &loader{port: Adapter{}, seen: map[Request]version.Loaded{}}
	return l.load
}

// run is the shell, from request to response: it reads the cache through the port, calls the logic once and
// builds the response. It never fails: what stops the reading is data of the response.
//
//nolint:unparam // every shell returns (response, error), as docs/architecture.md says, even when it cannot fail
func run(ctx context.Context, port Port, req Request) (Response, error) {
	cache, exists, err := port.ReadCache(ctx, req.Path)
	out := Decide(cache, exists, err)
	return Response(out), nil
}

// load is the loader: the result of the shell for path, run only the first time it is asked for.
func (l *loader) load(ctx context.Context, path string) version.Loaded {
	l.mu.Lock()
	defer l.mu.Unlock()
	req := Request{Path: path}
	if loaded, ok := l.seen[req]; ok {
		return loaded
	}
	res, _ := run(ctx, l.port, req) // the shell never fails
	l.seen[req] = res.Loaded
	return res.Loaded
}
