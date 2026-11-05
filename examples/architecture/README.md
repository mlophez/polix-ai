# Architecture example: the use case `CheckRule`

A draft of the target layout, applied to one use case (`polix rules check`) before changing the real code. It
does not compile. The rules agreed so far are in `rules.yaml`.

## Layout

```
internal/rule/               domain package, named as a noun
  check_rule.md              documentation of the use case CheckRule
  check_rule_in.go           entry and shell: CheckRuleCommand, CheckRuleRequest, CheckRuleResponse,
                             CheckRulePort, checkRule
  check_rule_logic.go        pure logic: CheckRule and its decision, CheckRuleOutput
  check_rule_in_test.go      tests of the shell, with a fake of the port
  check_rule_logic_test.go   tests of the logic, domain values in and the output out, without fakes
  check_rule_out.go          CheckRuleAdapter: files, git and processes behind the port
  rule.go                    domain: the entity Rule (Valid, HasCheck) and Matching
  rule_test.go               tests of rule.go
  rule_parse.go              domain: the format of ai/rules/*.yaml (Parse)
  rule_parse_test.go         tests of rule_parse.go
  rule_result.go             domain: Result, the outcome of a check (Valid, Passed, Tail) and Failed
  rule_result_test.go        tests of rule_result.go
cmd/polix/
  main.go                    wires the root command and the entries of each use case
```

## Flow of `polix rules check`

1. `CheckRuleCommand` turns the flags into a `CheckRuleRequest` and injects `CheckRuleAdapter{Repo: repo}` as the
   `CheckRulePort`.
2. `checkRule` (shell) reads through the port: the rules and, with `--changed`, the changed files.
3. `CheckRule` (logic) decides which checks run: `CheckRuleOutput`.
4. `checkRule` carries out the output: it runs each check through the port and builds a `rule.Result` (domain)
   from each run, followed by `Valid()`.
5. `checkRule` builds the `CheckRuleResponse` from the results.
6. `CheckRuleCommand` serializes the response to the console and sets the exit code.

## Conventions

- **Domain**: `<noun>.go` and `<noun>_*.go`, without a known suffix; shared by the whole app.
- **Domain values**: exported structs with exported fields, open to read. They are built with a literal, never a
  constructor, followed by `Valid() error`; after that they change only through their methods, which have a
  value receiver and return a copy, named after an intent of the use cases: no setters nor getters.
- **Use case**: `<Verb><Noun>` with files of the same name that sort from input to output, whatever the
  transport: `<verb>_<noun>.md`, `<verb>_<noun>_in.go`, `<verb>_<noun>_logic.go`, `<verb>_<noun>_out.go`.
- **Pure core, imperative shell**: the shell, in the in file, is the only one that holds the port. Every use case
  has the same shape: the shell reads through the port, calls the logic once, carries out its output through the
  port and builds the response.
- **Logic**: one function `<Verb><Noun>(domain values) <Verb><Noun>Output`; the output is its decision. What is
  decided after the input and output of the shell is domain behavior (`rule.Result.Tail`, `rule.Failed`).
- **Request and response**: every use case has a `<Verb><Noun>Request` and a `<Verb><Noun>Response` in the in
  file, the only types with the shape of the transport; the command serializes the response to the console.
- **Port**: one interface per use case, declared in the in file where it is used; its methods take
  `context.Context` first and speak in domain values.
- **Adapter**: the out file holds only the public struct `<Verb><Noun>Adapter`, which implements the port, its
  methods and unexported constants; no constructor: the entry fills its fields and passes it as the port, where
  the compiler checks that it implements it. Its exported methods are those of the port; its helpers are
  unexported methods, or live in `pkg/` when shared.
- **Vertical slices**: a use case never calls another; it uses its own files, the domain of any package and `pkg/`.
- **Names**: every top-level name of the files of a use case starts with its name (`CheckRuleFailed`,
  `checkRuleTimeout`); the shell is `<verb><Noun>(ctx, port, req) (<Verb><Noun>Response, error)`, the only one
  that calls the logic, once, and the port.
- **Tests**: `<stem>_test.go` next to the `<stem>.go` it tests; each use case has `_logic_test.go` (domain values,
  no fakes) and `_in_test.go` (the shell with a fake of the port).

## Open decisions

1. `workflow.Bases` (the base asked, else those of polix.yaml) is domain of workflow, which is not in this
   example: the shell needs it before calling the logic, to read the changed files.
2. `rules` (the parent command of `check`) is built in `cmd/polix/main.go`.
3. `CheckRuleAdapter` reads polix.yaml itself and uses `workflow.Parse` and `BaseBranches`, so both must be in
   the domain of workflow.
4. The ui, the exit codes and the flag `--dir` move to `pkg/cli`, shared by the entries of every use case.
5. Running scripts is needed by hook too: the process part (timeout, interpreter on Windows) could move to
   `pkg/`, with each adapter wrapping it.
6. `Parse` is domain because the format of ai/rules/ belongs to polix; the yaml stays out of the entity Rule.
