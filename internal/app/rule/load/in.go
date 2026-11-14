// Package load is the use case LoadRule, the loader of the rules of a folder of rules (rule.Load): cmd/polix builds
// it once and passes it to every use case that reads the rules, which call it lazily with the folder they resolved.
package load

import (
	"context"
	"sync"

	"github.com/mlophez/polix-ai/internal/app/rule"
)

// Request is what a caller of the loader asks of the use case LoadRule: the arguments of rule.Load.
type Request struct {
	Root string // the root of the repository, with the separators of the system
	Dir  string // the folder of rules, relative to the root, with slashes (rules/ below path of polix.yaml)
}

// Response is what the use case LoadRule gives back to the caller of the loader.
type Response struct {
	Loaded rule.Loaded // what the loader returns
}

// Port is what the shell of LoadRule needs from the outside: the files of the folder of rules. It speaks in domain
// values, and every method takes the context of the caller.
type Port interface {
	// ReadRules returns the rules of the yaml files of the folder dir of root but rule.RetiredFile, file by file in
	// the order of their names and in the order each file writes them, not validated, and whether the folder
	// exists. The error tells why a file cannot be read or is not yaml, with the rules of the files before it.
	ReadRules(ctx context.Context, root, dir string) ([]rule.Rule, bool, error)
	// ReadRetired returns the rules of rule.RetiredFile of the folder dir of root, in the order of the file, each with
	// its file, not validated; none when the file is missing, cannot be read or is not yaml. It never fails: the
	// retired rules only take their ids.
	ReadRetired(ctx context.Context, root, dir string) []rule.Rule
}

// loader keeps what the shell loaded for each request for the life of the process, so a folder is read once.
type loader struct {
	port Port
	mu   sync.Mutex
	seen map[Request]rule.Loaded
}

// Command is the entry of the use case LoadRule: it is not a cobra command but the loader of the rules, which
// cmd/polix builds once and injects into the use cases that read them. The loader turns its arguments into a
// Request, runs the shell with the adapter as the port the first time it is asked for a folder, and returns the
// same result every later time.
func Command() rule.Load {
	l := &loader{port: Adapter{}, seen: map[Request]rule.Loaded{}}
	return l.load
}

// run is the shell, from request to response: it reads the rules of the folder and then its retired rules through
// the port, calls the logic once and builds the response. It never fails: what stops the reading is data of the
// response.
//
//nolint:unparam // every shell returns (response, error), as docs/architecture.md says, even when it cannot fail
func run(ctx context.Context, port Port, req Request) (Response, error) {
	rules, exists, err := port.ReadRules(ctx, req.Root, req.Dir)
	retired := port.ReadRetired(ctx, req.Root, req.Dir)
	out := Decide(rules, retired, exists, err)
	return Response(out), nil
}

// load is the loader: the result of the shell for root and dir, run only the first time they are asked for.
func (l *loader) load(ctx context.Context, root, dir string) rule.Loaded {
	l.mu.Lock()
	defer l.mu.Unlock()
	req := Request{Root: root, Dir: dir}
	if loaded, ok := l.seen[req]; ok {
		return loaded
	}
	res, _ := run(ctx, l.port, req) // the shell never fails
	l.seen[req] = res.Loaded
	return res.Loaded
}
