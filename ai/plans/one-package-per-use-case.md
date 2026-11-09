# Split internal/ into app and platform, one package per use case, with the depth of a folder as its kind

## Overview

The code of polix gets a layout where the depth of a folder says what kind of code it holds, and its width says how
much the system has grown.
Everything moves under the private folder of Go, so nothing of polix can be imported by other projects by accident.

- One folder holds what polix does (its domain and its commands), another the technology it runs on (git, processes,
  files, the console); the technology never depends on the product.
- Inside the first, each concept of the domain is a folder, each command is a folder inside its concept, and a
  command that grows too big may split into helper folders of its own, one level and no more.
- The compiler and a few import rules enforce what many rules only checked by file names today: a command cannot
  use another command, a concept cannot depend on its commands, and the helpers of a command belong only to it.
- Inside each command the names are short and the same everywhere (Command, Request, Port, Adapter, Decide);
  outside they always read with their concept and command, such as rulecheck.Command.
- The rules on imports that are plain lists move to the linter, so the agent sees them while it edits, and polix
  still decides them through a test that runs the linter.
- Nothing polix does changes: every command, flag, message, exit code and the version of the release stay the same.

## Context

Use cases share the Go package of their domain, so polix needs rules on file names, symbol prefixes and
same-package references (GOARCH-1, 9, 10, 11, 31, 40) that package boundaries would give for free.
`pkg/` holds technology without domain but is public API of the module, which polix, a binary, does not intend.
The outcome is `internal/app/<noun>/<verb>/[<helper>/]` and `internal/platform/<lib>/`, the GOARCH rules and their
tests rewritten on the depth of each folder, the fixed import lists in depguard, and the binary unchanged.

## Decisions

- The depth of a folder gives the kind of its code and the width of a folder its growth, as the user stated: the
  architecture tests classify by depth, and growing is adding sibling folders.
- `internal/app/` for the domains and their use cases and `internal/platform/` for the technical libraries, as the
  user chose; `pkg/` disappears.
- One package per use case at `internal/app/<noun>/<verb>/`, package `<verb>`, as the user chose.
- A use case may split into helper packages one level below it, `internal/app/<noun>/<verb>/<helper>/`, as the
  user asked; a helper may hold any part of its use case, logic or input and output, as the user chose.
- A helper is imported only by the files of its own use case package, never by another helper, as the user chose.
- `internal/platform/<lib>/` has no level below it, except `testdata/`, as the user asked.
- Files keep the vocabulary of the rules: `in.go`, `logic.go`, `out.go`, their tests `in_test.go`, `logic_test.go`,
  `out_test.go`, and the document `<verb>.md`, as the user chose.
- Fixed names inside a use case package, as the user chose: `Command`, `Request`, `Response`, `Port`, `Adapter`,
  the logic `Decide` with `Output`, and the shell `run`.
- Full names such as `CheckRuleCommand` were discarded: revive reports them as stutter inside a package `check`, and
  the alias `<noun><verb>` that `importas` enforces shows the full name wherever a use case is used.
- `importas` keeps one pattern, for the use cases only: with `no-unaliased`, a pattern for the domains or the
  libraries would force an alias equal to the package name on every import of them, as a trial showed.
- Only moves and renames in the code: one port per use case and the current shape of `Step` stay, as the user chose.
- The fixed import lists go to depguard in `.golangci.yml`, and each of their rules keeps a check: a test of
  `tests/architecture/` that runs golangci-lint with only depguard and keeps the findings of its list, as the user
  chose.
- The lists of depguard are named after the rule they decide (`GOARCH-41-domain`), so a finding names its rule.
- The import rules that depend on the structure (a use case never imports another, a helper only from its use case)
  stay in the AST tests, since depguard matches packages only by prefix and would need one entry per use case.
- The purity of the domain and the logic on imports becomes an allowlist instead of a list of forbidden packages:
  a new library of `internal/platform/` is forbidden to them until it is added on purpose.
- The direction of every import follows from the depth, in one rule (GOARCH-9): a use case imports domains of any
  noun and its own helpers, a domain never imports a use case or a helper, a helper only its use case may import,
  only `cmd/` and `tests/` import use cases, and `internal/platform/` never imports `internal/app/` (GOARCH-42, in
  depguard).
- The end-to-end tests that compose several use cases live in `tests/<noun>/`, as `tests/install/` does, so the
  tests of a use case never import another, as the user chose.
- The document of a use case keeps its title `# <Verb><Noun>` (`# CheckRule`), built from the noun and the verb.
- Assumption: the document is named `<verb>.md`, the file named after its folder, as `<noun>.go` is for the domain.
- Assumption: GOARCH-31 and GOARCH-40 are retired to `ai/rules/retired.yaml` with their reason, since the package
  namespace and the Go rule that methods live in the package of their type now decide them.
- Assumption: GOARCH-10 keeps only its part no test decides (logic shared by several use cases goes in the domain,
  the part of one use case in its helpers) and becomes non-deterministic; its import part joins GOARCH-9.
- Assumption: the decoding of the hooks of Claude Code that the logic of `doctor` does on unexported JSON types
  moves to an exported domain function of `setup` that returns tagless domain values; GOARCH-16 forbids exporting
  the JSON types.
- Assumption: the helpers that the tests of the five setup use cases share move to `internal/platform/testenv`,
  with the fake CLI of the agents, and a depguard rule (GOARCH-43) allows it only in `_test.go` files.
- Assumption: `archRules`, the only helper a test of `CheckRule` borrows from a domain test, is copied, being four
  lines.
- Assumption: `templates/` and `plugins/`, the Go packages at the root that embed the assets next to them, stay
  where they are, since `go:embed` reads only files below its package.
- `AGENTS.md` is a protected file of `polix.yaml`: its stale Layout section is left to a person (see Out of scope).

## Approach

### Layout by depth

The depth of a folder below `internal/` gives the kind of its code, and each level admits a closed set of children:

- `internal/app/`: what polix does; it holds only domain folders.
- `internal/app/<noun>/`: a domain package, shared by the whole app; it holds `<noun>.go`, `<noun>_*.go`, their
  tests, `testdata/` and use case folders.
- `internal/app/<noun>/<verb>/`: a use case, package `<verb>`; it holds `in.go`, `logic.go`, `out.go`, `<verb>.md`,
  their tests, `testdata/` and helper folders.
- `internal/app/<noun>/<verb>/<helper>/`: a part of one use case, package `<helper>`; it holds `<helper>.go`,
  `<helper>_*.go`, their tests and `testdata/`, and no folder below.
- `internal/platform/`: the technology polix runs on; it holds only library folders.
- `internal/platform/<lib>/`: a library without domain, package `<lib>`; it holds Go files, their tests and
  `testdata/`, and no folder below.

Every package clause equals the name of its folder.
Growth is width: a new command is a new `<verb>/`, a new concept a new `<noun>/`, a new technology a new `<lib>/`, a
use case too big a new `<helper>/`.

The current packages map as follows.
The domains `agent`, `diff`, `hook`, `rule`, `setup`, `state`, `version` and `workflow` move to
`internal/app/<noun>/`.
The libraries `build`, `cli`, `file`, `git`, `github`, `glob`, `jsonl`, `proc`, `semver`, `shell`, `text` and
`yamlfmt` move from `pkg/<lib>/` to `internal/platform/<lib>/`, and `testenv` is new there.
The thirteen use cases move to:

- `CheckRule`: `internal/app/rule/check/`.
- `CheckSetup`, `DiagnoseSetup`, `InstallSetup`, `RefreshSetup`, `UninstallSetup`: `internal/app/setup/check/`,
  `internal/app/setup/diagnose/`, `internal/app/setup/install/`, `internal/app/setup/refresh/`,
  `internal/app/setup/uninstall/`.
- `FormatWorkflow`, `GraphWorkflow`, `LogWorkflow`: `internal/app/workflow/format/`, `internal/app/workflow/graph/`,
  `internal/app/workflow/log/`.
- `ShowVersion`, `UpdateVersion`: `internal/app/version/show/`, `internal/app/version/update/`.
- `MeasureDiff`: `internal/app/diff/measure/`.
- `AnswerHook`: `internal/app/hook/answer/`.

No helper package is created in this change: the layout and the rules admit them, and the plans that break up the
big use cases will use them.

### Direction of the imports

The direction of an import follows from the depth (GOARCH-9, AST test):

- A use case imports the domains of any noun (up and across) and its own helpers (the only way down).
- A use case never imports another use case, of its noun or of another.
- A domain imports only other domains, never a use case nor a helper; Go already rejects the cycle with its own.
- A helper is imported only by the non-test and test files of its use case package; no other helper, use case,
  domain nor `cmd/` imports it.
- Only `cmd/` and `tests/` (the end-to-end tests that compose several commands) import use case packages.
- `internal/platform/` never imports `internal/app/` (GOARCH-42, depguard); `internal/app/` imports the libraries
  within the allowlist of GOARCH-41.
- No import cycle anywhere (GOARCH-7, AST test).

### Imports of the domain and the logic (GOARCH-41, depguard)

The domain files (`internal/app/*/*.go` without tests) and the logic files (`internal/app/*/*/logic.go`) import only
the standard library without what touches the outside, the randomness and the logs (`os`, `os/exec`, `os/signal`,
`net`, `net/http`, `io/fs`, `runtime`, `syscall`, `math/rand`, `math/rand/v2`, `crypto/rand`, `log`, `log/slog`),
the module under `internal/app/`, the pure libraries `glob`, `semver`, `shell`, `text` and `yamlfmt` of
`internal/platform/`, `gopkg.in/yaml.v3`, and `github.com/mlophez/polix-ai/templates`.
The logic files also never import `context`.
These are the imports the domain and the logic use today, measured on the code.
depguard gives `deny` precedence over `allow`, as a trial showed, so the denied standard packages are listed under
`deny` and everything else allowed under `allow`, with no prefix of `internal/platform` under `deny`.
When `logic.go` imports a helper of its use case, that helper must pass GOARCH-4 and the list `GOARCH-41-logic`;
the AST test of GOARCH-4 reads that list from `.golangci.yml`, so the list lives in one place.

### Tests that run depguard

A new helper of `tests/architecture/` runs golangci-lint once per test binary with only depguard on a root
(`--enable-only depguard`, the configuration of the repository), and gives each test the findings of its lists.
Each of `TestGOARCH41`, `TestGOARCH42` and `TestGOARCH43` keeps the two subtests of `checkRule`: `module` expects no
finding of its lists on the repository, and `detects` expects the findings of its lists on a fixture.
The fixture is a new tree `tests/architecture/testdata/depguard/` that compiles, since golangci-lint loads types:
its `go.mod` declares the module path of polix, so the prefixes of the configuration match, and holds one violation
per list.
Without golangci-lint on the PATH the tests fail with a message that says how to install it; they never skip.

### Mapping of names

Inside the package of a use case, `<Verb><Noun>` drops from every name:
`<Verb><Noun>Command` becomes `Command`, `<Verb><Noun>Request` becomes `Request`, `<Verb><Noun>Response` becomes
`Response`, `<Verb><Noun>Port` becomes `Port`, `<Verb><Noun>Adapter` becomes `Adapter`, the logic `<Verb><Noun>`
becomes `Decide`, `<Verb><Noun>Output` becomes `Output`, and the shell `<verb><Noun>` becomes `run`.
Every other name drops its prefix too (`CheckRuleFailed` becomes `Failed`, `MeasureDiffRow` becomes `Row`,
`LogWorkflowEntry` becomes `Entry`, `checkRuleTimeout` becomes `timeout`).
When the bare name clashes with another name of the package, a builtin or an imported package, it takes a short
name of its role instead (`answerHookRun`, the state of one pass of the evaluation, becomes `pass`, since `run` is
the shell).
The fakes of the tests follow the same mapping (`fakeCheckRulePort` becomes `fakePort`).
Names of the domain are written with their qualifier (`rule.Rule`, `workflow.Workflow`); no domain name changes,
except the ones Task 1 exports.
`cmd/polix/main.go` imports each use case with the alias `<noun><verb>` (`rulecheck`, `setupcheck`,
`workflowlog`, `hookanswer`) and wires `<alias>.Command()`; the domains and the libraries are imported without alias.
The names reserved to use cases (`Command`, `Request`, `Response`, `Port`, `Adapter`, `Decide`, `Output`, and any
`*Output`, `*Request`, `*Response`, `*Port`, `*Adapter`) are never declared by a domain nor a helper (GOARCH-23).

### Domain names the use cases borrow today

A compiler-based scan of the use case files found the only unexported domain names they use:
`hookGroup`, `hooksFileJSON`, `claudeHookEvents` (in `internal/setup/setup_hooks.go`) and `found`
(in `internal/setup/setup_finding.go`), used by `DiagnoseSetup` and the tests of `CheckSetup`;
and `shortcutProtectedFiles`, `shortcutForbiddenCommands` (in `internal/workflow/workflow_shortcuts.go`), used by
`GraphWorkflow`.
The tests of the setup use cases also use `read`, `write`, `calls`, `gitRepo`, `machine`, `home` and the fake CLI
built by `TestMain` of `internal/setup/setup_test.go`, and a test of `CheckRule` uses `archRules` of
`internal/rule/rule_run_test.go`.
Task 1 and Task 3 resolve them before the use cases move, so the moves only rename.

### Architecture rules and their tests

`tests/architecture/module_test.go` loads `internal/` and `cmd/` and classifies each folder by its depth (see Layout
by depth) instead of by the suffix of its file names; the helpers on stems (`useCaseSuffixes`, `goFile.useCase`,
`useCase.stem`, `lowerCamel`) give way to the noun, the verb and the helper of the path.
Each rule keeps its intent with the fixed names, and the rules change as follows:

- GOARCH-1: the layout by depth, its closed set of children per level, the maximum depth and the package clause.
- GOARCH-2: only domain packages need their entity.
- GOARCH-3: each use case folder has `in.go`, `logic.go` and `out.go`.
- GOARCH-4: the purity checks that are not imports stay (clock, console, goroutines, panic, state); the logic uses no
  name declared in `in.go` or `out.go`; a helper the logic imports passes GOARCH-4 and the list `GOARCH-41-logic`.
- GOARCH-7: no import cycle among the packages of the module.
- GOARCH-8: the permission, reworded on folders.
- GOARCH-9: the direction of the imports by depth, as above.
- GOARCH-10: non-deterministic, on where shared logic goes.
- GOARCH-11, 12, 15, 24, 32, 33, 34, 36, 37: the same checks on the fixed names.
- GOARCH-14: the document is `<verb>.md` with the title `# <Verb><Noun>` built from the path.
- GOARCH-17: nouns for domains, verbs for use cases, the role for helpers, the technology for libraries.
- GOARCH-19: `internal/platform/` holds technology without domain that adapters wrap, and support for tests.
- GOARCH-20, 22: their scopes on the new files.
- GOARCH-23: the logic exports only `Decide` and `Output`; the names reserved to use cases, bare ones included, are
  never declared by a domain nor a helper.
- GOARCH-34: `Port` and `Adapter` count only as unqualified identifiers, so a field such as `x.Port` is no match.
- GOARCH-31 and GOARCH-40: retired.
- GOARCH-41 (new, depguard): the imports of the domain and the logic.
- GOARCH-42 (new, depguard): `internal/platform/` never imports `internal/app/`.
- GOARCH-43 (new, depguard): `internal/platform/testenv` is imported only by `_test.go` files.
- The scopes on `internal/**` of the rules of domain and use cases become `internal/app/**`, the scopes on `pkg/**`
  become `internal/platform/**`, and those on `*_in.go`, `*_logic.go` and `*_out.go` become `in.go`, `logic.go`
  and `out.go` under `internal/app/*/*/`.

The fixture tree `tests/architecture/testdata/bad` is rebuilt in the new layout with one violation per AST rule, and
the real repository stays the good case: the subtest `module` of every `TestGOARCH<N>` expects no violation on it.

### Errors

No error changes: every port method, shell and entry keeps its errors, wrapping and exit codes.
The new domain function of Task 1 returns the error of the JSON decoder wrapped as today, and `DiagnoseSetup`
turns it into the same failing finding.
The tests that run depguard fail, never skip, when golangci-lint is missing or exits with an error other than its
findings, with the output of golangci-lint in the message.

### Discarded alternatives

- Full names inside each package (`check.CheckRuleCommand`) with the stutter check of revive disabled: discarded by
  the user in favour of fixed names and the alias.
- File names `command.go`, `logic.go`, `adapter.go` from the review of 2026-11-05: they rename the vocabulary of
  every rule and document for no gain over `in`, `logic`, `out`.
- Keeping `pkg/`: it is public API of the module, and polix is a binary.
- Only `internal/platform/` with the domains left at `internal/<noun>/`: it makes `platform` a reserved name and
  the layout asymmetric, and the depth would not tell the kind of a folder.
- Helpers under Go's `internal/` (`internal/app/rule/check/internal/<helper>/`): the compiler would enforce their
  visibility, but at one more level, against the layout the user asked for.
- depguard for the structural import rules: it matches packages only by prefix, so each use case and each helper
  would need its own entry, and a new one without it would go unchecked.
- The depguard rules as lint only, without a check: `polix rules check` would not decide them.
- Exporting the JSON types of the hooks of `setup`: GOARCH-16 forbids field tags on exported domain types.
- Copying the setup test helpers into each of the five packages: about 80 lines and a fake CLI built five times.
- Small ports and `Step` as a sealed interface in the same change: a separate change (see Out of scope).

## Acceptance criteria

- AC1: every one of the 13 use cases lives in `internal/app/<noun>/<verb>/` with package `<verb>`, its `in.go`,
  `logic.go`, `out.go`, `<verb>.md` and the tests it has today; no `<verb>_<noun>_*` file is left in `internal/`.
- AC2: inside each use case package the names are `Command`, `Request`, `Response`, `Port`, `Adapter`, `Decide`,
  `Output` and `run`, no top-level name starts with `<Verb><Noun>`, and `cmd/polix/main.go` imports each use case
  with the alias `<noun><verb>`.
- AC3: the behaviour is unchanged: `polix --help` and the help of every command and subcommand print the same text
  as before, and every test of `internal/`, `cmd/`, `plugins/`, `templates/` and `tests/install/` passes.
- AC4: no use case uses an unexported name of a domain package, and the setup use case tests use
  `internal/platform/testenv`.
- AC5: `tests/architecture` passes: every subtest `detects` finds its violations in its fixture, and every subtest
  `module` finds none in the repository.
- AC6: an import against the direction of the depth (a domain importing a use case or a helper, a use case importing
  another, a helper imported from outside its use case or by another helper, a use case imported outside `cmd/` and
  `tests/`, test files included) fails `TestGOARCH9`.
- AC7: GOARCH-31 and GOARCH-40 are in `ai/rules/retired.yaml` with a reason, their tests are gone, and
  `TestRulesChecks` passes.
- AC8: `golangci-lint run ./...` reports nothing, revive, `importas` and depguard included.
- AC9: `docs/architecture.md` opens with the principle of depth and width and describes the new layout, names and
  rules; `docs/polix.md`, `README.md`, `CONTRIBUTING.md` where it names paths, and the comments of `.golangci.yml`
  match it.
- AC10: `just check` passes, `polix rules check` among its steps.
- AC11: `pkg/` no longer exists, the twelve libraries and `testenv` live in `internal/platform/<lib>/`, the eight
  domains in `internal/app/<noun>/`, and no folder is deeper than `internal/app/<noun>/<verb>/<helper>/` or
  `internal/platform/<lib>/`, `testdata/` aside.
- AC12: a binary built as `scripts/dist.sh` builds it prints the version it was given in `polix version`.
- AC13: the domain or the logic importing a library of `internal/platform/` outside the allowlist, `internal/platform/`
  importing `internal/app/`, and a non-test file importing `internal/platform/testenv` are each reported by depguard
  with the name of its rule, and fail `TestGOARCH41`, `TestGOARCH42` and `TestGOARCH43`.
- AC14: no file of `internal/app/`, tests included, imports a use case package other than its own helpers; the
  end-to-end tests of setup live in `tests/setup/` and pass.

## Out of scope

- Lighten the domain of `workflow` (typed conditions instead of `Cond` as a map, a domain `event` for the names of
  the events, the runners of the rules moved to `rule` and the weights of risk to `diff`): its own plan, which
  depends on this one, since moving those types needs the use cases out of the domain packages.
- Break up `AnswerHook` (its logic split into domain types and helper packages, small ports, `Step` as a sealed
  interface): its own plan, which depends on this one and on the plan of `workflow`.
- Limits of size and complexity (gocyclo, gocognit, a limit of lines per file with a ratchet, and its rule in
  `ai/rules/go.yaml`): its own plan, independent of this one, the detector of growth that does not open siblings.
- `AGENTS.md`: a protected file; a person updates its Layout, which lists packages that no longer exist (`cli`,
  `engine`, `gitx`, `glob`, `rules`, `shell`, `update`) and lacks `tests/architecture/` and `docs/architecture.md`,
  to `internal/app/<noun>/<verb>/` and `internal/platform/<lib>/`, and its Testing section, which names
  `internal/cli/` for the tests of the install scripts that live in `tests/install/`.
- `templates/` and `plugins/` at the root: they embed the assets next to them.
- `ai/plans/` and `ai/reports/`: historical records that keep the old names.
- A public API of polix as a library: polix is a binary; such a facade would be a package outside `internal/`.

## Tasks

### Task 1: Export the domain names that the use cases borrow

- Covers: AC4.
- Depends on: none.
- Files: `internal/setup/setup_hooks.go` (`hookGroup`, `hookHandler`, `hooksFileJSON`, `claudeHookEvents`, modify),
  `internal/setup/setup_hooks_test.go` (modify), `internal/setup/setup_finding.go` (`found`, modify),
  `internal/setup/setup_finding_test.go` (modify), `internal/setup/diagnose_setup_logic.go`
  (`diagnoseSetupHooks`, modify), the files of `internal/setup/` that call `found`, tests included (modify),
  `internal/workflow/workflow_shortcuts.go` (`shortcutProtectedFiles`, `shortcutForbiddenCommands`, modify),
  `internal/workflow/graph_workflow_logic.go` and the files of `internal/workflow/` that use both constants (modify).
- Rules: GOARCH-1, GOARCH-2, GOARCH-4, GOARCH-9, GOARCH-13, GOARCH-16, GOARCH-18, GOARCH-23, GOARCH-25,
  GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-29, GOARCH-30, GOARCH-31, GOARCH-35, GOARCH-38, GOARCH-40, GO-2.
- Reuse: the unexported JSON types `hookGroup`, `hookHandler` and `hooksFileJSON` of `internal/setup/setup_hooks.go`
  as the decoder, and the existing tests of `diagnoseSetupHooks` in `internal/setup/diagnose_setup_logic_test.go`.
- Changes: in `setup_hooks.go`, add the exported tagless domain types `HookGroup` (matcher and handlers) and
  `HookHandler` (command and timeout, the fields `DiagnoseSetup` reads), each with its `Valid`.
  Add the exported domain function `ParseHooks`, which decodes the bytes of a settings file through the unexported
  JSON types and returns the groups of each event as `HookGroup` values, built as literals and validated.
  Export `claudeHookEvents` as `ClaudeHookEvents`, a table of the events with its fields exported.
  The unexported JSON types stay for the writing of the hooks by `hooks` and `mergeHooks`.
  `diagnoseSetupHooks` uses `ParseHooks`, `HookGroup` and `ClaudeHookEvents` instead of the unexported names, with
  the same findings and messages, and a decoding error turns into the same failing finding as today.
  Rename `found` to `Found` and update its callers, tests included.
  Export `shortcutProtectedFiles` and `shortcutForbiddenCommands` as `ShortcutProtectedFiles` and
  `ShortcutForbiddenCommands` and update their uses.
- Tests: in `internal/setup/setup_hooks_test.go`, `ParseHooks` on a settings file with hooks of several events,
  on one without hooks (no groups and no error) and on invalid JSON (an error); `Valid` of `HookGroup` and
  `HookHandler`.
  The tests of `diagnoseSetupHooks` keep passing unchanged, the ones on malformed settings included.
- Verification: `just test`, expected: every test passes, `tests/architecture` included.

### Task 2: Move the libraries of pkg/ to internal/platform/

- Covers: AC3, AC11, AC12.
- Depends on: Task 1.
- Files: `pkg/<lib>/*.go` for `build`, `cli`, `file`, `git`, `github`, `glob`, `jsonl`, `proc`, `semver`, `shell`,
  `text` and `yamlfmt` (delete), `internal/platform/<lib>/*.go` for the same twelve (create), every Go file of
  `internal/`, `cmd/` and `tests/install/` that imports them (modify), `scripts/dist.sh` (the flag `-X` of the
  version, modify), `tests/install/install_test.go` (its flag `-X`, modify), `.golangci.yml` (`importas`, modify),
  `polix.yaml` (`risk.weights`, modify), the use case documents `internal/*/*.md` and the comments that name
  `pkg/<lib>` (modify).
- Rules: GOARCH-7, GOARCH-13, GOARCH-19, GOARCH-35, GOARCH-38, GOARCH-39, XPLAT-1, GO-2, GO-3, DOC-1.
- Reuse: the code and tests of the libraries, moved unchanged.
- Changes: move each library to `internal/platform/<lib>/`, same package name and content.
  Update every import path of the module from `pkg/<lib>` to `internal/platform/<lib>`, imported without alias.
  `scripts/dist.sh` and `tests/install/install_test.go` set the version through
  `internal/platform/build.Version`; a wrong path builds a binary without version and no error, so AC12 checks it.
  In `.golangci.yml`, `importas` keeps one pattern, for the use cases only: `internal/app/<noun>/<verb>` imported as
  `<noun><verb>`; the old pattern would demand `platformgit` for the libraries; its comment says so.
  In `polix.yaml`, the weight of `pkg/git/**` becomes `internal/platform/git/**`.
  The documents and comments that name `pkg/<lib>` name `internal/platform/<lib>`.
- Tests: the moved tests keep every case; `tests/install` passes with the new flag `-X`.
- Verification: `go build ./... && go test ./internal/... ./cmd/... ./plugins/... ./templates/... ./tests/install/...`,
  expected: builds and passes; `golangci-lint run ./...`, expected: no finding;
  build with the flag of `scripts/dist.sh` and version `9.9.9` into the scratch folder and run `version`, expected:
  it prints 9.9.9.

### Task 3: Move the shared helpers of the setup tests to internal/platform/testenv

- Covers: AC4.
- Depends on: Task 2.
- Files: `internal/platform/testenv/testenv.go` (create), `internal/platform/testenv/testenv_test.go` (create),
  `internal/platform/testenv/testdata/fakecli/main.go` (create, moved from
  `internal/setup/testdata/fakecli/main.go`), `internal/setup/testdata/fakecli/main.go` (delete),
  `internal/setup/setup_test.go` (`TestMain`, `read`, `write`, `copyFile`, `machine`, `home`, `calls`, `gitRepo`,
  `exeSuffix`, `fakeCLI`, modify), the test files of `internal/setup/` that use them (modify).
- Rules: GOARCH-7, GOARCH-13, GOARCH-19, GOARCH-35, GOARCH-38, GOARCH-39, XPLAT-1, GO-2.
- Reuse: the bodies of the helpers of `internal/setup/setup_test.go` and its `TestMain`.
- Changes: create the library `testenv`, which imports nothing of `internal/app/`.
  It exports `Read`, `Write`, `Calls`, `GitRepo`, `Home` and `Machine` with the behaviour of the current helpers.
  `Machine` builds the fake CLI of the agents once per test binary, from `testdata/fakecli` next to the source of
  the library, and skips the test when git or the go tool is missing, as today.
  Remove the helpers and `TestMain` from `internal/setup/setup_test.go` and make every setup test call `testenv`.
- Tests: in `internal/platform/testenv/testenv_test.go`, `GitRepo` gives a repository with one commit on main,
  `Write` and `Read` round trip a file in a new folder, and `Machine` puts the fake `claude` on the PATH, whose call
  `Calls` returns.
  The setup tests keep passing on the three systems of CI, the fake CLI with its `.exe` on Windows.
- Verification: `go test ./internal/...`, expected: passes; `golangci-lint run ./internal/...`, expected: no finding.

### Task 4: Move the domain packages to internal/app/

- Covers: AC3, AC11.
- Depends on: Task 3.
- Files: every file of `internal/agent/`, `internal/diff/`, `internal/hook/`, `internal/rule/`, `internal/setup/`,
  `internal/state/`, `internal/version/` and `internal/workflow/`, use case files and `testdata/` included (delete),
  the same files under `internal/app/<noun>/` (create), every Go file of `internal/`, `cmd/`, `plugins/` and
  `tests/` that imports them (modify), `polix.yaml` (`risk.weights`, modify), `docs/polix.md` (the paths of the
  packages of the hook, modify).
- Rules: GOARCH-7, GOARCH-13, GOARCH-35, GOARCH-38, GOARCH-39, XPLAT-1, GO-2, DOC-1.
- Reuse: the code, tests and documents of the packages, moved unchanged.
- Changes: move each domain package, with the use case files it still holds, to `internal/app/<noun>/`, same package
  name and content, and update every import path, imported without alias.
  In `polix.yaml`, the weights of `internal/hook/**` and `internal/state/**` become `internal/app/hook/**` and
  `internal/app/state/**`.
  In `docs/polix.md`, the paths `internal/hook` and `internal/agent` become `internal/app/hook` and
  `internal/app/agent`.
- Tests: the moved tests keep every case.
- Verification: `go build ./... && go test ./internal/... ./cmd/... ./plugins/... ./templates/... ./tests/install/...`,
  expected: builds and passes; `golangci-lint run ./internal/... ./cmd/...`, expected: no finding.

### Task 5: Move CheckRule to internal/app/rule/check

- Covers: AC1, AC2, AC3.
- Depends on: Task 4.
- Files: `internal/app/rule/check_rule.md`, `check_rule_in.go`, `check_rule_logic.go`, `check_rule_out.go` and their
  tests in `internal/app/rule/` (delete); `internal/app/rule/check/check.md`, `in.go`, `logic.go`, `out.go`,
  `in_test.go`, `logic_test.go`, `out_test.go` (create); `internal/app/rule/rule_result.go` (comment, modify);
  `cmd/polix/main.go` (`main`, modify).
- Rules: GOARCH-4, GOARCH-6, GOARCH-7, GOARCH-8, GOARCH-13, GOARCH-14, GOARCH-16, GOARCH-18, GOARCH-20,
  GOARCH-21, GOARCH-22, GOARCH-23, GOARCH-25, GOARCH-28, GOARCH-29, GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35,
  GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, XPLAT-1, GO-2.
- Reuse: the code and tests of the current files, moved; `archRules` of `internal/app/rule/rule_run_test.go`, copied.
- Changes: move each file of `CheckRule` to its new path in package `check`, applying the mapping of names of the
  Approach (`CheckRuleCommand` to `Command`, `CheckRule` to `Decide`, `checkRule` to `run`, `CheckRuleFailed` to
  `Failed`, and the rest).
  Qualify the names of the domain of `rule` and import it; the code keeps its behaviour line for line.
  Copy `archRules` into `logic_test.go` and leave it in `rule_run_test.go`, which still uses it.
  `check.md` keeps its title `# CheckRule` and its sections, with the new names of the request, response and port.
  `cmd/polix/main.go` imports `internal/app/rule/check` as `rulecheck` and wires `rulecheck.Command()` under `rules`.
  Update the comment of `rule_result.go` that names the use case.
- Tests: the moved tests keep every case, renamed only; `go test ./internal/app/rule/...` runs the domain and the
  use case.
- Verification: `go build ./... && go test ./internal/... ./cmd/...`, expected: builds and passes;
  `golangci-lint run ./internal/app/rule/... ./cmd/...`, expected: no finding.

### Task 6: Move the five use cases of setup to their packages

- Covers: AC1, AC2, AC3.
- Depends on: Task 5.
- Files: in `internal/app/setup/`, the files `check_setup*`, `diagnose_setup*`, `install_setup*`, `refresh_setup*`
  and `uninstall_setup*` (`.md`, `_in.go`, `_logic.go`, `_out.go` and their tests, delete);
  `internal/app/setup/check/`, `diagnose/`, `install/`, `refresh/` and `uninstall/` with `<verb>.md`, `in.go`,
  `logic.go`, `out.go` and the tests each has today (create); the comments of `internal/app/setup/*.go` that name
  these use cases (modify); `cmd/polix/main.go` (`main`, modify).
- Rules: GOARCH-4, GOARCH-6, GOARCH-7, GOARCH-8, GOARCH-13, GOARCH-14, GOARCH-16, GOARCH-18, GOARCH-20,
  GOARCH-21, GOARCH-22, GOARCH-23, GOARCH-25, GOARCH-28, GOARCH-29, GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35,
  GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, XPLAT-1, GO-2.
- Reuse: the code and tests of the current files, moved; `internal/platform/testenv` for the tests;
  `setup.ParseHooks`, `setup.HookGroup`, `setup.ClaudeHookEvents` and `setup.Found` from Task 1.
- Changes: move each use case to its package (`check`, `diagnose`, `install`, `refresh`, `uninstall`), applying the
  mapping of names of the Approach to each one (`InstallSetupCommand` to `Command`, `diagnoseSetup` to `run`,
  `diagnoseSetupHooks` to `hooks`, and the rest).
  Qualify the names of the domains `setup`, `hook`, `rule`, `state`, `version` and `workflow` they use.
  Each document keeps its title (`# CheckSetup`, `# DiagnoseSetup`...) with the new names.
  `cmd/polix/main.go` imports them as `setupcheck`, `setupdiagnose`, `setupinstall`, `setuprefresh` and
  `setupuninstall`, and wires each `Command()` in the same group and order as today, the hidden `__refresh`
  included.
- Tests: the moved tests keep every case, renamed only, and use `testenv`; each package passes on its own.
- Verification: `go build ./... && go test ./internal/... ./cmd/...`, expected: builds and passes;
  `golangci-lint run ./internal/app/setup/... ./cmd/...`, expected: no finding.

### Task 7: Move the three use cases of workflow to their packages

- Covers: AC1, AC2, AC3.
- Depends on: Task 6.
- Files: in `internal/app/workflow/`, the files `format_workflow*`, `graph_workflow*` and `log_workflow*` (`.md`,
  `_in.go`, `_logic.go`, `_out.go` and their tests, delete); `internal/app/workflow/format/`, `graph/` and `log/`
  with `<verb>.md`, `in.go`, `logic.go`, `out.go` and their tests (create); the comments of
  `internal/app/workflow/*.go` that name these use cases (modify); `cmd/polix/main.go` (`main`, modify).
- Rules: GOARCH-4, GOARCH-6, GOARCH-7, GOARCH-8, GOARCH-13, GOARCH-14, GOARCH-16, GOARCH-18, GOARCH-20,
  GOARCH-21, GOARCH-22, GOARCH-23, GOARCH-25, GOARCH-28, GOARCH-29, GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35,
  GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, XPLAT-1, GO-2.
- Reuse: the code and tests of the current files, moved; `workflow.ShortcutProtectedFiles` and
  `workflow.ShortcutForbiddenCommands` from Task 1.
- Changes: move each use case to its package (`format`, `graph`, `log`), applying the mapping of names of the
  Approach to each one (`LogWorkflowEntry` to `Entry`, `GraphWorkflow` to `Decide`, and the rest).
  Qualify the names of the domains `workflow` and `state`; inside the package `log`, `Entry` is the row of the
  response and `workflow.Entry` stays qualified, so both coexist.
  The package `log` never imports the standard `log`, which GOARCH-4 already forbids to its logic.
  `cmd/polix/main.go` imports them as `workflowformat`, `workflowgraph` and `workflowlog`, and wires each
  `Command()` where it is today (`config fmt`, `graph`, `log`).
- Tests: the moved tests keep every case, renamed only; each package passes on its own.
- Verification: `go build ./... && go test ./internal/... ./cmd/...`, expected: builds and passes;
  `golangci-lint run ./internal/app/workflow/... ./cmd/...`, expected: no finding.

### Task 8: Move the two use cases of version to their packages

- Covers: AC1, AC2, AC3.
- Depends on: Task 7.
- Files: in `internal/app/version/`, the files `show_version*` and `update_version*` (`.md`, `_in.go`, `_logic.go`,
  `_out.go` and their tests, delete); `internal/app/version/show/` and `update/` with `<verb>.md`, `in.go`,
  `logic.go`, `out.go` and the tests each has today (create); `internal/app/version/version.go` (package comment,
  modify); `cmd/polix/main.go` (`main`, modify).
- Rules: GOARCH-4, GOARCH-6, GOARCH-7, GOARCH-8, GOARCH-13, GOARCH-14, GOARCH-16, GOARCH-18, GOARCH-20,
  GOARCH-21, GOARCH-22, GOARCH-23, GOARCH-25, GOARCH-28, GOARCH-29, GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35,
  GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, XPLAT-1, GO-2.
- Reuse: the code and tests of the current files, moved.
- Changes: move each use case to its package (`show`, `update`), applying the mapping of names of the Approach to
  each one (`UpdateVersionCommand` to `Command`, `updateVersion` to `run`, `updateVersionDecide` to a name of its
  role that does not clash with `Decide`, and the rest).
  Qualify the names of the domains `version` and `workflow`.
  `ShowVersion` has no adapter test today and keeps none.
  `cmd/polix/main.go` imports them as `versionshow` and `versionupdate` and wires each `Command()` as today.
  The package comment of `version.go` names the use cases by their new paths.
- Tests: the moved tests keep every case, renamed only; each package passes on its own.
- Verification: `go build ./... && go test ./internal/... ./cmd/...`, expected: builds and passes;
  `golangci-lint run ./internal/app/version/... ./cmd/...`, expected: no finding.

### Task 9: Move MeasureDiff to internal/app/diff/measure

- Covers: AC1, AC2, AC3.
- Depends on: Task 8.
- Files: in `internal/app/diff/`, the files `measure_diff*` (`.md`, `_in.go`, `_logic.go`, `_out.go` and their
  tests, delete); `internal/app/diff/measure/` with `measure.md`, `in.go`, `logic.go`, `out.go` and their tests
  (create); `cmd/polix/main.go` (`main`, modify).
- Rules: GOARCH-4, GOARCH-6, GOARCH-7, GOARCH-8, GOARCH-13, GOARCH-14, GOARCH-16, GOARCH-18, GOARCH-20,
  GOARCH-21, GOARCH-22, GOARCH-23, GOARCH-25, GOARCH-28, GOARCH-29, GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35,
  GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, XPLAT-1, GO-2.
- Reuse: the code and tests of the current files, moved.
- Changes: move the use case to package `measure`, applying the mapping of names of the Approach
  (`MeasureDiffRow` to `Row`, `MeasureDiff` to `Decide`, and the rest), and qualify the names of the domains `diff`
  and `workflow`.
  `cmd/polix/main.go` imports it as `diffmeasure` and wires `diffmeasure.Command()` in the group `repo` as today.
- Tests: the moved tests keep every case, renamed only.
- Verification: `go build ./... && go test ./internal/... ./cmd/...`, expected: builds and passes;
  `golangci-lint run ./internal/app/diff/... ./cmd/...`, expected: no finding.

### Task 10: Move AnswerHook to internal/app/hook/answer

- Covers: AC1, AC2, AC3.
- Depends on: Task 9.
- Files: in `internal/app/hook/`, the files `answer_hook*` (`.md`, `_in.go`, `_logic.go`, `_out.go` and their
  tests, delete); `internal/app/hook/answer/` with `answer.md`, `in.go`, `logic.go`, `out.go` and their tests
  (create); the comments of `internal/app/hook/*.go` that name the use case (modify); `cmd/polix/main.go`
  (`main`, modify); `docs/polix.md` (the line on the use case `AnswerHook`, modify).
- Rules: GOARCH-4, GOARCH-6, GOARCH-7, GOARCH-8, GOARCH-13, GOARCH-14, GOARCH-16, GOARCH-18, GOARCH-20,
  GOARCH-21, GOARCH-22, GOARCH-23, GOARCH-25, GOARCH-28, GOARCH-29, GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35,
  GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, XPLAT-1, GO-2, DOC-1.
- Reuse: the code and tests of the current files, moved.
- Changes: move the use case to package `answer`, applying the mapping of names of the Approach (`AnswerHook` to
  `Decide`, `answerHook` to `run`, `AnswerHookOutput` to `Output`, `answerHookRun` to `pass`,
  `answerHookInjected` to `passInject`, `answerHookOutputLines` to `outputLines`, and the rest).
  Qualify the names of the domain `hook` (`hook.Evaluation`, `hook.Step`, `hook.Facts`, `hook.Hook`) and of
  `agent`, `diff`, `rule`, `state` and `workflow`; the step shape and the port of 16 methods stay as they are.
  `cmd/polix/main.go` imports it as `hookanswer` and wires `hookanswer.Command()` in the group `agent` as today.
  In `docs/polix.md`, the use case of `polix hook` is `internal/app/hook/answer`.
- Tests: the moved tests keep every case, renamed only, the tests of the shell with the fake of the port, the
  walk of the logic and the adapter against a temporary repository among them.
- Verification: `go build ./... && go test ./internal/... ./cmd/...`, expected: builds and passes;
  `golangci-lint run ./...`, expected: no finding outside `tests/architecture`.

### Task 11: Move the end-to-end tests of setup to tests/setup

- Covers: AC14.
- Depends on: Task 10.
- Files: `internal/app/setup/install/out_test.go`, `internal/app/setup/uninstall/out_test.go`,
  `internal/app/setup/refresh/out_test.go`, `internal/app/setup/diagnose/out_test.go` (the tests and helpers that
  import another use case of setup, modify), `tests/setup/setup_test.go` (create).
- Rules: GOARCH-35, GOARCH-38, GOARCH-39, XPLAT-1, GO-2.
- Reuse: `runSetup` and the end-to-end tests of the four `out_test.go`, `internal/platform/testenv`.
- Changes: the tests that compose several setup commands (`TestCommand` of install, diagnose and refresh,
  `TestCommandsTheUpdateRuns`, and any other test that imports a sibling use case) move to `tests/setup/`, package
  `setup`, which builds the commands as `cmd/polix` does, imports the use cases with their aliases and uses
  `testenv`; `runSetup` lives there once.
  `uninstall/out_test.go` prepares the state its adapter removes by writing it with `testenv`, without the adapter of
  install.
  Afterwards no file of `internal/app/` imports a use case package other than its own helpers.
- Tests: every case of the moved tests is kept in `tests/setup/`; the tests left in each use case pass on their own.
- Verification: `rg -n 'internal/app/setup/(check|diagnose|install|refresh|uninstall)"' internal`, expected: no match;
  `go test ./internal/... ./tests/setup/...`, expected: passes; `golangci-lint run ./...`, expected: no finding.

### Task 12: Rewrite the AST architecture rules, their tests and the fixture tree for the layout by depth

- Covers: AC5, AC6, AC7, AC11.
- Depends on: Task 11.
- Files: `ai/rules/architecture.yaml` (header and GOARCH-1 to GOARCH-40 as the Approach lists, modify),
  `ai/rules/retired.yaml` (create), `tests/architecture/module_test.go` (`load`, `classify`, `useCases`, `useCase`,
  `goFile`, `packages`, `useCaseSuffixes`, `lowerCamel`, modify), the tests `tests/architecture/goarchNN_test.go` of
  the rules that change (modify), `tests/architecture/goarch10_test.go`, `goarch31_test.go` and `goarch40_test.go`
  (delete), `tests/architecture/testdata/bad/**` (modify, create and delete as described).
- Rules: GOARCH-13, GOARCH-35, GOARCH-38, GOARCH-39, GO-2, GO-3.
- Reuse: the helpers of `module_test.go` that already work on nested folders (`typed`, `sourceImporter`,
  `moduleDir`, `internalDir`, `packageRefs`, `cycles`), `camel` for titles and entities, and `checkRule` with its
  subtests `module` and `detects`.
- Changes: `load` reads `internal/` and `cmd/`; `classify` gives the kind of each folder and file by depth, as the
  section Layout by depth says, with the noun, the verb and the helper of the path.
  Rewrite each rule as the section Architecture rules of the Approach says, keeping the message style of the
  checks, and reword the descriptions and scopes in `architecture.yaml`, the header included.
  GOARCH-1 states the principle: the depth of a folder gives the kind of its code, and growth is new siblings.
  GOARCH-9 checks the whole direction of the imports of the section Direction of the imports, except the part of
  `internal/platform/`, which is GOARCH-42; its test reads the import graph, with the helpers of a use case.
  GOARCH-10 loses its check and becomes non-deterministic.
  GOARCH-4 reads the list `GOARCH-41-logic` of `.golangci.yml` to check the helpers the logic imports; until Task 13
  adds it, that part of the check is written against the list as the Approach states it.
  Move GOARCH-31 and GOARCH-40 to `ai/rules/retired.yaml` with their id, description and a reason (the package
  namespace of each use case, and Go declaring methods only in the package of their type).
  Rebuild `testdata/bad` in the new layout: `internal/<noun>/` becomes `internal/app/<noun>/`, its `pkg/` becomes
  `internal/platform/`, the use cases `place_order`, `cancel_order`, `count_stock` and `drop_stock` become
  `internal/app/order/place/`, `internal/app/order/cancel/`, `internal/app/stock/count/` and
  `internal/app/stock/drop/` with their faults kept, and `ship_parcel` gives way to new faults of GOARCH-1 and
  GOARCH-11: a package clause unlike its folder, a stray file in a use case folder, a folder below a helper, a folder
  below a library, a use case without `Decide` or `Adapter`.
  Add a helper `internal/app/order/place/pricing/` imported by another use case and by another helper, and a domain
  that imports a use case and a helper, so GOARCH-9 detects each direction it forbids; the method of GOARCH-40 is
  removed.
  Append to the files whose line numbers the `want` strings pin, update the `want` strings to the new names, and
  update `testdata/bad/README.md`.
- Tests: each `TestGOARCH<N>` keeps its two subtests: `detects` finds every violation of the rebuilt tree, and
  `module` finds none in the repository; `TestRulesChecks` passes with the retired rules out of `architecture.yaml`.
  `TestGOARCH9` detects a domain importing a use case, a domain importing a helper, a use case importing another,
  a helper imported by another use case, a helper imported by another helper, and a use case imported outside `cmd/`
  and `tests/`, from a test file too.
  `TestGOARCH1` detects a folder below a helper and a folder below a library, and accepts `testdata/` at any level.
- Verification: `go test ./tests/architecture`, expected: passes; `golangci-lint run ./...`, expected: no finding.

### Task 13: Move the fixed import lists to depguard, with tests that run it

- Covers: AC8, AC10, AC13.
- Depends on: Task 12.
- Files: `.golangci.yml` (`linters.enable`, `linters.settings.depguard`, modify), `ai/rules/architecture.yaml`
  (GOARCH-4 without its import lists, GOARCH-7, GOARCH-41, GOARCH-42, GOARCH-43, modify),
  `tests/architecture/depguard_test.go` (create), `tests/architecture/goarch41_test.go`, `goarch42_test.go`,
  `goarch43_test.go` (create), `tests/architecture/goarch04_test.go` (`checkPure`, modify),
  `tests/architecture/testdata/depguard/**` (create), `tests/architecture/testdata/bad/**` (the import faults of
  GOARCH-4 that move to GOARCH-41, modify).
- Rules: GOARCH-13, GOARCH-35, GOARCH-38, GOARCH-39, XPLAT-1, GO-2, GO-3.
- Reuse: `checkRule` of `tests/architecture/module_test.go`, and the cache of parsed roots of that file as the model
  for running golangci-lint once per root.
- Changes: enable depguard in `.golangci.yml` with the lists `GOARCH-41-domain` (domain files, tests excluded),
  `GOARCH-41-logic` (the `logic.go` of every use case), `GOARCH-42` (every file of `internal/platform/`) and
  `GOARCH-43` (every non-test file), each `desc` naming its rule and what to do instead, as the section Imports of
  the domain and the logic says; deny the forbidden standard packages and allow the rest, since deny wins.
  Add GOARCH-41, GOARCH-42 and GOARCH-43 to `architecture.yaml`, deterministic, each with its test as `check`, and
  take the import lists out of GOARCH-4, which keeps its other checks.
  `depguard_test.go` runs golangci-lint with only depguard on a root, once per root and test binary, parses its
  findings by list, and fails with the output when golangci-lint is missing or fails for another reason.
  `goarch04_test.go` reads the list `GOARCH-41-logic` from `.golangci.yml` instead of its own table.
  Create `testdata/depguard`, a module that compiles with the module path of polix and its own `go.mod`, holding a
  domain that imports `os` and `internal/platform/git`, a logic that imports `context`, a library that imports
  `internal/app/`, and a non-test file that imports `internal/platform/testenv`.
- Tests: `TestGOARCH41`, `TestGOARCH42` and `TestGOARCH43`: `detects` finds each violation of `testdata/depguard`
  named by its list, `module` finds none in the repository; a test of `depguard_test.go` checks that a missing
  golangci-lint gives a failure, not a skip.
- Verification: `go test ./tests/architecture`, expected: passes; `golangci-lint run ./...`, expected: no finding;
  `just check`, expected: passes, `polix rules check` included.

### Task 14: Update the docs to the new layout

- Covers: AC9.
- Depends on: Task 13.
- Files: `docs/architecture.md` (opening, layout, the example of `CheckRule`, flow of a use case, conventions,
  resolved decisions, modify), `README.md` (the paragraph on where the code lives, modify), `CONTRIBUTING.md` (the
  paths and rules it names, modify), `.golangci.yml` (the comments of `importas` and depguard, modify).
- Rules: GO-2, DOC-1.
- Reuse: the sections Layout by depth, Direction of the imports and Mapping of names of this plan.
- Changes: `docs/architecture.md` opens with the principle (the depth of a folder gives the kind of its code, its
  width the growth of the system), shows the layout tree of `internal/app/` and `internal/platform/` with the
  helpers, lists the direction of the imports, names the fixed names in the example of `internal/app/rule/check/`
  and in the flow, drops the prefix rule, says which rules depguard decides and how their tests run it, and replaces
  every `pkg/<lib>` by `internal/platform/<lib>`.
  `README.md` says the code lives in `cmd/polix`, `internal/app/` and `internal/platform/`.
  `CONTRIBUTING.md` follows the new paths and rule numbers where it names them.
- Tests: none, documentation only.
- Verification: `rg -n 'pkg/|CheckRuleCommand|AnswerHookCommand|_in\.go|_logic\.go|_out\.go' docs README.md
  CONTRIBUTING.md`, expected: no match outside the examples of other repositories in `docs/use-cases.md`.

## Risks

- The gate `run_before_done` of `polix.yaml` runs `go test ./...` and golangci-lint at every stop, and
  `tests/architecture` fails from Task 2 until Task 12: run the tasks in order in one go on a branch, and expect that
  gate to fail until Task 12 is done.
- The version of the release lost by a wrong flag `-X`, with no error: AC12 builds with the flag of
  `scripts/dist.sh` and checks the output of `polix version`.
- The tests of depguard need golangci-lint on the PATH of the job `test` of CI on the three systems: check that the
  step `Lint` (`golangci/golangci-lint-action`) leaves it there, or install it in that job at the pinned version.
- golangci-lint on the fixture `testdata/depguard` reads the configuration of the repository: run it with that
  configuration explicitly, so a change of `.golangci.yml` is tested by the fixture.
- polix checks only the files created through the edit tool of the agent, so the moves are written as new files
  and the old ones deleted: git still detects most of them as renames, but a file whose names change much may show
  as a deletion and a creation; `git log --follow` finds it.
- A rename that misses a caller, a fake or a comment: the compiler catches the code, and the `rg` of Task 14 and
  GOARCH-36 catch the documents.
- A package named `log` shadows the standard library inside its own files: `importas` gives it the alias
  `workflowlog` everywhere else, and its logic never imports the standard `log`.
- A setup test that depended on the state shared by `TestMain` (the fake CLI built once): `Machine` keeps building it
  once per test binary.
- A different help or output after the moves: AC3 compares the help of every command before and after.

## Verification

- Before Task 2, save the output of `polix --help` and of `--help` for every command and subcommand (`config check`,
  `config fmt`, `install`, `uninstall`, `doctor`, `graph`, `log`, `risk`, `update`, `version`, `hook`,
  `rules check`) into the scratch folder; after Task 10, run them again with the installed binary (`just install`)
  and compare: identical.
- `just test`: every test passes, `tests/architecture` and `tests/install` included.
- `golangci-lint run ./...`: no finding.
- `just check`: the config, the format of polix.yaml, `polix rules check` and `polix doctor --offline` pass.
- `just dist 9.9.9`, then the binary of this system in `dist/` prints 9.9.9 in `polix version`.
- `claude plugin validate plugins/polix`: passes, unchanged.
- `fd -t d . internal` lists no folder deeper than `internal/app/<noun>/<verb>/<helper>/` or
  `internal/platform/<lib>/`, `testdata/` aside, and `pkg/` does not exist.
- In a scratch repository with `polix install`, run a short Claude Code session that edits a protected file and
  stops with a failing check: polix blocks and answers as before.
