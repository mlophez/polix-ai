// Package load is the use case LoadWorkflow, the loader of polix.yaml (workflow.Load): cmd/polix builds it once and
// passes it to every use case that reads polix.yaml, which call it lazily with the root they resolved.
package load

import (
	"context"
	"sync"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Request is what a caller of the loader asks of the use case LoadWorkflow: the arguments of workflow.Load.
type Request struct {
	Root    string // the root of the repository, with the separators of the system
	Running string // the version of the running polix, which decides whether polix.yaml requires a newer one
}

// Response is what the use case LoadWorkflow gives back to the caller of the loader.
type Response struct {
	Loaded workflow.Loaded // what the loader returns
}

// Port is what the shell of LoadWorkflow needs from the outside: the file polix.yaml. It speaks in domain values,
// and every method takes the context of the caller.
type Port interface {
	// ReadWorkflow reads polix.yaml of root for the polix of version running and decodes it: the workflow, with the
	// protections and the entries its shortcuts expand into, and the section guardrails as written, with the
	// problems of the values of its items; and whether the file exists. The error tells why the file cannot be
	// read, requires a newer polix (workflow.TooOldError), is not yaml or has an unknown top level key
	// (workflow.UnknownKeyError), with an empty workflow and section; else it is the error of Workflow.Valid, with the
	// workflow. A missing file has no workflow and no error.
	ReadWorkflow(ctx context.Context, root, running string) (workflow.Workflow, workflow.Guardrails, bool, error)
}

// loader keeps what the shell loaded for each request for the life of the process, so polix.yaml is read once.
type loader struct {
	port Port
	mu   sync.Mutex
	seen map[Request]workflow.Loaded
}

// Command is the entry of the use case LoadWorkflow: it is not a cobra command but the loader of polix.yaml, which
// cmd/polix builds once and injects into the use cases that read it. The loader turns its arguments into a Request,
// runs the shell with the adapter as the port the first time it is asked for a root and a version, and returns the
// same result every later time.
func Command() workflow.Load {
	l := &loader{port: Adapter{}, seen: map[Request]workflow.Loaded{}}
	return l.load
}

// run is the shell, from request to response: it reads polix.yaml through the port, calls the logic once and
// builds the response. It never fails: what stops the reading is data of the response.
//
//nolint:unparam // every shell returns (response, error), as docs/architecture.md says, even when it cannot fail
func run(ctx context.Context, port Port, req Request) (Response, error) {
	wf, guardrails, exists, err := port.ReadWorkflow(ctx, req.Root, req.Running)
	out := Decide(wf, guardrails, exists, err)
	return Response(out), nil
}

// load is the loader: the result of the shell for root and running, run only the first time they are asked for.
func (l *loader) load(ctx context.Context, root, running string) workflow.Loaded {
	l.mu.Lock()
	defer l.mu.Unlock()
	req := Request{Root: root, Running: running}
	if loaded, ok := l.seen[req]; ok {
		return loaded
	}
	res, _ := run(ctx, l.port, req) // the shell never fails
	l.seen[req] = res.Loaded
	return res.Loaded
}
