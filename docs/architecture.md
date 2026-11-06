# Architecture of the Go code of polix

The layout every command of polix follows: domain packages named as nouns, each command a use case with an entry,
a pure logic and an adapter (pure core, imperative shell). The rules are GOARCH-1 to GOARCH-40 of
`ai/rules/architecture.yaml`; the deterministic ones are decided by the AST tests of `tests/architecture/`
(`go test ./tests/architecture`, one `TestGOARCH<N>` per rule), which read `internal/`, `pkg/` and `cmd/`.

The domain packages are `agent` (the events of Claude Code and Codex), `hook` (`polix hook`), `rule`
(`polix rules check`), `setup` (`check`, `install`, `uninstall`, `doctor`, the hidden
`__refresh` of `RefreshSetup`), `state` (the log of a repository),
`version` (`version`, `update`) and `workflow` (`polix.yaml`, `graph`, `log`). How polix behaves is in
`docs/polix.md`.

## Layout

```
cmd/polix/
  main.go                    declares only main: builds the root command with pkg/cli, adds the entries of
                             every use case (and the parent command rules) and exits with the code of cli.Run
internal/<noun>/             domain package, named as a noun
  <noun>.go                  domain: the entity <Noun>
  <noun>_*.go                the rest of the domain
  <verb>_<noun>.md           documentation of the use case <Verb><Noun>
  <verb>_<noun>_in.go        entry and shell
  <verb>_<noun>_logic.go     pure logic
  <verb>_<noun>_out.go       adapter behind the port
  *_test.go                  white box tests, next to the file they test
pkg/<library>/               technology shared by several adapters, without domain; never imports internal/
  build/                     the version of the binary: Version, set by the linker, and Current
  cli/                       the ui, the exit code error, the flag --dir, the root command and Run (with fang)
  file/                      files as the agents expect them: a missing one is empty, atomic writes that keep
                             symlinks and modes, a folder synced to a set of files, the cache folder
  git/                       the repository: root, branch, changed files, base branch, content hash
  github/                    the releases of a repository: latest tag, checked download, replace of a binary
  glob/                      paths and command lines matched against patterns
  jsonl/                     files of one record per line: bounded reading and appending
  semver/                    versions of releases: parse and compare
  proc/                      running an argv with a deadline, and the interpreter of a file on Windows
  shell/                     a command line split into the commands it runs
  text/                      texts cut to a byte limit, and the last lines of a text
tests/architecture/          the AST tests of the GOARCH rules
tests/install/               the Go tests of install.sh and install.ps1
```

The use case `CheckRule` (`polix rules check`), in `internal/rule/`, as an example:

```
check_rule.md              documentation of the use case CheckRule
check_rule_in.go           entry and shell: CheckRuleCommand, CheckRuleRequest, CheckRuleResponse,
                           CheckRuleFailed, CheckRulePort, checkRuleFlags, checkRule, checkRuleResponse
check_rule_logic.go        pure logic: CheckRule and its decision, CheckRuleOutput
check_rule_in_test.go      tests of the shell, with a fake of the port
check_rule_logic_test.go   tests of the logic, domain values in and the output out, without fakes
check_rule_out.go          CheckRuleAdapter: files, git and processes behind the port
check_rule_out_test.go     tests of the adapter, against a temporary repository
rule.go                    domain: the entity Rule (Valid, HasCheck)
rule_parse.go              domain: the format of ai/rules/*.yaml (Parse)
rule_result.go             domain: Result, the outcome of a check (Valid, Passed, Tail) and Failed
rule_run.go                domain: Run, a check with the argv of its runner (Valid)
rule_set.go                domain: Matching and Repeated, on a list of rules
```

## Flow of a use case

The flow of `polix rules check`:

1. `CheckRuleCommand` checks the flags, turns them into a `CheckRuleRequest` and injects
   `CheckRuleAdapter{Repo: repo}` as the `CheckRulePort`.
2. `checkRule` (shell) reads through the port: the rules, polix.yaml whenever it exists (`LoadWorkflow`, for the
   runners of `rules.run`) and, with `--changed`, the changed files against its base branches, which `--base`
   only replaces when given.
3. `CheckRule` (logic) takes the outcomes carried out so far and decides the next run, which may decide several
   rules at once, or, when none is left, the result of every rule: `CheckRuleOutput` (the step shape).
4. `checkRule` carries out each next run through the port, builds a `rule.Outcome` (domain) from it, followed by
   `Valid()`, and calls the logic again, in a loop, until it has no next run.
5. `checkRule` builds the `CheckRuleResponse` from the results.
6. `CheckRuleCommand` serializes the response to the console and sets the exit code.

## Conventions

- **Domain**: `<noun>.go` and `<noun>_*.go`, without a known suffix; shared by the whole app. It uses only
  domain, `pkg/` and packages outside the module. Its methods live in its own files. Its package variables are
  tables, built once and never written, or sentinel errors.
- **Domain values**: exported structs with exported fields, open to read. They are built with a literal, never a
  constructor, followed by `Valid() error`; after that they change only through their methods, which have a
  value receiver and return a copy, named after an intent of the use cases: no setters nor getters.
- **Use case**: `<Verb><Noun>` with files of the same name that sort from input to output:
  `<verb>_<noun>.md`, `<verb>_<noun>_in.go`, `<verb>_<noun>_logic.go`, `<verb>_<noun>_out.go`.
- **Pure core, imperative shell**: the shell, in the in file, is the only one that holds the port. Every use case has
  the same shape: the shell reads through the port what the logic needs (branching on the request to know what to read),
  calls the logic from one call site (once, or in a loop in the step shape), carries out its output through the port,
  step by step, and builds the response. What the result of a step means is asked to the domain, never computed in the
  shell: its conditions only mention the request, errors, and fields or methods of the domain and the output.
- **Step shape**: when what the logic decides depends on the result of an earlier step (`polix hook`, where a
  later entry of an event sees what an earlier one did), the logic returns the next step instead of the whole
  output. The shell carries the step out through the port and calls the logic again with its result, from one
  call site in a loop, until the logic answers with no step. Only use cases whose plan depends on the results of
  earlier steps take this shape; the review rejects it elsewhere.
- **Logic**: one function `<Verb><Noun>(domain values) <Verb><Noun>Output`; the output is its decision and a
  refusal is data of it, never an error. What is decided after the input and output of the shell is domain
  behavior (`rule.Result.Tail`, `rule.Failed`).
- **Request and response**: every use case has a `<Verb><Noun>Request` and a `<Verb><Noun>Response` in the in
  file, the only types with the shape of the transport; the entry validates the flags and serializes the
  response to the console.
- **Port**: one interface per use case, declared in the in file where it is used; its methods take
  `context.Context` first and speak in domain values.
- **Adapter**: the out file holds only the public struct `<Verb><Noun>Adapter`, which implements the port, its
  methods and unexported constants; no constructor: the entry fills its fields and passes it as the port, where
  the compiler checks that it implements it. Its exported methods are those of the port; its helpers are
  unexported methods, or live in `pkg/` when shared. Concurrency and deadlines live here.
- **Vertical slices**: a use case never calls another; it uses its own files, the domain of any package, `pkg/`
  and packages outside the module.
- **Names**: every top-level name of the files of a use case starts with its name (`CheckRuleFailed`,
  `checkRuleTimeout`); the shell is `<verb><Noun>(ctx, port, req) (<Verb><Noun>Response, error)`, the only one
  that calls the logic, from one call site, and the port.
- **Imports**: a nested package of the module, `internal/<parent>/<child>` or `pkg/<parent>/<child>`, is always
  imported with the alias `<parent><child>` (`internal/rules/check` as `rulescheck`), so the qualifier tells
  where a name comes from even when several packages share the last element of their path. The linter
  `importas` of `.golangci.yml` checks it with one pattern for every such package, without listing them, and
  fails both a missing alias and a different one.
- **Tests**: white box, in the package of the folder; `<stem>_test.go` next to the `<stem>.go` it tests. Each
  use case has `_logic_test.go` (domain values, no fakes, nothing from `_in_test.go`), `_in_test.go` (the shell
  with a fake of the port) and, when the adapter does more than call a library, `_out_test.go` (the adapter
  against a temporary repository).
- **Errors**: each port method returns an error wrapped with what failed (the file, the command, the branch); the
  shell returns it unchanged; the entry prints it through the ui of `pkg/cli` and exits with 1, unless the
  command has its own policy.

## Resolved decisions

- The ui, the exit codes and the flag `--dir` live in `pkg/cli`, shared by the entries of every use case.
- `glob` and `Cut` are technology without domain: `pkg/glob` and `pkg/text`. Git lives in `pkg/git`, and the
  process part (timeout, interpreter on Windows and its helpers on paths) in `pkg/proc`, which each adapter wraps.
- `workflow.Bases` (the base asked, else those of polix.yaml), `workflow.Parse` and `BaseBranches` are domain of
  workflow: the shell of `CheckRule` needs the bases before calling the logic.
- `rule.Parse` is domain because the format of ai/rules/ belongs to polix; the yaml stays out of the entity Rule.
- `rules`, the parent command of `rules check`, is built in `cmd/polix/main.go`.
- The version set by the linker lives in `pkg/build.Version`; the entries take it from `pkg/build.Current` and
  pass it to their use cases in the request, so the domain never reads it.
- `polix rules check` changes on purpose: a rule that is not `Valid` (no description, or a `check` without
  `deterministic`) stops the command; `--base` without `--changed` is an error; the hint to set `base` only comes
  when no base exists. It always reads polix.yaml when it exists, to know the runners of `rules.run`, and
  `--base` only replaces its base branches; a polix.yaml that cannot be read or parsed stops the command.
- `polix hook` takes the step shape, since a later entry of an event sees what an earlier one did (a `record`,
  or a `run` that changes the hash `done` is pinned to), and `allow` and `deny` stop the evaluation.
