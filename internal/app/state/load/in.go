// Package load is the use case LoadState, the loader of the log of polix (state.Load): cmd/polix builds it once and
// passes it to every use case that reads the log, which call it with the file they resolved.
package load

import (
	"context"

	"github.com/mlophez/polix-ai/internal/app/state"
)

// Request is what a caller of the loader asks of the use case LoadState: the argument of state.Load.
type Request struct {
	Path string // the file of the log, with the separators of the system
}

// Response is what the use case LoadState gives back to the caller of the loader.
type Response struct {
	Loaded state.Loaded // what the loader returns
}

// Port is what the shell of LoadState needs from the outside: the file of the log. It speaks in domain values, and
// every method takes the context of the caller.
type Port interface {
	// ReadLog returns the entries of the lines of the log at path that decode, in file order and kept as they are,
	// and how many lines were skipped for being over state.MaxLine; the error tells why the log cannot be read. A
	// missing log has no lines and no error.
	ReadLog(ctx context.Context, path string) ([]state.Entry, int, error)
}

// loader runs the shell for every call: the log is appended to by the process that reads it, so each call reads
// it again instead of keeping an earlier result.
type loader struct {
	port Port
}

// Command is the entry of the use case LoadState: it is not a cobra command but the loader of the log, which
// cmd/polix builds once and injects into the use cases that read it. The loader turns its argument into a Request
// and runs the shell with the adapter as the port.
func Command() state.Load {
	l := loader{port: Adapter{}}
	return l.load
}

// run is the shell, from request to response: it reads the log through the port, calls the logic once and builds
// the response. It never fails: what stops the reading is data of the response.
//
//nolint:unparam // every shell returns (response, error), as docs/architecture.md says, even when it cannot fail
func run(ctx context.Context, port Port, req Request) (Response, error) {
	entries, skipped, err := port.ReadLog(ctx, req.Path)
	out := Decide(entries, skipped, err)
	return Response(out), nil
}

// load is the loader: the result of the shell for path.
func (l loader) load(ctx context.Context, path string) state.Loaded {
	res, _ := run(ctx, l.port, Request{Path: path}) // the shell never fails
	return res.Loaded
}
