# Architecture of the Go code of polix

The depth of a folder gives the kind of its code, and its width the growth of the system. `internal/app/` holds
what polix does: each concept of the domain is a folder named as a noun, each command a use case folder inside its
noun, named as a verb, and a use case that grows too big splits into helper folders of its own, one level and no
more. `internal/platform/` holds the technology polix runs on (git, processes, files, the console), which never
depends on the product. Growing is adding sibling folders, never deeper ones, and everything lives under the
`internal/` of Go, so no other project imports polix by accident.

Each use case is an entry, a pure logic and an adapter (pure core, imperative shell), with the same short names in
every use case package (`Command`, `Request`, `Response`, `Port`, `Adapter`, `Decide`, `Output`), read with their
noun and verb outside it (`rulecheck.Command`). The rules are GOARCH-1 to GOARCH-43 of
`ai/rules/architecture.yaml` (GOARCH-31 and GOARCH-40 are retired in `ai/rules/retired.yaml`); the deterministic
ones are decided by the AST tests of `tests/architecture/` (`go test ./tests/architecture`, one `TestGOARCH<N>` per
rule), which read `internal/` and `cmd/`, and the fixed lists of imports by depguard, which those tests run (see
[How the rules are decided](#how-the-rules-are-decided)).

The domains are `agent` (the events of Claude Code, with a Codex adapter that is not implemented), `diff`
(`risk`), `hook` (`hook`), `rule` (`rules check`), `setup` (`config check`, `install`, `uninstall`, `doctor`, the
hidden `__refresh`), `state` (the log of a repository), `version` (`version`, `update`) and `workflow`
(`polix.yaml`, `config fmt`, `graph`, `log`). How polix behaves is in `docs/polix.md`.

## Layout

```
cmd/polix/
  main.go                    declares only main: builds the root command with internal/platform/cli, adds the
                             entry of every use case, imported as <noun><verb> (and the parent commands config
                             and rules), and exits with the code of cli.Run
internal/app/                what polix does: only domain folders
  <noun>/                    domain package, named as a noun, shared by the whole app
    <noun>.go                domain: the entity <Noun>
    <noun>_*.go              the rest of the domain
    <verb>/                  use case <Verb><Noun>, package <verb>
      <verb>.md              documentation of the use case, titled # <Verb><Noun>
      in.go                  entry and shell
      logic.go               pure logic
      out.go                 adapter behind the port
      <helper>/              a part of this use case alone, package <helper>: <helper>.go, <helper>_*.go and
                             their tests; no folder below
internal/platform/           the technology polix runs on, without domain: only library folders, no folder below
                             them; never imports internal/app/
  build/                     the version of the binary: Version, set by the linker, and Current
  cli/                       the ui, the exit code error, the flag --dir, the root command and Run (with fang)
  file/                      files as the agents expect them: a missing one is empty, atomic writes that keep
                             symlinks and modes, a folder synced to a set of files, the cache folder
  git/                       the repository: root, branch, changed files, base branch, content hash, the paths
                             that differ between two trees
  github/                    the releases of a repository: latest tag, checked download, replace of a binary
  glob/                      paths and command lines matched against patterns
  jsonl/                     files of one record per line: bounded reading and appending
  proc/                      running an argv with a deadline, and the interpreter of a file on Windows
  semver/                    versions of releases: parse and compare
  shell/                     a command line split into the commands it runs
  testenv/                   support for the tests: files in one call, a repository with one commit, a machine
                             of its own with the fake CLIs of the agents; only _test.go files import it
  text/                      texts cut to a byte limit, and the last lines of a text
  yamlfmt/                   a YAML document formatted in one layout, keeping its comments
tests/architecture/          the AST tests of the GOARCH rules, and the tests that run depguard
tests/install/               the Go tests of install.sh and install.ps1
tests/setup/                 the end-to-end tests of the commands of setup, composed as cmd/polix composes them
```

Every package clause equals the name of its folder, `*_test.go` sits next to the file it tests, and a `testdata/`
folder is admitted at any level. A new command is a new `<verb>/`, a new concept a new `<noun>/`, a new technology
a new `<lib>/`, and a use case too big a new `<helper>/`; no use case has a helper yet. The use cases are:

- `agent`: none; `hook` and `AnswerHook` use its domain.
- `diff`: `measure/` (`MeasureDiff`, `polix risk`).
- `hook`: `answer/` (`AnswerHook`, `polix hook`).
- `rule`: `check/` (`CheckRule`, `polix rules check`).
- `setup`: `check/` (`CheckSetup`, `polix config check`), `diagnose/` (`DiagnoseSetup`, `polix doctor`),
  `install/` (`InstallSetup`), `refresh/` (`RefreshSetup`, the hidden `polix __refresh`) and `uninstall/`
  (`UninstallSetup`).
- `state`: none; `hook`, `AnswerHook`, `DiagnoseSetup` and `LogWorkflow` use its domain.
- `version`: `show/` (`ShowVersion`, `polix version`) and `update/` (`UpdateVersion`).
- `workflow`: `format/` (`FormatWorkflow`, `polix config fmt`), `graph/` (`GraphWorkflow`) and `log/`
  (`LogWorkflow`).

## Direction of the imports

The direction of every import follows from the depth (GOARCH-9, unless another rule is named):

- A use case imports the domains of any noun (up and across) and its own helpers (the only way down).
- A use case never imports another use case, of its noun or of another.
- Of `internal/app/`, a domain imports only other domains, never a use case nor a helper.
- A helper is imported only by the files of its use case package, tests included: no other helper, use case,
  domain nor `cmd/` imports it.
- Only `cmd/` and `tests/` (the end-to-end tests that compose several commands) import a use case.
- `internal/platform/` never imports `internal/app/` (GOARCH-42); the domain and the logic import only the pure
  libraries of `internal/platform/` (GOARCH-41), and only test files import `internal/platform/testenv`
  (GOARCH-43).
- No import cycle anywhere (GOARCH-7).

The test files count as their package.

## The example of CheckRule

The use case `CheckRule` (`polix rules check`), in `internal/app/rule/check/`, with its domain:

```
internal/app/rule/
  rule.go                    domain: the entity Rule (Valid, HasCheck)
  rule_parse.go              domain: the format of ai/rules/*.yaml (Parse, ParseFiles)
  rule_result.go             domain: Outcome, the output of a run, and Result, the outcome of a check (Valid,
                             Passed, Tail), and Failed
  rule_run.go                domain: Run, a check with the argv of its runner (Valid, Settle), and Plan
  rule_set.go                domain: Matching and Repeated, on a list of rules
  check/                     the use case CheckRule, package check, imported as rulecheck
    check.md                 documentation of the use case, titled # CheckRule
    in.go                    entry and shell: Command, Request, Response, Failed, Port, flags, run, response
    logic.go                 pure logic: Decide and its decision, Output
    out.go                   Adapter: files, git and processes behind the port
    in_test.go               tests of the shell, with a fake of the port
    logic_test.go            tests of the logic, domain values in and the output out, without fakes
    out_test.go              tests of the adapter, against a temporary repository
```

`cmd/polix/main.go` imports it as `rulecheck` and adds `rulecheck.Command()` under the parent command `rules`.

## Flow of a use case

The flow of `polix rules check`:

1. `Command` checks the flags, turns them into a `Request` and injects `Adapter{Repo: repo}` as the `Port`.
2. `run` (shell) reads through the port: polix.yaml whenever it exists (`LoadWorkflow`, for the runners of
   `rules.run`), the rules of its `rules.path` (`LoadRules`) and, with `--changed`, the changed files against its
   base branches (`Changed`), which `--base` only replaces when given.
3. `Decide` (logic) takes the outcomes carried out so far and decides the next run, which may decide several rules
   at once, or, when none is left, the result of every rule: `Output` (the step shape).
4. `run` carries out each next run through the port (`RunCheck`), builds a `rule.Outcome` (domain) from it,
   followed by `Valid()`, and calls the logic again, in a loop, until it has no next run.
5. `run` builds the `Response` from the results.
6. `Command` serializes the response to the console and sets the exit code.

## Conventions

- **Depth**: the folder of a file says its kind: `internal/app/<noun>/` a domain, `internal/app/<noun>/<verb>/` a
  use case, `internal/app/<noun>/<verb>/<helper>/` a part of one use case, `internal/platform/<lib>/` a library;
  nothing deeper, `testdata/` aside (GOARCH-1). A domain is named after its entity, a use case after what it does
  to its noun, a helper after its role in its use case, a library after its technology (GOARCH-17).
- **Domain**: `<noun>.go` and `<noun>_*.go` in `internal/app/<noun>/`; shared by the whole app. It imports only
  other domains, the pure libraries of `internal/platform/`, yaml, the templates and the standard library without
  what touches the outside (GOARCH-41). Its methods live in its package, as Go requires. Its package variables are
  tables, built once and never written, or sentinel errors.
- **Domain values**: exported structs with exported fields, open to read. They are built with a literal, never a
  constructor, followed by `Valid() error`; after that they change only through their methods, which have a
  value receiver and return a copy, named after an intent of the use cases: no setters nor getters.
- **Use case**: `<Verb><Noun>` is the folder `internal/app/<noun>/<verb>/`, package `<verb>`, with files that read
  from input to output: `<verb>.md`, `in.go`, `logic.go`, `out.go`.
- **Helper**: a part of one use case that grows, in `internal/app/<noun>/<verb>/<helper>/`: logic, input or
  output, imported only by its use case package. A helper that the logic imports is as pure as the logic
  (GOARCH-4). Logic shared by several use cases goes in the domain instead (GOARCH-10).
- **Library**: technology without domain in `internal/platform/<lib>/`, which the adapters wrap instead of each
  repeating it, and the support that the tests share (`testenv`); it takes the values it needs as parameters and
  never imports `internal/app/`.
- **Pure core, imperative shell**: the shell, in `in.go`, is the only one that holds the port. Every use case has
  the same shape: the shell reads through the port what the logic needs (branching on the request to know what to
  read), calls the logic from one call site (once, or in a loop in the step shape), carries out its output through
  the port, step by step, and builds the response. What the result of a step means is asked to the domain, never
  computed in the shell: its conditions only mention the request, errors, and fields or methods of the domain and
  the output.
- **Step shape**: when what the logic decides depends on the result of an earlier step (`polix hook`, where a
  later entry of an event sees what an earlier one did), the logic returns the next step instead of the whole
  output. The shell carries the step out through the port and calls the logic again with its result, from one
  call site in a loop, until the logic answers with no step. Only use cases whose plan depends on the results of
  earlier steps take this shape; the review rejects it elsewhere.
- **Logic**: one function `Decide(domain values) Output` in `logic.go`, which exports nothing else; the output is
  its decision and a refusal is data of it, never an error. What is decided after the input and output of the
  shell is domain behavior (`rule.Result.Tail`, `rule.Failed`).
- **Request and response**: every use case has a `Request` and a `Response` in `in.go`, the only types with the
  shape of the transport; the entry validates the flags and serializes the response to the console.
- **Port**: one interface per use case, `Port`, declared in `in.go` where it is used; its methods take
  `context.Context` first and speak in domain values.
- **Adapter**: `out.go` holds only the public struct `Adapter`, which implements the port, its methods and
  unexported constants; no constructor: the entry fills its fields and passes it as the port, where the compiler
  checks that it implements it. Its exported methods are those of the port; its helpers are unexported methods,
  live in a helper of the use case, or in `internal/platform/` when shared. Concurrency and deadlines live here.
- **Vertical slices**: a use case never imports another; it uses its own files and helpers, the domain of any
  noun, `internal/platform/` and packages outside the module.
- **Names**: the names of a use case are fixed and the same in every use case package: the entry `Command`, the
  only exported function of `in.go`; `Request`, `Response`, `Port`; the shell
  `run(ctx, port, req) (Response, error)`, the only one that calls the logic, from one call site, and the port;
  `Decide` and `Output` in `logic.go`; `Adapter` in `out.go`. Every other name is short and says its role, without
  the name of the use case (`Failed`, `timeout`): the package already says which use case it belongs to. A domain
  or a helper never declares a name reserved to the use cases (`Command`, `Request`, `Response`, `Port`,
  `Adapter`, `Decide`, `Output`, or any `*Output`, `*Request`, `*Response`, `*Port`, `*Adapter`; GOARCH-23).
- **Imports**: a use case is always imported with the alias `<noun><verb>` (`internal/app/rule/check` as
  `rulecheck`), so its names read with their noun and verb (`rulecheck.Command`) and two use cases with the same
  verb (`rulecheck`, `setupcheck`) never clash. The linter `importas` of `.golangci.yml` checks it with one
  pattern for every use case, without listing them, and fails both a missing alias and a different one. The
  domains and the libraries are imported without alias: with `no-unaliased`, a pattern for them would demand an
  alias on every import of them.
- **Tests**: white box, in the package of the folder; `<stem>_test.go` next to the `<stem>.go` it tests. Each
  use case has `logic_test.go` (domain values, no fakes, nothing from `in_test.go`), `in_test.go` (the shell with
  a fake of the port) and, when the adapter does more than call a library, `out_test.go` (the adapter against a
  temporary repository). The tests of a use case never import another use case: the end-to-end tests that compose
  several live in `tests/<noun>/` (`tests/setup/`), and the support the tests share in `internal/platform/testenv`.
- **Errors**: each port method returns an error wrapped with what failed (the file, the command, the branch); the
  shell returns it unchanged; the entry prints it through the ui of `internal/platform/cli` and exits with 1,
  unless the command has its own policy.

## How the rules are decided

- **AST tests**: each deterministic rule that depends on the structure has its `TestGOARCH<N>` in
  `tests/architecture/goarchNN_test.go`, which classifies every folder of `internal/` and `cmd/` by its depth and
  reads the Go AST, type checked tolerantly. Its subtest `module` expects no violation on the repository, and its
  subtest `detects` expects every violation of the rule in the fixture `tests/architecture/testdata/bad`, a tree
  in the same layout with faults on purpose. The import rules that depend on the structure (GOARCH-9: a use case
  never imports another, a helper only from its use case) stay here, since depguard matches packages only by
  prefix and would need one entry per use case.
- **depguard**: the fixed lists of imports are lists of depguard in `.golangci.yml`, named after the rule they
  decide, so golangci-lint shows them while the code is edited and every finding names its rule: GOARCH-41
  (`GOARCH-41-domain` for the domain files, `GOARCH-41-logic` for every `logic.go`), GOARCH-42 and GOARCH-43. The
  purity of the domain and the logic is an allowlist: a new library of `internal/platform/` is denied to them until
  it is added on purpose. `TestGOARCH41`, `TestGOARCH42` and `TestGOARCH43` run golangci-lint with only depguard
  and the `.golangci.yml` of the repository, once per root and test binary, and keep the findings of their lists:
  `module` expects none on the repository, and `detects` expects each violation of
  `tests/architecture/testdata/depguard`, a module that compiles under the module path of polix, since
  golangci-lint loads its types. Without golangci-lint on the PATH these tests fail, never skip. GOARCH-4 reads the
  list `GOARCH-41-logic` to check the helpers that the logic imports, so the list lives in one place.
- **Review**: the rules with `deterministic: false` (GOARCH-8, 10, 17 to 22, 30) are left to the review.
- **Retired**: GOARCH-31 (every name of a use case starts with its name) and GOARCH-40 (the methods of a domain
  struct live in its domain) are in `ai/rules/retired.yaml`, with their reason: the package of each use case and
  Go itself decide them now. Their ids are never reused.

## Resolved decisions

- The ui, the exit codes and the flag `--dir` live in `internal/platform/cli`, shared by the entries of every use
  case.
- `glob` and `Cut` are technology without domain: `internal/platform/glob` and `internal/platform/text`. Git lives
  in `internal/platform/git`, and the process part (timeout, interpreter on Windows and its helpers on paths) in
  `internal/platform/proc`, which each adapter wraps.
- `workflow.Bases` (the base asked, else those of polix.yaml), `workflow.Parse` and `BaseBranches` are domain of
  workflow: the shell of `CheckRule` needs the bases before calling the logic.
- `rule.Parse` is domain because the format of ai/rules/ belongs to polix; the yaml stays out of the entity Rule.
- `config` and `rules`, the parent commands of `config check`, `config fmt` and `rules check`, are built in
  `cmd/polix/main.go`.
- The version set by the linker lives in `internal/platform/build.Version`; the entries take it from
  `internal/platform/build.Current` and pass it to their use cases in the request, so the domain never reads it.
- One package per use case, with fixed short names, instead of names prefixed with `<Verb><Noun>` (`Command`, not
  the name of the use case followed by `Command`): revive reports the prefix as stutter inside a package `check`,
  and the alias `<noun><verb>` shows the full name wherever a use case is used.
- Every library lives in `internal/platform/`, never in a public folder of the module: polix is a binary, and a
  facade for other projects would be a package outside `internal/`.
- `templates/` and `plugins/` stay at the root of the module: they embed the assets next to them, and `go:embed`
  reads only files below its package.
- The helpers of a use case sit right below it, not under a Go `internal/` of the use case: the compiler would
  enforce their visibility, but one level deeper than the layout allows; GOARCH-9 enforces it instead.
- `polix rules check` changes on purpose: a rule that is not `Valid` (no description, or a `check` without
  `deterministic`) stops the command; `--base` without `--changed` is an error; the hint to set `base_branch` only
  comes when no base exists. It always reads polix.yaml when it exists, to know the runners of `rules.run`, and
  `--base` only replaces its base branches; a polix.yaml that cannot be read or parsed stops the command.
- `polix hook` takes the step shape, since a later entry of an event sees what an earlier one did (an observed
  line it wrote, or a `run` that rewrites files and changes the content a later `finished` compares), and the
  first entry that actually blocks ends the evaluation.
