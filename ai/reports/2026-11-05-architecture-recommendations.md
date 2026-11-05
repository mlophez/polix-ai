# Examples of the recommendations of the architecture review

Date: 2026-11-05. Base: commit 15c1a46 (main). Before and after examples, on the code of polix, of the three
recommendations of `ai/reports/2026-11-05-architecture-review.md`: one subpackage per use case, small ports that
compose, and `Step` as a sealed interface. The code of the "after" parts is a sketch, not a tested design.

## 1. One subpackage per use case

### Layout

```
BEFORE                                  AFTER
internal/rule/                          internal/rule/                 pure domain
  rule.go                                 rule.go
  rule_parse.go                           rule_parse.go
  rule_result.go                          rule_result.go
  rule_run.go                             rule_run.go
  rule_set.go                             rule_set.go
  check_rule.md                         internal/rule/check/           use case
  check_rule_in.go      (cobra, git)      check.md
  check_rule_logic.go                     command.go      Command, Request, Response, Port
  check_rule_out.go                       logic.go        Decide, Output
                                          adapter.go      Adapter
```

### Import graph

```
BEFORE: the package is the unit, so the domain drags the framework along

  setup ──────────► hook ──────────► cobra, pkg/git, pkg/proc, pkg/jsonl
  (only wants         (Evaluation,
   the domain)         Step... and also
                       AnswerHookCommand
                       and AnswerHookAdapter)

AFTER: the compiler enforces the dependency rule

  setup/doctor ───► hook            (domain only: no cobra nor git)
  hook/answer  ───► hook, rule, state, workflow, cobra, pkg/git
  rule/check   ───► rule, workflow, cobra, pkg/git
  rule/check   ──X─► hook/answer    (a cycle does not compile; visibility isolates)
```

### Names

```go
// BEFORE: the name of the use case repeats as a prefix (GOARCH-31)
rule.CheckRuleCommand()
rule.CheckRuleRequest{...}
rule.CheckRuleAdapter{Repo: repo}
func checkRule(ctx context.Context, port CheckRulePort, req CheckRuleRequest) (CheckRuleResponse, error)

// AFTER: the package gives the context
check.Command()
check.Request{...}
check.Adapter{Repo: repo}
func run(ctx context.Context, port Port, req Request) (Response, error)
```

```go
// cmd/polix/main.go
import (
	rulecheck "github.com/mlophez/polix-ai/internal/rule/check"
	"github.com/mlophez/polix-ai/internal/setup/check"
	"github.com/mlophez/polix-ai/internal/hook/answer"
)

rules.AddCommand(rulecheck.Command())
root.AddCommand(check.Command(), answer.Command())
```

The cost: `rule/check` and `setup/check` share a name, so `main.go` needs an import alias. That is normal in Go and
limited to that file.

### Rules it retires or simplifies

- **GOARCH-31:** retired, since there is no shared namespace.
- **GOARCH-10:** the visibility between packages replaces most of its AST test.
- **GOARCH-40:** retired, since Go does not allow methods on the types of another package.
- **GOARCH-4:** it only has to forbid imports in the whole domain package, not file by file.

## 2. Small ports that compose

### Before: one port of 16 methods

`internal/hook/answer_hook_in.go`:

```go
type AnswerHookPort interface {
	Open(ctx context.Context, dir string) (root, gitDir string, err error)
	LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, bool, error)
	Locate(ctx context.Context, path, cwd string) (string, bool)
	Branch(ctx context.Context) string
	Hash(ctx context.Context) (string, error)
	Changed(ctx context.Context) []string
	Files(ctx context.Context) []string
	LoadRules(ctx context.Context) ([]rule.Rule, error)
	LoadState(ctx context.Context) (state.State, error)
	Ignored(ctx context.Context, path string) bool
	Real(ctx context.Context, paths []string) []string
	Contains(ctx context.Context, patterns []string, text string) bool
	Read(ctx context.Context, path string) (string, error)
	Run(ctx context.Context, argv []string) (string, error)
	RunCheck(ctx context.Context, run rule.Run) (string, error)
	Append(ctx context.Context, e state.Entry) error
}
```

### After: ports by responsibility, grouped in a struct

```go
// internal/hook/answer/ports.go

// Tree is the working tree of the repository, read only.
type Tree interface {
	Branch(ctx context.Context) string
	Hash(ctx context.Context) (string, error)
	Changed(ctx context.Context) []string
	Files(ctx context.Context) []string
	Ignored(ctx context.Context, path string) bool
	Real(ctx context.Context, paths []string) []string
	Read(ctx context.Context, path string) (string, error)
	Contains(ctx context.Context, patterns []string, text string) bool
}

// Runner runs commands and the checks of the rules from the root.
type Runner interface {
	Run(ctx context.Context, argv []string) (string, error)
	RunCheck(ctx context.Context, run rule.Run) (string, error)
}

// Log is the state of polix.
type Log interface {
	Load(ctx context.Context) (state.State, error)
	Append(ctx context.Context, e state.Entry) error
}

// Deps is what the shell needs from the outside; the entry fills it.
type Deps struct {
	Tree   Tree
	Runner Runner
	Log    Log
	Config Config // LoadWorkflow, LoadRules
}
```

### Gain in the tests

Each test fakes only what it uses:

```go
// BEFORE: answerHookFakePort implements the 16 methods for every test
// AFTER: the test of a failing log only replaces the Log
deps := fakeDeps()                                  // inert default fakes
deps.Log = &fakeLog{appendErr: errors.New("disk full")}
res, err := run(ctx, deps, req)
```

### Gain in the adapters

`Runner` is implemented today by two adapters (`CheckRuleAdapter.RunCheck` and `AnswerHookAdapter.RunCheck`) with
almost the same code. With small ports, one adapter serves both use cases, which also solves finding 5 of the
review (adapters with drifting semantics):

```
internal/repo/               shared adapter of the repository (not pkg/: it speaks in domain types)
  runner.go    Runner{Root, Deadline}   ── used by rule/check and hook/answer
  config.go    Config{Root}             ── LoadWorkflow and LoadRules with ONE semantics
```

### Rules it conflicts with

GOARCH-12 (one interface in the in file) and GOARCH-33 (a port embeds no interface) would have to be relaxed.

## 3. Step as a sealed interface

### Before: a union of 13 fields, of which only one must ask

`internal/hook/hook_step.go`:

```go
type Step struct {
	Branch   bool
	Hash     bool
	Epoch    int
	Changed  bool
	Files    bool
	Rules    bool
	State    bool
	Ignored  string
	Real     []string
	Contains []string
	Text     string
	Read     string
	Run      []string
	Check    *rule.Run
	Append   *state.Entry
}

func (s Step) Valid() error { /* counts how many fields ask; more than one is an error */ }
func (s Step) None() bool   { return !s.Branch && !s.Hash && !s.Changed && ... && s.Append == nil }
```

```go
// the shell: the order of the cases matters, and Epoch and Text go "with" another field
switch {
case s.None():
case s.Branch:
	ev = ev.KnowBranch(port.Branch(ctx))
case s.Hash:
	hash, err := port.Hash(ctx)
	ev = ev.KnowHash(s.Epoch, hash, err)
case s.Contains != nil:
	ev = ev.KnowFound(s.Epoch, s.Contains, s.Text, port.Contains(ctx, s.Contains, s.Text))
// ...
}
```

### After: one type per step; illegal states cannot be written

```go
// internal/hook/hook_step.go

// Step is what the evaluation needs from the outside before it can go on; nil means the answer is ready.
//
//sumtype:decl
type Step interface{ step() }

type AskBranch   struct{}
type AskHash     struct{ Epoch int }
type AskChanged  struct{}
type AskFiles    struct{}
type AskRules    struct{}
type AskState    struct{}
type AskIgnored  struct{ Path string }
type AskReal     struct{ Paths []string }
type AskContains struct{ Epoch int; Patterns []string; Text string }
type AskRead     struct{ Epoch int; Path string }
type DoRun       struct{ Argv []string }
type DoCheck     struct{ Run rule.Run }
type DoAppend    struct{ Entry state.Entry }

func (AskBranch) step() {}
func (AskHash) step()   {}
// ... one per type
```

```go
// the shell: each case carries exactly its data
for step := Decide(ev); step != nil; step = Decide(ev) {
	switch s := step.(type) {
	case AskBranch:
		ev = ev.KnowBranch(deps.Tree.Branch(ctx))
	case AskHash:
		hash, err := deps.Tree.Hash(ctx)
		ev = ev.KnowHash(s.Epoch, hash, err)
	case AskContains:
		ev = ev.KnowFound(s.Epoch, s.Patterns, s.Text, deps.Tree.Contains(ctx, s.Patterns, s.Text))
	case DoRun:
		ev = ev.Note(deps.Runner.Run(ctx, s.Argv))
	case DoAppend:
		ev = ev.Note("", deps.Log.Append(ctx, s.Entry))
	// ...
	}
}
```

### What it gains

- `Step.Valid()` and `Step.None()` disappear: the type guarantees "exactly one", and "none" is `nil`.
- `Epoch` and `Text` exist only in the steps that use them.
- With the linter `gochecksumtype` (shipped with golangci-lint) and the annotation `//sumtype:decl`, a `switch`
  that forgets a kind of step fails the lint. A new step is no longer a manual search through six files: the linter
  points at the places.

### Rules it conflicts with

GOARCH-26: every exported struct of the domain would need a `Valid()` that always returns nil. The types that
implement a sealed interface would need an exception.

### Alternative B: a read port in the logic

It changes more. The reads go through a lazy cache, and the logic only returns effects:

```go
// the logic reads facts on demand; the cache asks once and remembers
type Facts interface {
	Branch() string
	Changed() []string
	Hash() (string, error)
	// ...
}

// for each entry of the workflow: a pure decision on the facts
func decide(entry workflow.Entry, facts Facts) Action // Action: Allow, Deny, Run, Check, Append

// the shell: walks the entries once, without journal nor replay
for _, e := range wf.On[event] {
	switch a := decide(e, cache).(type) {
	case Run:
		out, err := deps.Runner.Run(ctx, a.Argv)
		cache.Invalidate() // a run may change the hash and the files
		// ...
	}
}
```

`Journal`, `Epoch`, `StateAt` and the replay from the start disappear. The price: the logic is no longer strictly
pure (it reads through an interface), and its tests need a fake of `Facts`.

## Order

Start with option A of recommendation 3, which is incremental, and weigh option B only if the hook keeps growing.
Recommendations 1 and 2 go together: the subpackage gives each use case its own namespace for its ports, and the
shared adapter of `internal/repo` replaces the repeated methods of the adapters.
