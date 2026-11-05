# Rebuild polix on the target architecture of examples/architecture, with its rules and tests in the root

## Overview

polix is rewritten, command by command, on the architecture drafted in the example folder of the repository.
Every command becomes a use case with the same shape: a thin entry that reads the flags, a pure decision that is
easy to test, and an adapter that talks to the disk, git, the network and other programs.
The forty architecture rules agreed in the example move to the rules of the repository, and their tests move to the
root, where they check the real code instead of the draft.

- During the change the old code is parked in a folder the architecture tests ignore, and each command leaves it
  as it is rebuilt; at the end the folder is deleted and nothing of the old layout remains.
- Every command keeps what it does today, proven by tests that cover the cases of the old ones.
  The only intended changes are those of the command that checks the rules, agreed in the example, plus the new way
  to run checks.
- A rule may now be checked by any file, a Go test among them, not only by a script: the workflow file says which
  command runs each kind of file.
  The architecture rules use it, so the command that checks the rules and CI run their Go tests one rule at a time.
- The hook keeps evaluating the entries of an event one after the other, each one seeing what the previous ones
  left, through a decision that is asked again after every step.
- The work ends with the whole test suite, the linter, the architecture tests and the checks of the rules passing
  on Linux, macOS and Windows, and with the docs matching the new code.

## Context

The target layout of `examples/architecture/` (domain packages, use cases as in, logic and out files, pure core and
imperative shell) was drafted on one use case, `CheckRule`, with 40 rules in `examples/architecture/rules.yaml` and
AST tests in `examples/architecture/tests/` that already detect every violation of `testdata/bad`.
Run against the root today they report 343 violations: the real code under `internal/` follows none of it.
The outcome is the whole binary rebuilt on that layout, governed by those rules from `ai/rules/`, with the tests
deciding the deterministic ones through `polix rules check`.

## Decisions

- One plan for the whole migration, as the user asked; the tasks are ordered and state what each one waits for,
  so independent ones can run at the same time.
- The current code moves unchanged to `legacy/` (outside `internal/`, `pkg/` and `cmd/`, which the architecture
  tests do not read) in the first task; each later task takes its commands out of it, and the last one deletes it.
- The open decisions of `examples/architecture/README.md` are accepted as drafted: `pkg/cli`, `pkg/git`, `pkg/glob`,
  `pkg/text`, the process part in `pkg/proc`, `workflow.Bases`, `workflow.Parse` and `BaseBranches` as domain of
  workflow, the command `rules` built in `cmd/polix/main.go`, and the changes of behaviour of `polix rules check`
  (a rule that is not `Valid` stops the command, `--base` without `--changed` is an error, the hint to set `base`
  only comes when no base exists).
- Consequence of the runners on decision 8 of the README: `polix rules check` always reads `polix.yaml` when it
  exists, to know the runners; `--base` only replaces its base branches.
  A `polix.yaml` that cannot be read or parsed stops the command; without it there are no runners and the default
  bases apply.
- The runner of ai/tasks/rule-tests-runner.md is part of this plan, in the form the user chose: a rule keeps
  `deterministic` and `check`, and `check` names any file of the repository (a script or a test).
  `polix.yaml` gains the top level key `rules` with `run`, a map from a glob of the file name to an argv.
  The argv takes the placeholders `{file}` (the check, relative to the root, with slashes), `{dir}` (its folder as
  `./<folder>`), `{id}` (`GOARCH-4`) and `{name}` (the id without separators, `GOARCH4`).
  A check that no glob matches runs as a program, as today, through the interpreter of Windows by extension.
  There are no default runners, one run per rule and no batching.
- Helper files of a test folder (`module_test.go`, `testdata/`) need no convention: only the files that a rule
  names in `check` run.
- The architecture tests live in `tests/architecture/`, package `architecture`; each test function is renamed to
  `TestGOARCH<N>` (the `{name}` of its rule with the prefix `Test`), the files keep their names.
- `polix hook` takes the step shape that decision 9 of the README foresaw, because a later entry of an event sees
  what an earlier one did (a `record`, or a `run` that rewrites files and changes the hash `done` is pinned to),
  and `allow` and `deny` stop the evaluation.
  The logic is asked for the next step, the shell carries it out and feeds the result back, from one call site in
  a loop, until the logic answers.
  GOARCH-20 and `docs/architecture.md` describe this second shape, only for use cases whose plan depends on the
  results of earlier steps.
- Names of packages and use cases, as proposed and accepted: `rule` (`CheckRule`, `polix rules check`); `workflow`
  (`GraphWorkflow` for `polix graph`, `LogWorkflow` for `polix log`); `hook`
  (`AnswerHook`, `polix hook`); `setup` (`CheckSetup` for `polix check`, `InstallSetup`, `UninstallSetup`,
  `DiagnoseSetup` for `polix doctor`, `RefreshSetup` for `polix __refresh`); `version` (`UpdateVersion` for
  `polix update`, `ShowVersion` for `polix version`).
  Domain only packages: `agent` (the formats of Claude Code and Codex, the neutral event and the answer) and
  `state` (the log of `.git/polix/log.jsonl`).
  Libraries without domain in `pkg/`: `cli`, `git`, `glob`, `text`, `shell`, `proc`, `jsonl`, `github`, `semver`,
  `file` and `build`.
- Amendment during the implementation: `polix check` cannot live in `workflow`, because `rule` imports `workflow`
  (bases and runners) and checking the rules from `workflow` would close an import cycle (GOARCH-7).
  It becomes `CheckSetup` in `setup`, which already sits above `workflow` and `rule`, and shares with `DiagnoseSetup`
  the domain that turns the workflow and the rules into problems; it moves from Task 5 to Task 8.
- Amendment during the implementation: the comparison of versions with the semantics of `legacy/version` (missing
  numbers padded) is a library without domain, `pkg/semver`, used by `workflow` (the check of `requires`) and by
  `version`, since `version` imports `workflow` and the reverse would close a cycle.
  The name of the asset and the check of `checksums.txt` stay in `pkg/github`, so `version` has no
  `version_asset.go` nor `version_checksum.go`.
- Amendment during the implementation: the file handling shared by the adapters of `setup` and `version` (atomic
  write that follows symlinks and keeps the mode, read of an optional file, sync of a folder, the cache file) is a
  library without domain, `pkg/file` (GOARCH-19), and the embedded plugin is read through `plugins.Files`.
- The version of the binary set by the linker moves to `pkg/build.Version`; the entries pass the current version
  to their use cases in the request, so the domain never reads it.
- The Go tests of `install.sh` and `install.ps1` leave the package of the commands for `tests/install/`, outside
  the folders the architecture tests read.
- polix is in development: no version is bumped and no upgrade notes are written; the runner is documented in
  `docs/polix.md` and `examples/polix.yaml`.

## Approach

The final layout of the module:

- `cmd/polix/main.go` declares only `main`: it builds the root command with `pkg/cli`, adds the entries of every
  use case (and the parent command `rules`) and exits with the code of `pkg/cli.Run`.
- `internal/<noun>/` is a domain package: `<noun>.go` declares the entity `<Noun>`, `<noun>_*.go` the rest of the
  domain, and each use case `<Verb><Noun>` has `<verb>_<noun>.md`, `_in.go`, `_logic.go`, `_out.go` and the tests
  `_logic_test.go`, `_in_test.go` and, when the adapter does more than call a library, `_out_test.go`.
- `pkg/<library>/` holds technology shared by several adapters, never importing `internal/`.
- `plugins/` (the embedded plugin) stays as it is; `tests/architecture/` and `tests/install/` hold the tests that
  are not of one package.

Every use case follows `docs/architecture.md` (the conventions of the README of the example): the entry turns flags
or stdin into `<Verb><Noun>Request` and validates them, injects `<Verb><Noun>Adapter` as `<Verb><Noun>Port`, runs the
shell `<verb><Noun>` and serializes `<Verb><Noun>Response` to the console with the exit code.
The shell reads through the port, calls the logic, carries out its output step by step and asks the domain what each
result means.
Domain values are exported structs with exported fields, built with a literal followed by `Valid() error`, and
changed only through value receiver methods named after an intent.

Contracts that change:

- `polix.yaml` gains `rules.run` (glob of the check file to argv with placeholders), parsed into
  `workflow.Runner` and validated by `polix check`: an empty argv, an invalid glob or an unknown placeholder is a
  problem.
- `polix rules check` changes as listed in `Decisions`, and runs each check through its runner.
  `run: { rules: changed }` of the hooks runs the checks through the same runners.
- `ai/rules/architecture.yaml` is a new rules file; the format of ai/rules/ keeps its fields, and `check` may name
  any file.
- The linker flag of the version becomes `-X github.com/mlophez/polix-ai/pkg/build.Version=<version>`.
- Every other command keeps its flags, output, messages, exit codes and the answers of the hook.

Error handling: each port method returns an error wrapped with what failed (the file, the command, the branch); the
shell returns it unchanged; the entry prints it through the ui of `pkg/cli` and exits with 1, except where a command
already has its own policy, which is kept: the hook fails open outside a repository and fails closed on an invalid
workflow only for edits and commands; `update --hook` never fails; the log fails open on stop.
A refusal decided by the logic is data of its output, never an error.

### Discarded alternatives

- Several plans, one per function: the user asked for one plan.
- A list of excluded folders in the architecture tests during the migration: a transitional mechanism inside the
  tests, where `legacy/` keeps them unchanged.
- Rules found by the name of their file in a test folder, without `check`: it breaks the format of the rules and
  needs a naming convention to tell helpers from rule files.
- A precomputed plan for the hook, logic called once: an entry would no longer see what the previous one did in the
  same event.
- One wrapper script per GOARCH rule calling `go test -run`: about thirty scripts the runner makes unnecessary.

## Acceptance criteria

- AC1: `ai/rules/architecture.yaml` holds GOARCH-1 to GOARCH-40 as agreed in the example, GOARCH-20 extended with the
  step shape, and `docs/architecture.md` holds the conventions; `examples/architecture/` no longer exists.
- AC2: `go test ./tests/architecture` passes: every `TestGOARCH<N>` finds no violation in the root module and detects
  every violation it expects in `tests/architecture/testdata/bad`.
- AC3: every GOARCH rule with `deterministic: true` has `check: tests/architecture/goarchNN_test.go`, `polix.yaml`
  has a runner for `*_test.go`, and `polix rules check` runs each of them as
  `go test ./tests/architecture -run ^TestGOARCH<N>$`, reporting all of them passed.
- AC4: a check that matches a runner of `rules.run` runs with the placeholders expanded, one that matches none runs
  as a program as before, and `polix check` reports a runner with an empty argv, an invalid glob or an unknown
  placeholder; `run: { rules: changed }` in a hook uses the same runners.
- AC5: `internal/` holds only the packages `agent`, `hook`, `rule`, `setup`, `state`, `version` and `workflow`,
  `pkg/` only `build`, `cli`, `git`, `github`, `glob`, `file`, `jsonl`, `proc`, `semver`, `shell` and `text`,
  `cmd/polix/main.go` declares only `main`, and `legacy/` does not exist.
- AC6: every command other than `polix rules check` behaves as before (flags, output, exit codes, answers of the
  hook), shown by tests that cover the cases of the deleted legacy tests.
- AC7: `polix rules check` follows the documented changes: a rule that is not `Valid` stops it, `--base` without
  `--changed` is an error, the hint to set `base` only comes when no base exists.
- AC8: in a hook, an entry sees what the earlier entries of the same event left: after a `run` that rewrites files,
  a later `unless: { done: ... }` is decided on the new content; a `record` makes a later `done` hold; `allow` and a
  `deny` that blocks stop the evaluation; the loop guard counts as before.
- AC9: `just test`, `golangci-lint run ./...`, `just check` and `claude plugin validate plugins/polix` pass, and the
  CI workflow passes on Linux, macOS and Windows.
- AC10: `docs/polix.md`, `examples/polix.yaml`, `AGENTS.md`, `scripts/dist.sh` and the plugin (`add-rule`,
  `doctor`, `rules-author`) match the new code, and `ai/tasks/rule-tests-runner.md` is removed.

## Out of scope

- New behaviour of any command beyond the runner and the agreed changes of `polix rules check`.
- Batching several rules in one run and default runners: left out by decision.
- The plugin `polix` itself (skills, agents) beyond the texts that describe the format of the rules and the runner.

## Tasks

### Task 1: Move the rules, tests and conventions of the example to the root, and park the current code in legacy/

- Covers: AC1, AC2.
- Files: `internal/**` (move to `legacy/**`), `cmd/polix/main.go` (`main`, modify), `scripts/dist.sh` (modify),
  `ai/rules/architecture.yaml` (create), `tests/architecture/**` (create, moved from
  `examples/architecture/tests/**`), `docs/architecture.md` (create), `examples/architecture/**` (delete).
- Rules: GO-2, GO-3, XPLAT-1, GOARCH-6, GOARCH-13, GOARCH-39.
- Reuse: `examples/architecture/rules.yaml`, `examples/architecture/README.md` and `examples/architecture/tests/`.
- Changes: every package of `internal/` moves as it is to `legacy/<package>`, with its tests, and the import paths of
  the module change to `github.com/mlophez/polix-ai/legacy/...`; nothing else of its code changes.
  `cmd/polix/main.go` calls `legacy/cli.Run`, and the linker flag of `scripts/dist.sh` and of the tests that build
  the binary point to `legacy/cli.Version`.
  `ai/rules/architecture.yaml` takes the 40 rules of the example with their descriptions; its header drops the
  note on candidates and says the tests are in `tests/architecture/`; no rule has `check` yet (Task 4 adds them).
  GOARCH-20 adds the step shape: when what the logic decides depends on the result of an earlier step, the logic
  returns the next step, the shell carries it out and calls the logic again with the result, from one call site.
  The tests move to `tests/architecture/` with package `architecture`; `-root` defaults to `../..`; `badRoot`
  stays `testdata/bad`; each test function is renamed `TestGOARCH<N>` without zero padding and without suffix; the
  comments that cite `rules.yaml` cite `ai/rules/architecture.yaml`.
  `docs/architecture.md` takes the layout, flow and conventions of the README, the open decisions as resolved in
  `Decisions` of this plan, and the step shape; `examples/architecture/` is deleted.
- Tests: the moved tests must pass on the root, where `internal/`, `pkg/` are empty and `cmd/polix/main.go` declares
  only `main`, and on `testdata/bad`; the legacy tests pass unchanged after the move of paths.
- Verification: `just test`, expected: every package passes, `tests/architecture` included; `golangci-lint run ./...`,
  expected: no findings, the moved tests included.

### Task 2: Build the shared libraries of pkg/

- Covers: AC5.
- Files: `pkg/build/build.go`, `pkg/cli/cli.go`, `pkg/git/git.go`, `pkg/github/github.go`, `pkg/glob/glob.go`,
  `pkg/jsonl/jsonl.go`, `pkg/proc/proc.go`, `pkg/shell/shell.go`, `pkg/text/text.go` (create, each with its
  `_test.go`).
- Rules: GO-2, GO-3, XPLAT-1, GOARCH-7, GOARCH-13, GOARCH-19, GOARCH-35, GOARCH-38, GOARCH-39.
- Reuse: `legacy/gitx` (`Repo`, `Open`, `Branch`, `Changed`, `ResolveBase`, `ContentHash`, `Files`, `Ignored`, `Rel`,
  `RealPath`), `legacy/glob` (`Path`, `AnyPath`, `Command`), `legacy/shell` (`Split`, `Segment`), `legacy/state`
  (`Cut`, `readLines`), `legacy/engine` (`lastLines`), `legacy/rules` (`Interpreter` and its helpers on Windows
  paths, the process part of `RunChecks`), `legacy/update` (`Client`, `Latest`, `Download`, `AssetName`,
  `Executable`, `Writable`, `Replace`, `Cleanup`), `legacy/cli` (`ui`, `exitCode`, `exit`, `Run`, `printError`,
  `dirFlag`, `Version` and the `init` that reads the build info).
- Changes: starts after Task 1.
  Each library is technology without domain and imports nothing of `internal/` nor `legacy/`.
  `pkg/build`: `Version`, set by the linker, and `Current`, which falls back to the module version of a tagged build.
  `pkg/cli`: the ui (OK, fail, warn lines), the exit error with its code, the flag `--dir`, the root command with the
  styles of fang, and `Run`, which executes a root command over args, stdin, stdout and stderr and returns the code.
  `pkg/git`: the repository with its root and git dir, every method that runs git taking `context.Context` first.
  `pkg/github`: the client of the releases of a repository (latest tag, download of an asset and of
  `checksums.txt`), with its timeouts given by the caller.
  `pkg/glob` and `pkg/shell`: the same functions as today.
  `pkg/jsonl`: read the lines of a file under a size limit, skipping over-long lines and counting them, and append
  one line.
  `pkg/proc`: run an argv from a folder with a deadline, a wait delay and the combined output; resolve how a file
  runs as a program (directly, or on Windows through bash of Git for Windows for `.sh` and the py launcher or
  python for `.py`, skipping the Windows folder and the Store aliases), with the lookup of the PATH and the test of
  existence injectable.
  `pkg/text`: `Cut` (a string cut to a byte limit with a marker) and the last non-empty lines of a text.
  The legacy packages keep their own copies until Task 9.
- Tests: the cases of the legacy tests of each function move to the test of its library; `pkg/proc` adds a script
  that exits with an error, one that runs out of time, one that cannot start and, with fakes, every branch of the
  interpreter of Windows; `pkg/jsonl` adds an over-long line that is skipped and counted.
- Verification: `just test`, expected: every package passes, `pkg/` and `tests/architecture` included.

### Task 3: Build the domain of workflow and state, with the runners of rules.run

- Covers: AC4, AC5.
- Files: `internal/workflow/workflow.go`, `internal/workflow/workflow_parse.go`, `internal/workflow/workflow_entry.go`,
  `internal/workflow/workflow_runner.go`, `internal/workflow/workflow_base.go` (create, each with its `_test.go`),
  `internal/state/state.go`, `internal/state/state_entry.go`, `internal/state/state_line.go` (create, each with its
  `_test.go`).
- Rules: GO-2, GO-3, DOC-1, GOARCH-1 to GOARCH-40 (all of `ai/rules/architecture.yaml`).
- Reuse: `legacy/workflow` (`Workflow`, `Entry`, `Cond`, `Var`, `Run`, `Context`, `Names`, `Args`, `Parse`,
  `Validate`, `TooOld`, `BaseBranches`, `EffectiveMode`, `Action`, `Strings`, `Nested`), `legacy/state` (`Entry`,
  the kinds, `Done`, `Injected`, `StopBlocks`), `pkg/glob`.
- Changes: starts after Task 2.
  `workflow`: the entity `Workflow` with its entries by event, vars, base, protected paths, `max_stop_blocks` and the
  new `Rules` holding the runners; `Valid` returns one error that joins every problem `Validate` returns today, the
  runners included; `TooOld` stays the error of a `requires` newer than the running version, which `Parse` receives
  as a value.
  `Parse` decodes `polix.yaml` through unexported yaml types and builds the domain values; a missing file is the
  default workflow, decided by the caller passing no data.
  `Bases(base, branches)` returns the base asked or else the branches; `BaseBranches` the configured ones or the
  default `main`.
  `Runner` holds a glob and an argv; `Valid` rejects an empty argv, an invalid glob and an unknown placeholder;
  `Workflow.CheckCommand(check, id)` returns the argv of the first runner whose glob matches the base name of the
  check, with `{file}`, `{dir}`, `{id}` and `{name}` expanded, or none when no runner matches.
  `state`: the entity `State` (the entries of the log and how many lines were skipped) with `Done`, `Injected` and
  `StopBlocks` as value methods, `Entry` with its kinds and `Valid`, and the decoding and encoding of one line of
  `log.jsonl` with the cut of long fields; reading and writing the file stays in the adapters, through `pkg/jsonl`.
  The conditions of the domain change nothing of today's semantics of `polix.yaml`.
- Tests: the cases of `legacy/workflow/workflow_test.go` and `legacy/state/state_test.go`; for the runners, a glob that
  matches by base name, the first of two matching runners wins, each placeholder expanded, `{dir}` of a check at the
  root is `./`, no runner matches, and `Valid` on an empty argv, `[` as glob and `{other}` in the argv.
- Verification: `just test`, expected: every package passes, `internal/workflow` and `internal/state` included.

### Task 4: Rebuild polix rules check as the use case CheckRule, with the runner

- Covers: AC3, AC4, AC7.
- Files: `internal/rule/rule.go`, `internal/rule/rule_parse.go`, `internal/rule/rule_result.go`,
  `internal/rule/rule_run.go`, `internal/rule/rule_set.go`, `internal/rule/check_rule.md`,
  `internal/rule/check_rule_in.go`, `internal/rule/check_rule_logic.go`, `internal/rule/check_rule_out.go` (create,
  each Go file with its `_test.go`), `cmd/polix/main.go` (`main`, modify), `legacy/cli/cli.go` (`rulesCmd`,
  `rulesCheck`, `newRoot`, modify), `ai/rules/architecture.yaml` (modify), `polix.yaml` (modify), `docs/polix.md`
  (modify).
- Rules: GO-2, GO-3, XPLAT-1, DOC-1, GOARCH-1 to GOARCH-40 (all of `ai/rules/architecture.yaml`).
- Reuse: the draft `examples/architecture/internal/rule/*` at commit `aae62b6` (deleted by Task 1),
  `legacy/rules` (`Load`, `Matching`, `Validate`, `RunChecks`), `workflow.Bases`, `workflow.Parse`,
  `Workflow.CheckCommand`, `pkg/git`, `pkg/proc`, `pkg/text`, `pkg/cli`.
- Changes: starts after Task 3.
  The domain of `rule` is that of the draft: `Rule` (`Valid`, `HasCheck`), `Matching`, `Parse` of `ai/rules/*.yaml`,
  `Result` (`Valid`, `Passed`, `Tail`) and `Failed`, plus `Run`, a rule with the argv its runner gives (empty to run
  the check as a program), and `Repeated`, the problems of an id repeated across files, for `polix check`.
  The use case is that of the draft with one change: the port `CheckRulePort` replaces `BaseBranches` with
  `LoadWorkflow`, which reads `polix.yaml` (the default workflow when missing) and is always called; the logic
  `CheckRule` takes the rules, the workflow, the choice of the request and the changed files and returns
  `CheckRuleOutput.Run` as a list of `rule.Run`; `RunScript` becomes `RunCheck`, which runs the argv of the run from
  the root with 5 minutes, or the file as a program through `pkg/proc` when the argv is empty.
  `check_rule.md` documents all of it, the runner included, with the sections GOARCH-14 and GOARCH-36 require.
  `cmd/polix/main.go` builds the root with `pkg/cli`, adds the commands of `legacy/cli` (which exports them instead
  of its root, without `rules`) and the command `rules` with `rule.CheckRuleCommand`, and exits with `pkg/cli.Run`.
  Every rule of `ai/rules/architecture.yaml` with `deterministic: true` gets `check:` with its file
  `tests/architecture/goarchNN_test.go`; `polix.yaml` gets `rules.run` mapping `*_test.go` to `go test {dir} -run
  ^Test{name}$`, with a comment that explains it.
  `docs/polix.md` documents `rules.run`, its placeholders, the fallback to a program, and the new behaviour of
  `polix rules check`.
- Tests: `rule_test.go`, `rule_parse_test.go`, `rule_result_test.go`, `rule_run_test.go`, `rule_set_test.go` with the
  cases of `legacy/rules/rules_test.go`; `check_rule_logic_test.go` with domain values: no check, all, `--changed`
  matching and not matching, a rule without scope, a runner and no runner; `check_rule_in_test.go` with a fake port:
  an invalid rule stops, `--base` skips the bases of `polix.yaml` but not the runners, an error of each port method
  is returned, a failed run goes to the response with its tail; `check_rule_out_test.go` against a temporary
  repository: rules loaded and ordered with `retired.yaml` left out, a missing `ai/rules/`, a broken `polix.yaml`,
  `Changed` without any base (the hint to set `base`), a check through a runner and a check as a program.
  The entry is tested for `--base` without `--changed` and for the exit codes 0 and 1.
- Verification: `just test`, expected: every package passes; `go run ./cmd/polix rules check`,
  expected: every GOARCH check runs and passes, `<n> checks, 0 failed`.

### Task 5: Rebuild polix graph and log as the use cases of workflow

- Covers: AC4, AC6.
- Files: `internal/workflow/graph_workflow.md`, `internal/workflow/graph_workflow_in.go`,
  `internal/workflow/graph_workflow_logic.go`, `internal/workflow/graph_workflow_out.go`,
  `internal/workflow/log_workflow.md`, `internal/workflow/log_workflow_in.go`,
  `internal/workflow/log_workflow_logic.go`, `internal/workflow/log_workflow_out.go` (create, each Go file with its
  `_test.go`), `cmd/polix/main.go` (`main`, modify), `legacy/cli/cli.go` (`graphCmd`, `graph`, `logCmd`, `printLog`,
  `mermaidLabel`, `condText`, modify).
- Rules: GO-2, GO-3, XPLAT-1, DOC-1, GOARCH-1 to GOARCH-40.
- Reuse: `Workflow`, `TooOld`, `state.State`, `pkg/git`, `pkg/jsonl`, `pkg/cli`.
- Changes: starts after Task 4.
  `polix check` is not here: it is `CheckSetup` of Task 8 (see the amendment in `Decisions`).
  `GraphWorkflow` (`polix graph`): the port loads the workflow; the logic builds the Mermaid text that `mermaidLabel`
  and `condText` build today.
  `LogWorkflow` (`polix log` with its flags of count and raw): the port reads the log through `pkg/jsonl`; the logic
  picks the entries to show; the entry prints them as today, with the warning of skipped lines.
  Each use case has its document; both commands leave `legacy/cli` and are added in `cmd/polix/main.go`.
- Tests: the cases of `legacy/cli/cli_test.go` for graph and log, split into the logic, shell and adapter tests of
  each use case.
- Verification: `just test`, expected: every package passes; `go run ./cmd/polix graph` in this repository,
  expected: the same output as the legacy build.

### Task 6: Rebuild polix hook as the use case AnswerHook, with the step shape

- Covers: AC4, AC6, AC8.
- Files: `internal/agent/agent.go`, `internal/agent/agent_event.go`, `internal/agent/agent_answer.go`,
  `internal/agent/agent_claude.go`, `internal/agent/agent_codex.go` (create, each with its `_test.go`),
  `internal/hook/hook.go`, `internal/hook/hook_step.go`, `internal/hook/hook_evaluation.go`,
  `internal/hook/hook_protect.go`, `internal/hook/hook_condition.go`, `internal/hook/hook_command.go`,
  `internal/hook/answer_hook.md`, `internal/hook/answer_hook_in.go`, `internal/hook/answer_hook_logic.go`,
  `internal/hook/answer_hook_out.go` (create, each Go file with its `_test.go`), `cmd/polix/main.go` (`main`,
  modify), `legacy/cli/cli.go` (`hookCmd`, `hook`, modify).
- Rules: GO-2, GO-3, XPLAT-1, DOC-1, GOARCH-1 to GOARCH-40.
- Reuse: `legacy/agent` (`Claude`, `Codex`, `Parse`, `Render`, the exit codes), `legacy/engine` (`Event`, `Result`,
  `Merge`, `Handle`, `protect`, `apply`, `matches`, `condition`, `expand`, `writes` and the parsing of commands of
  `command.go`), `state.State`, `Workflow`, `Workflow.CheckCommand`, `rule.Matching`, `rule.Result`, `pkg/shell`,
  `pkg/glob`, `pkg/git`, `pkg/proc`, `pkg/jsonl`, `pkg/text`.
- Changes: starts after Task 4.
  `agent`: the entity `Agent` (claude or codex, `Valid`), `Event` (the neutral event with its name, session, actor,
  paths, command and cwd), `Answer` (block, reasons, contexts, warnings, with a value method that merges another), and
  the pure formats of Claude Code and Codex: the input bytes to the native event name and the events, and an answer
  to the stdout, stderr and exit code the agent expects.
  `hook`: the entity `Hook` (the agent, the native name and the events of one call); `Evaluation`, the state of the
  evaluation of one event (the workflow, the event, the index of the next entry, the facts known so far: hash,
  changed files, files, rules, done facts, stop blocks, and the answer so far), changed only through its methods;
  `Step`, what the shell must do next (read one of those facts, run a command, run the checks of rules, append an
  entry to the log, or none when the answer is ready); the protections, the conditions and the heuristic of the
  commands that write files, as today.
  `AnswerHook(hook.Evaluation) AnswerHookOutput` returns the next step; the shell `answerHook` parses the input with
  the format of the agent, opens the repository and resolves the paths through the port, and for each event loops:
  it calls the logic from one call site, carries out the step through the port and feeds the result back to the
  evaluation, until the step is none; then it merges the answers into the response.
  The facts are read only when a step asks for them, as the engine reads them lazily today, and a run resets the
  hash so a later `done` is decided on the new content.
  The adapter holds the budget of 10 minutes of the call, runs commands and checks through `pkg/proc` (checks
  through the argv of their runner, or as programs), reads git through `pkg/git` and the log through `pkg/jsonl`.
  The policy of failures stays: outside a repository or without input the call lets everything through; an invalid
  workflow fails closed on edits and commands and never blocks a stop; a log that cannot be used fails open on stop.
  `answer_hook.md` documents the use case and the step shape; `hook` leaves `legacy/cli` and is added in main.
- Tests: the cases of `legacy/engine/engine_test.go`, `legacy/agent/agent_test.go` and the hook cases of
  `legacy/cli/cli_test.go`, split into domain, logic, shell and adapter tests; `answer_hook_logic_test.go` walks an
  evaluation step by step with domain values: a `run` that changes the hash followed by an entry with
  `unless: { done }`, a `record` followed by a `done`, `allow` and a blocking `deny` that end it, a `deny` in warn
  mode that does not, the loop guard after `max_stop_blocks` and when the log fails; `answer_hook_in_test.go` with a
  fake port: several events merged, an error of the port per step; `answer_hook_out_test.go` against a temporary
  repository, a check run through a runner included.
- Verification: `just test`, expected: every package passes; a session of Claude
  Code in this repository edits a Go file and stops, expected: gofmt runs on the edit and lint and tests run on stop,
  as before.

### Task 7: Rebuild polix version and polix update as the use cases of version

- Covers: AC6.
- Files: `internal/version/version.go`, `internal/version/version_parse.go`, `internal/version/version_asset.go`,
  `internal/version/version_checksum.go`, `internal/version/version_cache.go`, `internal/version/show_version.md`,
  `internal/version/show_version_in.go`, `internal/version/show_version_logic.go`,
  `internal/version/show_version_out.go`, `internal/version/update_version.md`,
  `internal/version/update_version_in.go`, `internal/version/update_version_logic.go`,
  `internal/version/update_version_out.go` (create, each Go file with its `_test.go`), `cmd/polix/main.go` (`main`,
  modify), `legacy/cli/cli.go` (`versionCmd`, modify), `legacy/cli/update.go` (`updateCmd`, `doUpdate`, `runUpdate`,
  `hookShouldCheck`, `workflowAhead`, `hookDoctor`, `refreshAfterUpdate`, modify).
- Rules: GO-2, GO-3, XPLAT-1, DOC-1, GOARCH-1 to GOARCH-40.
- Reuse: `legacy/version` (`Valid`, `Compare`, `IsRelease`), `legacy/update` (`Cache`, `LoadCache`, `SaveCache`,
  `AssetName`, the check of checksums), `pkg/build`, `pkg/github`, `pkg/proc`, `TooOld` of workflow.
- Changes: starts after Task 2 and Task 3.
  `version`: the entity `Version` (major, minor, patch, pre-release, `Valid`, `Compare`, `IsRelease`) built by
  `Parse`; the name of the asset of a system; the check of an asset against `checksums.txt`; `Cache` with the time of
  the last check, the latest release known and the last failure, and the decision of whether the session start hook
  checks now, which takes the current time as a value.
  `ShowVersion` (`polix version`): prints the version the entry passes from `pkg/build.Current`.
  `UpdateVersion` (`polix update` with `--check`, `--version` and `--hook`): the entry reads `POLIX_NO_UPDATE` and the
  current version into the request; the port asks the latest release, downloads the asset and the checksums, finds
  and replaces the executable, reads and writes the cache, tells whether this binary rejects `polix.yaml` as too old,
  and runs the new binary for `__refresh` and `doctor --hook`; the logic decides what to do and what to tell; with
  `--hook` it never fails, as today.
- Tests: the cases of `legacy/version/version_test.go`, `legacy/update/update_test.go` and the update cases of
  `legacy/cli/update_test.go` except the tests of the install scripts, split into domain, logic, shell and adapter
  tests, with a fake server of releases for the adapter.
- Verification: `just test`, expected: every package passes; `go run ./cmd/polix version`,
  expected: `dev`.

### Task 8: Rebuild check, install, uninstall, doctor and __refresh as the use cases of setup

- Covers: AC6.
- Files: `internal/setup/setup.go`, `internal/setup/setup_change.go`, `internal/setup/setup_hooks.go`,
  `internal/setup/setup_settings.go`, `internal/setup/setup_plugin.go`, `internal/setup/setup_codex.go`,
  `internal/setup/setup_finding.go`, `internal/setup/setup_problems.go` (create, each with its `_test.go`), and for
  each of `check_setup`, `install_setup`, `uninstall_setup`, `diagnose_setup` and `refresh_setup` under
  `internal/setup/`: `<stem>.md`, `<stem>_in.go`,
  `<stem>_logic.go`, `<stem>_out.go` (create, each Go file with its `_test.go`); `cmd/polix/main.go` (`main`,
  modify), `legacy/cli/cli.go` (`checkCmd`, `check`, `baseWarning`, `loadValid`, `mayNeedNewer`, `newerHint`,
  modify), `legacy/cli/install.go`, `legacy/cli/doctor.go`, `legacy/cli/update.go` (`refreshCmd`, modify).
- Rules: GO-2, GO-3, XPLAT-1, DOC-1, GOARCH-1 to GOARCH-40.
- Reuse: `legacy/rules` (`Validate`, its messages), `rule.Repeated`, `Rule.Valid`, `legacy/setup` (`Options`,
  `Change`, `Hooks`, `IsPolixCommand`, `IsUpdateCommand`, `IsLegacyCommand`, `Install`,
  `Uninstall`, `RegisterHooks`, `ReadHooks`, `Registered`, `mergeHooks`, the encoding of settings,
  `WritePlugin`, `PluginVersion`, `Refresh`, `ClaudeStatus`, `CodexEnabled`, `setCodexPlugin`), `legacy/cli`
  (`installFlags`, `resolveAgents`, `confirm`, `installNext`, `warnDuplicateHooks`, `refreshInstalled`, the checks of
  `doctorRun`), `plugins`, `version.Version`, `Workflow`, `rule.Rule`, `state.State`, `pkg/github`, `pkg/proc`.
- Changes: starts after Task 5 and Task 7.
  `setup`: the entity `Setup` (home, root, scope and agents, from the flags), `Change` (a path and what happened to
  it), the hook groups of each agent and the polix commands among them, the merge of the hooks into the bytes of a
  settings file (keeping order, symlinks and permissions are the adapter's), the files of the plugin from the bytes
  the adapter reads from `plugins` with the version of the manifest, the edit of the block of the plugin in the
  config of Codex, and `Finding` (level, area, message) for doctor; all pure, bytes in and bytes out.
  `setup_problems.go` turns a workflow, whether `polix.yaml` exists, the rules (with their problems in the messages of
  `legacy/rules.Validate`, every problem listed, not only the first), the checks that do not exist and the existence of
  the bases into the problems, hints and warnings that both `polix check` and `polix doctor` show.
  A check matched by a runner of `rules.run` is never required to be executable nor to have an interpreter.
  `CheckSetup` (`polix check`): its port loads the workflow, the rules and whether each check file and each base
  branch exists; the logic gathers the problems (workflow, rules, repeated ids, missing checks, runners) and the
  warning when no base exists and a deterministic rule has a check, and tells when a newer polix may be needed; the
  output, messages and exit codes stay those of today.
  `InstallSetup` and `UninstallSetup` (`polix install` and `polix uninstall` with their flags): the entry asks for
  confirmation when interactive, as today; the port reads and writes the settings, the plugin folder and the config
  of Codex, and calls the CLI of Claude Code through `pkg/proc`.
  `DiagnoseSetup` (`polix doctor` with `--hook` and `--offline`): the port gathers the binary, the PATH, the latest
  release, the workflow, the base, the rules, the hooks of the settings, the plugin, Codex and the log; the logic
  turns them into findings and the summary; output and exit codes as today.
  `RefreshSetup` (hidden `polix __refresh` with `--quiet` and `--hooks`): refreshes the plugin and, with `--hooks`, the
  hooks of the repository, printing the line in Spanish of today with `--quiet`.
  The commands leave `legacy/cli` and are added in `cmd/polix/main.go`.
- Tests: the cases of `legacy/setup/setup_test.go`, `legacy/cli/install_test.go`, `legacy/cli/doctor_test.go` and
  the check cases of `legacy/cli/cli_test.go`, plus the problems of the runners and checks matched by a runner,
  split into domain, logic, shell and adapter tests; the adapters run against a temporary home and repository with a
  fake CLI of Claude Code on the PATH.
- Verification: `just test`, expected: every package passes; `go run ./cmd/polix check` and
  `go run ./cmd/polix doctor --offline`, expected: the same output as the legacy build, except that the checks of the
  GOARCH rules, matched by a runner, are not reported as not executable.

### Task 9: Delete legacy/, finish the wiring and bring the docs, the tests of the installers and CI up to date

- Covers: AC5, AC6, AC9, AC10.
- Files: `legacy/**` (delete), `tests/install/install_test.go` (create), `cmd/polix/main.go` (`main`, modify),
  `scripts/dist.sh` (modify), `AGENTS.md` (modify), `docs/polix.md` (modify), `examples/polix.yaml` (modify),
  `plugins/polix/skills/add-rule/SKILL.md`, `plugins/polix/skills/doctor/SKILL.md`,
  `plugins/polix/agents/rules-author.md` (modify), `ai/tasks/rule-tests-runner.md` (delete).
- Rules: GO-2, GO-3, XPLAT-1, XPLAT-2, DOC-1, GOARCH-6, GOARCH-13, GOARCH-39.
- Reuse: `TestInstallScript`, `TestInstallPowerShell` and their helpers (`fakeBinary`, the fake server of releases)
  from `legacy/cli/update_test.go`.
- Changes: starts after Tasks 4 to 8, when `legacy/cli` exports no command any more.
  `cmd/polix/main.go` no longer imports `legacy/`; `legacy/` is deleted, after checking that every case of its tests
  has a counterpart in the new tests.
  The tests of `install.sh` and `install.ps1` move to `tests/install/`, building the binary with the flag
  `-X github.com/mlophez/polix-ai/pkg/build.Version`; `scripts/dist.sh` uses the same flag.
  `AGENTS.md` lists the new layout (`internal/` packages, `pkg/`, `tests/`) and where the tests live;
  `docs/polix.md` replaces the paths of the old packages (`internal/agent`, `internal/engine`) in its layers and the
  answer of a hook, and links `docs/architecture.md`; `examples/polix.yaml` annotates `rules.run` with its
  placeholders; the skill `add-rule` and the agent `rules-author` say that `check` names any file and that a runner
  of `rules.run` runs it; the skill `doctor` mentions the problems of the runners; the task of the runner is removed.
- Tests: `tests/install/install_test.go` runs the cases of today on every system of CI, PowerShell 5.1 and 7 on
  Windows included.
- Verification: `just check`, expected: install, `polix check`, `polix rules check` and `polix doctor --offline` pass;
  `claude plugin validate plugins/polix`, expected: valid.

## Risks

- Behaviour lost in the rewrite of about 10,000 lines: each task ports the cases of the legacy tests of what it
  replaces, and Task 9 deletes `legacy/` only after checking every case has a counterpart.
- Tasks run at the same time collide in `cmd/polix/main.go` and `legacy/cli`: each task only adds its own commands to
  main and removes its own from legacy, so the merges are additions and removals of separate lines.
- The tolerant type check of the architecture tests accepts what it cannot resolve: once `legacy/` is gone every import
  of the module resolves, so `go test ./tests/architecture` at the end of Task 9 is the strict run.
- The step shape of the hook slows the hot path: the facts stay lazy and the shell loops in memory, and the latency of
  a hook on this repository is compared with the legacy build in Task 6.
- On Windows `go` and `bash` may resolve to the wrong program: runners run through `pkg/proc`, which skips the
  Windows folder and the Store aliases, and CI runs `polix rules check` on the three systems.
- `polix rules check` gets slower, one `go test` per rule: Go caches the build of the test package, so only the first
  run compiles.
- The step shape is a second shape of use case: GOARCH-20 and `docs/architecture.md` limit it to use cases whose plan
  depends on the result of an earlier step, and the review rejects it elsewhere.

## Verification

- `just test`: every package passes, `tests/architecture` and `tests/install` included.
- `golangci-lint run ./...`: no findings.
- `just check`: `polix check`, `polix rules check` (every GOARCH check passes) and `polix doctor --offline` pass.
- `claude plugin validate plugins/polix`: valid.
- `ls legacy examples/architecture internal/cli internal/engine internal/gitx internal/glob internal/rules
  internal/shell internal/update`: none of them exists.
- End to end: `just install`, then in a session of Claude Code in this repository edit a Go file with a format error
  and end the turn: the edit is formatted, the stop runs lint and tests, and a second stop without changes runs
  nothing; `polix log` shows the decisions.
- End to end: in a temporary repository run `polix install --yes --agent claude`, `polix doctor`, `polix uninstall
  --yes`: the same output as the legacy build.
- Push a branch and check that the CI workflow passes on Linux, macOS and Windows.
