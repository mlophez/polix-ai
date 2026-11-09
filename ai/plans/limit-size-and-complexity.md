# Limits of size and complexity, domains without free functions, loaders injected by main and AnswerHook by family

## Overview

Today nothing stops a Go function or file from growing without end.
The domain packages hold loose functions, among them the parsers of YAML and of the hooks of the agents.
And the hook command handles nine kinds of events in one 1224-line file.
This change sets limits on the size and the complexity of the code and brings the whole project under them.

- A function may have a cyclomatic complexity of at most 15 and a cognitive complexity of at most 20.
  A Go file may have at most 400 lines; past that, part of it moves to a sibling file.
  The tests are left out of the three limits.
- A domain holds only types, interfaces, constants and methods: no function that is not a method.
  Logic that only one command uses moves into that command.
  Logic shared by several commands becomes a method of a domain type.
- Translating a format is not domain: it lives on the input or output side of a command.
  - Reading the hooks of Claude Code and Codex is one interface with one implementation per agent, in the entry of
    the hook command.
  - Reading `polix.yaml`, the rule files and the update cache belongs to new load commands.
    The program builds them once at start and injects them into the commands that need them.
    They load lazily, once per directory, so a command that never asks pays nothing, and tomorrow they could read
    JSON instead of YAML.
- A command may split its entry, its logic and its adapter into sibling files; only the entry and the adapter talk
  to the outside.
- The hook command routes each event to the logic of its family, one file each:
  - session: start and prompt;
  - edit: pre and post edit;
  - command: pre and post command, pre commit;
  - stop: stop and subagent stop.

  Each family reads top-down and calls a shared engine for the entries of `polix.yaml`, their conditions and their
  verbs.
- The linter and the architecture tests enforce all of it.
- The behaviour of polix does not change.

## Context

The plan of one package per use case deferred the limits of size and complexity to their own plan.
Today:
- 32 functions of production code go past 15 cyclomatic or 20 cognitive, and 9 files go past 400 lines;
- the domain packages hold 152 functions that are not methods.
  Among them, the YAML decoding of `polix.yaml` (`workflow.Parse`, called by six adapters), of the rule files, of
  the update cache and of the log, and the parsing of the input of Claude Code and Codex.
- `internal/app/hook/answer/logic.go` mixes the engine of the workflow with the policy of each event behind `if`
  statements on its name.

The outcome is:
- the three limits;
- domains without free functions;
- the translations of formats on the input and output side, with shared loaders injected by `main`;
- AnswerHook routed by family;

all enforced by the linter and the architecture tests.

## Decisions

### Limits

- gocyclo with `min-complexity: 15` and gocognit with `min-complexity: 20`; both report strictly above the value.
- revive `file-length-limit` with `max: 400`, counting every physical line, and `enable-default-rules: true`.
- The `_test.go` files are out of the three limits by one exclusion of `.golangci.yml`.
  GO-3 is amended to name it as its only exception.
- The limits become GO-4 of `ai/rules/go.yaml`, deterministic, with its check `TestGO4`.
- Everything over the limits is refactored now: no baseline, no ratchet, no `//nolint` on the limits.
- The limits are added to the settings first and enabled in the last task.

### Domains

- A domain package (`internal/app/<noun>/*.go`, tests aside) declares types, interfaces, constants, variables that
  are not functions, and methods of its types (structs or named types such as `workflow.Cond`).
  It declares no function that is not a method, no `init`, and no package variable whose value is a function.
  This is the new rule GOARCH-44, with its check `TestGOARCH44`; GOARCH-10 keeps its wording.
- A domain keeps the validation of its values (`Valid`, `Workflow.Problems`), but never the decoding of a format.
- Each free function of a domain goes to one of these places:
  - a translation of a format goes to the input or output side (see `Translations`);
  - logic that several use cases share, or that a shell asks (GOARCH-20, GOARCH-37), becomes a method of the domain
    type it works on;
  - logic of only one use case goes to its `logic.go` or a `logic_<topic>.go`.
- Its tests move with it.

### Files of a use case

- A use case folder holds `in.go`, `logic.go` and `out.go`, and may add siblings `in_<topic>.go`,
  `logic_<topic>.go` and `out_<topic>.go` with their tests.
  GOARCH-1 and GOARCH-3 admit them.
  Each sibling is checked by the rules of its kind as the main file of that kind:
  - the in files: GOARCH-12, 15, 20, 22, 32, 37;
  - the logic files: GOARCH-4, 23, 41;
  - the out files: GOARCH-5, 24.
- `Decide` and `Output` stay in `logic.go`; `Command`, `Request`, `Response` and `Port` stay in `in.go`; `Adapter`
  stays in `out.go`.
  Their siblings export nothing; the methods of `Adapter` may be declared in any out file.
- GOARCH-12 is amended: besides `Port`, the in files may declare one unexported interface of the transport formats
  the entry reads, implemented by unexported types of the in files.
- GOARCH-24 is amended: the out files may declare unexported types that decode or encode a format, next to
  `Adapter`.
- GOARCH-32 is amended:
  - the port and the adapter are called only from the functions and methods of the in files other than the entry
    `Command`, and from the methods of the adapter;
  - `run` still is the only caller of `Decide`, from one call site.
- The subfolders of a use case (helpers) are not used here; their role stays as the rules say today.

### Translations

- The input of a hook (Claude Code, Codex) is translated by the entry of AnswerHook, as GOARCH-22 asks of an entry.
  - The in files of AnswerHook declare the unexported interface `format`, with a method that parses the native
    input into domain events and one that renders a domain answer into stdout, stderr and an exit code.
  - It has one implementation per agent: `claude` in `in_claude.go` and `codex` in `in_codex.go`.
  - The entry chooses it from the agent of the flags, and the code of the domain `agent` that parses and renders
    moves there.
  - The domain `agent` keeps `Agent`, `Event`, `Answer` and their methods.
- A file that several use cases read is loaded by a load use case: `workflow/load` for `polix.yaml`, `rule/load`
  for the rule files and `version/load` for the update cache.
  - Its out files read the file and decode it; its logic validates the result with the domain.
  - Its entry is not a cobra command: `Command` returns the loader, a value of a named function type of its domain
    (`workflow.Load`, `rule.Load`, `version.Load`).
  - The loader returns a domain value, `workflow.Loaded`, `rule.Loaded` or `version.Loaded`, with what was loaded,
    whether the file exists and its problems.
  - It never fails; each use case decides what an absent or invalid file means to it.
- `cmd/polix/main.go` builds each loader once and passes it to the `Command` of every use case that needs it.
  - The entry puts it in a field of the adapter, and the adapter calls it in the port method that loads (for
    example `LoadWorkflow`), with the directory the use case resolved.
  - The loader is lazy and caches its result per directory for the life of the process.
  - The ports, the shells and the logic of the users do not change.
- A file that only one use case reads or writes is translated in its out files: the log of `state` in AnswerHook,
  the writing of the update cache in UpdateVersion, the settings of the agents in their setup use cases.
- A value that comes from the flags is translated by the entry that reads it, using the domain to validate it.

### AnswerHook

- `Decide` is a router: a `switch` on the name of the event in `logic.go` calls one of the methods `session`,
  `edit`, `command` and `stop` of `pass`, each in its `logic_<family>.go`.
- A family reads top-down: it calls the engine as a library and holds only what is its own.
  - session: the session context and the started mark of start, and the discard of injected guidance on a blocked
    prompt.
  - edit and command: the protections of their events; command also holds the commits outside the change of
    pre_commit.
  - stop: the loop guard of `max_stop_retries` and the finished observation of subagent_stop.
  - The informed events turn a block into a warning (`workflow.Informed`).
- The engine is the rest, in `logic_<topic>.go` files of AnswerHook:
  - the step machinery and the fact readers;
  - the protections;
  - the walk of the entries and their verbs;
  - the conditions and placeholders;
  - the runs and their gates;
  - the injection;
  - the parsing of command lines;
  - the log.

  It has no `if` on the name of an event beyond the routing.
- The free functions of `internal/app/hook` move into those files, since only AnswerHook uses them.
  The domain keeps `Hook`, `Evaluation`, `Facts`, `Step`, `Outcome` and `RunBudget`.
- The shell keeps one port and one adapter, and carries out each step through unexported functions of its in files.

### Assumptions

- Assumption: the engine files are named after their topic, for example `logic_entries.go`, `logic_condition.go`,
  `logic_protect.go`, `logic_cmdline.go`, `logic_run.go`, `logic_inject.go` and `logic_facts.go`.
- Assumption: the update cache is the only file of `version` worth a loader; the log of `state` stays in AnswerHook,
  its only reader.

### Adjustments during the implementation

- `workflow.Load` takes the context, the root and the running version, since decoding needs it (`TooOld`,
  `requires`); the loader caches per root and running version, and the ports keep their `running` parameter.
- Task 7 runs in two steps, one after the other: first `rule/load` and `version/load`, then `workflow/load`.
- Task 7 also touches `internal/app/workflow/workflow_shortcuts.go` and `workflow_entry.go` with their tests:
  the YAML decoding of the guardrails leaves the domain with the rest of `workflow_parse.go`.
  The domain exposes exported types and methods for what the decoder needs (the expansion of the shortcuts, their
  problems merged by line, the keys of an entry), so nothing is duplicated.
- Task 7 also rewrites the domain tests that call `workflow.Parse` (`workflow_entry_test.go`,
  `workflow_shortcuts_test.go`, `workflow_layout_test.go`, `workflow_base_test.go`,
  `internal/app/hook/hook_evaluation_test.go`): they build domain values, or move to the tests of `workflow/load`.
  `tests/setup/setup_test.go` builds and passes the loaders as `main` does.
- GOARCH-9 is amended: the `_test.go` files of a use case may import load use cases, and only them, to build the
  loaders as `main` does; so the tests that run commands on real files keep using the real decoding.
- The log of `state` gets its own load use case, `state/load`, instead of staying in the out files of AnswerHook:
  `polix log` (LogWorkflow) reads it too, so it follows the rule of a file that several use cases read.
  Its loader reads the log at every call, since AnswerHook appends to it in the process that reads it.
- The settings files of Claude Code and Codex (hooks, Codex config table, marketplace manifests) are read and written
  by a new library, `internal/platform/agentconf`, which the out files of the setup use cases wrap.
  Four setup use cases share that technology, and it holds no knowledge of polix, which the callers pass in
  (GOARCH-19), so it is a library instead of a copy in each out file.

## Approach

The work goes package by package, with the existing tests as the proof that the behaviour does not change.

Rules and checks: `tests/architecture/` learns:
- the sibling files of each kind;
- the format interface of the in files;
- the decoding types of the out files;
- the amended GOARCH-32;
- the load use cases;
- GOARCH-44.

`TestGO4` runs gocyclo, gocognit and revive as the tests of depguard run depguard, and `TestRulesChecks` reads
every rule file.

The load use cases come before the domains lose their parsers, so every caller switches to an injected loader first.
AnswerHook takes the formats of the agents into its entry, then routes by family with the engine and the code of the
domain `hook` in its logic files.
Each domain then loses its remaining free functions and splits what goes past the limits.

Contracts that change:

- `.golangci.yml`: three more limits on every lint run; the list GOARCH-41-logic covers the logic siblings.
- The rules:
  - GOARCH-1, 3, 6, 11, 12, 14, 15, 23, 24, 32 and 41 amended, and GOARCH-44 added;
  - GO-3 amended and GO-4 added.
- The Go API between packages: free functions of the domains become methods or move into use cases.
  The `Command` of the users of a loader takes it as a parameter, and `cmd/polix/main.go` wires the loaders.
- No command line, `polix.yaml` schema, hook configuration, port or behaviour of the binary changes.

Errors:
- A breach fails `golangci-lint run ./...` or the architecture test of its rule, naming the file, the line and the
  cause.
- A loader never returns an error: the problems of an absent or invalid file are data of its result, reported by
  each use case as today.

### Discarded alternatives

- A baseline with a ratchet, or `//nolint` on each offender: the debt is paid now.
- An exception of GOARCH-44 for the `Parse*` functions: parsing translates a format, so it is not domain.
- A new layer `internal/adapter/` for shared translations: the load use cases hold them without a new depth.
- Loading eagerly in `main` from the working directory: the hook knows its directory only from the event on stdin.
- One use case per event with its own command: it changes the hooks that setup writes.
- One subfolder per event family: helpers cannot import each other, so the engine would leave the use case.
- The engine in the domain `hook`: only AnswerHook uses it (GOARCH-10).
- Limits on the tests, and funlen.

## Acceptance criteria

- AC1: `golangci-lint run ./...` reports nothing with gocyclo (15), gocognit (20) and revive `file-length-limit`
  (400) enabled, the other revive defaults still on.
- AC2: a non-test function above 15 cyclomatic or 20 cognitive, and a non-test Go file above 400 lines, are
  reported; a `_test.go` never is.
  `TestGO4` passes on the module and finds each limit broken in `tests/architecture/testdata/limits`.
- AC3: `TestGOARCH32` accepts an unexported function of an in file that calls the port.
  It still reports:
  - the entry `Command` calling the port or the adapter;
  - a logic or out file calling them;
  - any function but `run` calling `Decide`;
  - `run` calling `Decide` other than once.
- AC4: `TestGOARCH44` passes on the module.
  On the bad testdata it reports a free function in a domain file, exported or not, an `init` and a package variable
  whose value is a function literal.
  It accepts methods of structs and of named types, and the test files.
- AC5: no domain package of `internal/app/` declares a function that is not a method.
  None decodes YAML nor JSON, and none parses the input of an agent.
- AC6: the architecture tests accept `in_<topic>.go`, `logic_<topic>.go` and `out_<topic>.go`, and check each as
  its kind.
  They also accept one unexported interface of formats in the in files and unexported decoding types in the out
  files.
  They report:
  - a sibling that exports a name;
  - a second interface in the in files;
  - an exported type in an out file other than `Adapter`;
  - a logic sibling that breaks GOARCH-4 or GOARCH-41.
- AC7: `workflow/load`, `rule/load` and `version/load` exist, each with its document, and its `Command` returns its
  loader.
  `cmd/polix/main.go` builds each once and passes it to every use case that reads that file.
  No other use case decodes `polix.yaml`, the rule files nor the update cache.
  A loader called twice for one directory reads the file once.
- AC8: AnswerHook parses and renders only through the `format` of its in files, with `claude` and `codex` as its
  implementations.
  Its `Decide` routes each event to `session`, `edit`, `command` or `stop`, each declared in its
  `logic_<family>.go`.
  The engine has no `if` on the name of an event beyond the routing.
- AC9: `just test` passes.
  A test changes only where the function it calls moved, became a method or is reached through a loader, and keeps
  its cases.
- AC10: the rule files say what AC2 to AC7 check.
  `TestRulesChecks` checks every rule file but `retired.yaml`.
- AC11: `docs/architecture.md` describes:
  - the limits;
  - the domain without free functions;
  - the sibling files;
  - where the translations of formats live;
  - the load use cases and their injection from `main`;
  - the amended GOARCH-32.

  `CONTRIBUTING.md` names GO-4 among the rules whose tests run golangci-lint.
  The document of each touched use case describes the same behaviour and names its code as GOARCH-36 asks.
- AC12: `just check` passes, and polix answers the hooks of a real session of Claude Code as before.

## Out of scope

- The role of the subfolders of a use case (helpers).
- One command per event in the hooks that setup writes.
- `Step` as a sealed interface and smaller ports for AnswerHook.
- Typed conditions instead of `workflow.Cond` as a map, a domain `event`, and moving the runners and the weights out
  of `workflow`.
- Reading `polix.yaml` from JSON: the loader makes it possible, no other format is added.
- Limits on the tests and on the length of a function.
- `AGENTS.md`: protected; no line of it names what this plan moves.

## Tasks

### Task 1: The limits in the settings of .golangci.yml, not enabled yet

- Covers: none.
- Depends on: none.
- Files: `.golangci.yml` (`linters.settings`, `linters.exclusions`, modify), `ai/rules/go.yaml` (GO-3, modify).
- Rules: GO-3, GOARCH-41, GOARCH-42, GOARCH-43.
- Reuse: the comments and the layout of the settings of depguard in `.golangci.yml`.
- Changes: add under `linters.settings`:
  - gocyclo with `min-complexity: 15`;
  - gocognit with `min-complexity: 20`;
  - revive with `enable-default-rules: true` and the rule `file-length-limit` with `max: 400`.

  Add under `linters.exclusions.rules` one rule for the paths ending in `_test.go`.
  It covers gocyclo and gocognit, and revive only for the text `file-length-limit`.
  A comment says these are the limits of GO-4.
  Do not add gocyclo nor gocognit to `linters.enable` yet.
  Amend GO-3 so this exclusion of the tests is its only exception.
- Tests: none; golangci-lint checks its configuration.
- Verification: `golangci-lint config verify`, expected: no error.
  `golangci-lint run ./...`, expected: no finding.
  `golangci-lint run --enable-only gocyclo,gocognit,revive ./...`, expected: only the functions and files over the
  limits, none in a `_test.go`.

### Task 2: The rules of the files of a use case

- Covers: AC3, AC6, AC10, AC11.
- Depends on: Task 1.
- Files:
  - `ai/rules/architecture.yaml` (GOARCH-1, 3, 12, 15, 23, 24, 32, 41, modify);
  - `.golangci.yml` (`files` of the list GOARCH-41-logic, modify);
  - `tests/architecture/module_test.go` (the kinds of the files of a use case, modify);
  - `tests/architecture/goarch01_test.go`, `goarch03_test.go`, `goarch04_test.go`, `goarch05_test.go`,
    `goarch12_test.go`, `goarch15_test.go`, `goarch23_test.go`, `goarch24_test.go`, `goarch32_test.go`,
    `goarch37_test.go` and `goarch41_test.go` (modify);
  - the files of `tests/architecture/testdata/bad/` and `tests/architecture/testdata/depguard/` that their
    expectations need (create or modify);
  - `docs/architecture.md` (layout, Port, Adapter and Names, modify).
- Rules: GO-2, GO-3, GOARCH-41, GOARCH-42, GOARCH-43, XPLAT-1.
- Reuse: `module.useCases`, the kinds `kindIn`, `kindLogic` and `kindOut`, `checkShellShape`, `portCalls`,
  `logicCalls` and `checkDepguard` of `tests/architecture/`.
- Changes: the kind of `in_<topic>.go`, `logic_<topic>.go` and `out_<topic>.go` is that of its main file, and so is
  the kind of their tests.
  The rules of `Files of a use case` in `Decisions` are written and checked:
  - GOARCH-1 and GOARCH-3 admit the siblings;
  - GOARCH-4, 5, 15, 23, 24, 37 and 41 apply to every file of their kind;
  - GOARCH-12 admits one unexported interface of formats in the in files, implemented by unexported types;
  - GOARCH-24 admits unexported types in the out files;
  - GOARCH-32 accepts port calls from every function and method of the in files but `Command`.

  The list GOARCH-41-logic of depguard adds the glob `**/internal/app/*/*/logic_*.go`.
  In the bad testdata, `load` and `closeAll` of `stock/count/in.go` become accepted cases.
  The promoted method through an embedded field keeps a reported case from `Command` or from another file.
  New bad cases:
  - a sibling that exports a name;
  - a second interface in an in sibling;
  - an exported type in an out sibling;
  - a logic sibling that reads the clock;
  - one that imports a denied package, in `testdata/depguard`.

  New good cases: one sibling of each kind, an unexported format interface and a decoding type.
  `docs/architecture.md` describes the siblings, the format interface, the decoding types and the amended
  GOARCH-32.
- Tests: the listed `TestGOARCH*` expect the new bad cases and accept the good ones; their other expectations stay.
- Verification: `just test -run 'TestGOARCH(1|3|4|5|12|15|23|24|32|37|41)$|TestRulesChecks'`, expected: pass.
  `golangci-lint run ./...`, expected: no finding.

### Task 3: yamlfmt under the limits

- Covers: AC1, AC9.
- Depends on: Task 1.
- Files: `internal/platform/yamlfmt/yamlfmt.go` (`Format`, `flowComment`, `(*printer).readBody`,
  `(*printer).readTrivia`, modify), new siblings `internal/platform/yamlfmt/yamlfmt_<topic>.go` (create),
  `internal/platform/yamlfmt/yamlfmt_test.go` (modify only if a tested unexported function changes).
- Rules: GOARCH-1, GOARCH-7, GOARCH-13, GOARCH-17, GOARCH-19, GOARCH-35, GOARCH-38, GOARCH-39, GOARCH-42,
  GOARCH-43, GO-2, GO-3, XPLAT-1.
- Reuse: the types and helpers of `internal/platform/yamlfmt/yamlfmt.go`.
- Changes: split `yamlfmt.go` (777 lines) by topic into siblings of at most 400 lines.
  For example, the reading of the source and the writing of comments go apart from `Format`.
  Split `Format` (cyclomatic 18), `flowComment` (24 and 23), `readBody` (cognitive 21) and `readTrivia` (18 and 32)
  into unexported functions or methods named after their steps, until all are within 15 and 20.
  The output does not change.
- Tests: the tests of `yamlfmt_test.go` pass unchanged.
- Verification: `golangci-lint run --enable-only gocyclo,gocognit,revive ./internal/platform/yamlfmt/...`,
  expected: no finding.
  `go test ./internal/platform/yamlfmt/`, expected: pass.

### Task 4: git under the limits

- Covers: AC1, AC9.
- Depends on: Task 1.
- Files: `internal/platform/git/git.go` (`(*Repo).ResolveBase` and the methods that move, modify), new siblings
  `internal/platform/git/git_<topic>.go` (create), `internal/platform/git/git_test.go` (modify only if a tested
  unexported function changes).
- Rules: GOARCH-1, GOARCH-7, GOARCH-13, GOARCH-17, GOARCH-19, GOARCH-35, GOARCH-38, GOARCH-39, GOARCH-42,
  GOARCH-43, GO-2, GO-3, XPLAT-1.
- Reuse: `Repo.git`, `Repo.paths` and the unexported helpers of `internal/platform/git/git.go`.
- Changes: split `git.go` (533 lines) by topic into siblings of at most 400 lines.
  For example, the statistics of the diff and the resolution of the base branch go apart from the opening of the
  repository.
  Split `ResolveBase` (cognitive 21) into unexported steps until it is within 20.
  Same results and errors.
- Tests: the tests of `git_test.go` pass unchanged.
- Verification: `golangci-lint run --enable-only gocyclo,gocognit,revive ./internal/platform/git/...`,
  expected: no finding.
  `go test ./internal/platform/git/`, expected: pass.

### Task 5: shell, jsonl and file under the limits

- Covers: AC1, AC9.
- Depends on: Task 1.
- Files: `internal/platform/shell/shell.go` (`Split`, modify), `internal/platform/jsonl/jsonl.go` (`readLines`,
  modify), `internal/platform/file/file.go` (`Sync`, modify).
- Rules: GOARCH-1, GOARCH-7, GOARCH-13, GOARCH-17, GOARCH-19, GOARCH-35, GOARCH-38, GOARCH-39, GOARCH-42,
  GOARCH-43, GO-2, GO-3, XPLAT-1.
- Reuse: the helpers of each file.
- Changes: split `shell.Split` (cyclomatic 26, cognitive 23) into unexported functions, until it is within 15 and 20.
  For example, the handling of each quote and escape state gets its own function.
  Split `jsonl.readLines` (cognitive 21) and `file.Sync` (16 and 24) the same way.
  Same results and errors, Windows paths included.
- Tests: the tests of the three packages pass unchanged.
- Verification: `golangci-lint run --enable-only gocyclo,gocognit,revive ./internal/platform/...`,
  expected: no finding in shell, jsonl nor file.
  `go test ./internal/platform/...`, expected: pass.

### Task 6: The formats of the agents in the entry of AnswerHook

- Covers: AC5, AC8, AC9.
- Depends on: Task 2.
- Files:
  - `internal/app/agent/agent.go`, `agent_claude.go`, `agent_codex.go`, `agent_event.go`, `agent_answer.go` and
    their tests (modify or delete);
  - `internal/app/hook/answer/in.go` (`Command`, `run`, modify);
  - `internal/app/hook/answer/in_claude.go`, `in_codex.go` and their tests (create);
  - `internal/app/hook/answer/in_test.go` and `answer.md` (modify).
- Rules: GOARCH-1, GOARCH-2, GOARCH-3, GOARCH-7, GOARCH-9, GOARCH-10, GOARCH-12, GOARCH-13, GOARCH-14, GOARCH-15,
  GOARCH-16, GOARCH-17, GOARCH-20, GOARCH-21, GOARCH-22, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-29,
  GOARCH-30, GOARCH-32, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, GOARCH-41, GOARCH-43,
  GO-2, GO-3, XPLAT-1.
- Reuse: `parseClaude`, `parseCodex`, the context events and `Agent.Render` of `internal/app/agent/`.
- Changes: `in.go` of AnswerHook declares the unexported interface `format`.
  It has a method that parses the native input into the native event name and domain events, and a method that
  renders an `agent.Answer` into stdout, stderr and an exit code.
  `in_claude.go` declares the unexported type `claude` and `in_codex.go` the type `codex`, which implement it.
  Each receives as its methods the parsing and rendering code of its agent from the domain `agent`, with its
  unexported helpers.
  The entry chooses the format from the agent of the request, through the domain `Agent` for the validation of the
  name.
  `run` parses and renders only through it.
  The domain `agent` keeps `Agent`, `Event`, `Answer`, their constants and methods, and no function that is not a
  method.
  `agent.Answer` keeps its methods `Inform`, `Warn`, `Merge` and the others the logic uses.
  The events, the answers and their rendering do not change for either agent.
  `answer.md` describes the formats in Request, Flow and Errors.
- Tests: the tests of `agent_claude_test.go` and `agent_codex_test.go` move to `in_claude_test.go` and
  `in_codex_test.go` with their cases.
  The tests of `in_test.go` pass with their cases, for both agents.
- Verification: `go test ./internal/app/agent/ ./internal/app/hook/... ./tests/architecture/`, expected: pass.
  `golangci-lint run --enable-only gocyclo,gocognit,revive ./internal/app/agent/ ./internal/app/hook/answer/`,
  expected: no finding in the files of this task.

### Task 7: The load use cases of polix.yaml, the rule files and the update cache, injected by main

- Covers: AC5, AC7, AC9, AC10, AC11.
- Depends on: Task 2, Task 6.
- Files:
  - new `internal/app/workflow/load/`, `internal/app/rule/load/` and `internal/app/version/load/`: `in.go`,
    `logic.go`, `out.go`, `out_<topic>.go` if needed, `load.md` and tests (create);
  - `internal/app/workflow/workflow.go` (`Load`, `Loaded`, modify) and `internal/app/workflow/workflow_parse.go`
    (`Parse`, the `*YAML` types and their decoding, modify or delete), and their tests;
  - `internal/app/rule/rule.go` (`Load`, `Loaded`, modify) and `internal/app/rule/rule_parse.go` (`Parse`,
    `ParseAll`, `ParseFiles`, `ParseFilesAll`, modify or delete), and their tests;
  - `internal/app/version/version.go` (`Load`, `Loaded`, modify) and `internal/app/version/version_cache.go`
    (`ParseCache`, modify), and their tests;
  - `internal/app/setup/setup_problems.go` (`ParseWorkflow`, modify);
  - the users, each in its `in.go`, `out.go` and tests: `hook/answer`, `rule/check`, `setup/check`,
    `setup/diagnose`, `workflow/graph`, `diff/measure` and `version/update` (modify);
  - `cmd/polix/main.go` (`main`, modify);
  - `ai/rules/architecture.yaml` (GOARCH-6, 11, 14, 15, modify) and `tests/architecture/goarch06_test.go`,
    `goarch11_test.go`, `goarch14_test.go`, `goarch15_test.go` and the testdata they need (modify);
  - `docs/architecture.md` (modify).
- Rules: GOARCH-1, GOARCH-2, GOARCH-3, GOARCH-4, GOARCH-5, GOARCH-6, GOARCH-7, GOARCH-8, GOARCH-9, GOARCH-10,
  GOARCH-11, GOARCH-12, GOARCH-13, GOARCH-14, GOARCH-15, GOARCH-16, GOARCH-17, GOARCH-18, GOARCH-19, GOARCH-20,
  GOARCH-21, GOARCH-22, GOARCH-23, GOARCH-24, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-29, GOARCH-30,
  GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, GOARCH-41, GOARCH-42,
  GOARCH-43, GO-2, GO-3, XPLAT-1, DOC-1.
- Reuse:
  - the decoding of `workflow_parse.go` (`workflowYAML` and the ordered decoding of runners and weights);
  - the validation of `validator` and `Workflow.Problems`;
  - `rule_parse.go`, `version_cache.go`;
  - the `LoadWorkflow` and `LoadRules` methods of the ports and adapters of the users;
  - `internal/platform/file`.
- Changes: each domain declares its loader type and its result.
  - `workflow.Load`: a named function type that takes a context and the root of a repository and returns
    `workflow.Loaded`.
  - `workflow.Loaded`: the `Workflow`, whether `polix.yaml` exists and its problem, with `Valid`.
  - `rule.Load` and `rule.Loaded` do the same for the rule files of a folder of a root.
  - `version.Load` and `version.Loaded` do the same for the update cache.

  Each load use case:
  - its out files read the file through `internal/platform/file` and decode it, with the decoding types that leave
    the domain;
  - its logic validates the decoded value with the domain and builds the result;
  - its shell sequences both;
  - its `Command` returns the loader, which calls the shell once per directory and caches the result for the life
    of the process.

  GOARCH-11 and GOARCH-15 say that the `Command` of a load use case returns the loader type of its domain instead
  of a cobra command; GOARCH-14 lets its Trigger name `main`.
  GOARCH-6 says that `main` builds each loader once and passes it to the `Command` of its users.
  `main` builds the three loaders and passes them to the `Command` of every use case that read those files.
  Each such entry puts the loader in a field of its adapter.
  The port methods that load call it, so the ports, the shells and the logic of the users do not change.
  `workflow.Parse`, `rule.Parse`, `rule.ParseAll`, `rule.ParseFiles`, `rule.ParseFilesAll`,
  `version.ParseCache` and `setup.ParseWorkflow` leave their domains.
  The problems reported for an absent, unreadable, too new or invalid file do not change, nor their text and order.
  `docs/architecture.md` describes the load use cases and the injection from `main`.
- Tests: each load use case has `logic_test.go` with domain values, `in_test.go` with a fake port, and
  `out_test.go` on a temporary folder.
  They cover a valid file, an absent one, an unreadable one, an invalid one and a second call for the same directory
  that does not read again.
  The tests that parsed through the removed functions call the load use case or build domain values, with their
  cases.
  The architecture tests accept a load use case and report a non-cobra `Command` in another use case.
- Verification: `go test ./internal/... ./tests/...`, expected: pass.
  `just build`, then `build/polix config check` and `build/polix graph` in this repository, expected: the same
  output as before the task.

### Task 8: AnswerHook routed by event family, with the engine and the code of the domain hook in its logic

- Covers: AC1, AC5, AC8, AC9, AC11.
- Depends on: Task 2, Task 6, Task 7.
- Files:
  - `internal/app/hook/hook_condition.go`, `hook_protect.go`, `hook_command.go` and their tests (delete);
  - `internal/app/hook/hook_evaluation.go`, `hook_step.go` (`(Step).None`), `hook.go` and their tests (modify);
  - new siblings `internal/app/hook/hook_<topic>.go` (create), if `hook_evaluation.go` still goes past 400 lines;
  - `internal/app/hook/answer/logic.go` (`Decide`, `pass`, `handle`, modify);
  - new `internal/app/hook/answer/logic_session.go`, `logic_edit.go`, `logic_command.go`, `logic_stop.go` (create);
  - new engine files `internal/app/hook/answer/logic_<topic>.go` and their tests (create);
  - `internal/app/hook/answer/in.go` (`run`), `in_<topic>.go` if needed, `out.go`, `out_<topic>.go` for the log of
    `state`, and `answer.md` (modify or create);
  - `internal/app/state/state_line.go` (`Parse`, `EncodeLine`, modify);
  - `internal/app/hook/answer/logic_test.go`, `in_test.go`, `out_test.go` (modify only if a tested name moves).
- Rules: GOARCH-1, GOARCH-2, GOARCH-3, GOARCH-4, GOARCH-5, GOARCH-7, GOARCH-8, GOARCH-9, GOARCH-10, GOARCH-11,
  GOARCH-12, GOARCH-13, GOARCH-14, GOARCH-16, GOARCH-17, GOARCH-18, GOARCH-19, GOARCH-20, GOARCH-21, GOARCH-22,
  GOARCH-23, GOARCH-24, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-29, GOARCH-30, GOARCH-32, GOARCH-33,
  GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, GOARCH-41, GOARCH-43, GO-2, GO-3, XPLAT-1.
- Reuse: `pass` with its step machinery (`ask`, `act`, `waiting`) and fact readers; `hook.Evaluation`, `hook.Facts`
  and `hook.Step`; `internal/platform/jsonl` for the log.
- Changes: the domain `hook` keeps only `Hook`, `Evaluation`, `Facts`, `Step`, `Outcome`, their methods and
  `RunBudget`.
  The free functions of `hook_condition.go`, `hook_protect.go` and `hook_command.go`, and those left in
  `hook_evaluation.go` (`Unusable` and the key helpers), go to the engine files of AnswerHook.
  There they become unexported functions or methods of `pass`, or methods of `Evaluation` and `Facts`.
  Their constants and tables move with them.
  `(Step).None` is rewritten within 15 cyclomatic.
  The decoding of the log (`state.Parse`) and its encoding (`state.EncodeLine`) move to the out files of AnswerHook,
  their only user, as unexported methods of `Adapter`.
  `Decide` builds the `pass` and routes the event to `session`, `edit`, `command` or `stop`, each in its
  `logic_<family>.go`.
  Each family reads top-down, calls the engine and holds the steps only its events take, as listed in `Decisions`.
  The engine files hold the rest of today's `logic.go` and the code moved from the domain, with no `if` on the name
  of an event.
  Every file has at most 400 lines and every function is within 15 and 20, `condition` split per condition.
  `run` keeps its single call of `Decide` in its loop and hands the carrying out of a `hook.Step` to unexported
  functions of the in files, whose conditions follow GOARCH-37.
  `answer.md` describes the router, the four families, the engine and the shell.
- Tests: the cases of `logic_test.go`, `in_test.go` and `out_test.go` stay and pass.
  The tests of the deleted `hook_*` files and of `state_line.go` move to the test files of the files that receive
  their code, with the same cases.
  A new case of `logic_test.go` checks that each event of `workflow.Events` reaches the family of `Decisions`.
- Verification: `golangci-lint run --enable-only gocyclo,gocognit,revive ./internal/app/hook/...`, expected: no
  finding.
  `go test ./internal/app/hook/... ./internal/app/state/ ./tests/architecture/`, expected: pass.

### Task 9: The domain workflow without free functions and under the limits

- Covers: AC1, AC5, AC9.
- Depends on: Task 7, Task 8.
- Files: `internal/app/workflow/*.go` (modify), new siblings `internal/app/workflow/workflow_<topic>.go` (create),
  their tests, and the use cases that receive a function: `internal/app/workflow/format/`,
  `internal/app/workflow/graph/`, `internal/app/rule/check/` and `internal/app/hook/answer/` (their logic files and
  tests, modify or create).
- Rules: GOARCH-1, GOARCH-2, GOARCH-3, GOARCH-4, GOARCH-7, GOARCH-8, GOARCH-9, GOARCH-10, GOARCH-13, GOARCH-14,
  GOARCH-16, GOARCH-17, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-29,
  GOARCH-30, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-38, GOARCH-39, GOARCH-41, GOARCH-43, GO-2, GO-3, XPLAT-1.
- Reuse: `validator`, `Cond` and the entities of `internal/app/workflow/`.
- Changes: the free functions left after Task 7 go where `Decisions` says.
  - `Nested`, `Measured` and `Strings` become methods of `Cond` or of the types they read.
  - `Format`, `Bases`, `DefaultBase`, `ShortcutForbiddenCommands` and `ShortcutProtectedFiles` become methods of
    `Workflow` or move to their only use case.
  - The unexported helpers become methods of `validator` or of the types they work on.

  Split the files over 400 lines by topic (`workflow_shortcuts.go`, `workflow.go`, `workflow_entry.go` and what is
  left of `workflow_parse.go`).
  Split `(*validator).cond` (31 and 63) per condition, keeping `Cond` as a map.
  Split `(*validator).workflow` (20 and 27) and `(*validator).action` (16) into methods.
  The problems, their text and their order do not change.
- Tests: the tests of `internal/app/workflow/...` pass with their cases, the validation of `examples/polix.yaml`
  included; the ones that called a free function call its new place.
- Verification: `golangci-lint run --enable-only gocyclo,gocognit,revive ./internal/app/workflow/...`,
  expected: no finding.
  `go test ./internal/app/... ./tests/architecture/`, expected: pass.

### Task 10: The domain setup and its use cases without free functions and under the limits

- Covers: AC1, AC5, AC9.
- Depends on: Task 7.
- Files: `internal/app/setup/*.go` (modify), new siblings `internal/app/setup/setup_<topic>.go` (create), their
  tests, the use cases of `internal/app/setup/` that receive a function (their in, logic and out files, documents
  and tests, modify or create), and `tests/setup/` (modify only if a moved name is used).
- Rules: GOARCH-1, GOARCH-2, GOARCH-3, GOARCH-4, GOARCH-5, GOARCH-7, GOARCH-8, GOARCH-9, GOARCH-10, GOARCH-11,
  GOARCH-12, GOARCH-13, GOARCH-14, GOARCH-15, GOARCH-16, GOARCH-17, GOARCH-18, GOARCH-19, GOARCH-20, GOARCH-21,
  GOARCH-22, GOARCH-23, GOARCH-24, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-29, GOARCH-30, GOARCH-32,
  GOARCH-33, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, GOARCH-41, GOARCH-43, GO-2, GO-3,
  XPLAT-1.
- Reuse: `HookGroup`, `HookHandler` and the domain types of `internal/app/setup/`; `internal/platform/file` and
  `internal/platform/proc` for what is technology.
- Changes: the free functions of the domain go where `Decisions` says.
  - The decoding and encoding of the settings files of Claude Code and Codex (`ParseHooks`, `mergeHooks`,
    `inlineTo`, `CodexPlugin`...) are translations.
    They move to the out files of the setup use cases that read or write them, as unexported methods of `Adapter`
    and decoding types.
    The port of DiagnoseSetup returns the hooks already decoded instead of bytes for its logic.
  - `ParseAgents` moves to the entries of InstallSetup and UninstallSetup, which validate each name with the
    domain.
  - The ones only DiagnoseSetup decides with (`RunProblems`, `IsPolixCommand`, `IsLegacyCommand`,
    `IsUpdateCommand`...) move to its logic files, or become domain methods when its shell asks them.
  - The same goes for InstallSetup and UninstallSetup.
  - The rest becomes methods of the domain types shared by several setup use cases.

  Split what goes past the limits:
  - `mergeHooks` (30 and 39), `inlineTo` (23) and `CodexPlugin` (19), in their new place;
  - the `hooks` of the logic of DiagnoseSetup (33 and 57), per check;
  - its shell `run` (18), into unexported functions of the in files.

  The settings written, the findings and the messages do not change.
- Tests: the tests of `internal/app/setup/...` and `tests/setup/` pass with their cases; the ones that called a
  free function call its new place.
- Verification: `golangci-lint run --enable-only gocyclo,gocognit,revive ./internal/app/setup/...`,
  expected: no finding.
  `go test ./internal/app/setup/... ./tests/...`, expected: pass.

### Task 11: The domains version, rule and state without free functions, UpdateVersion and CheckRule under the limits

- Covers: AC1, AC5, AC9.
- Depends on: Task 7, Task 8, Task 9, Task 10.
- Files: `internal/app/version/*.go`, `internal/app/rule/*.go`, `internal/app/state/*.go` and their tests (modify);
  `internal/app/version/update/` and `internal/app/rule/check/` (in, logic and out files, documents and tests,
  modify or create); the other use cases that call a moved function, DiagnoseSetup included (modify).
- Rules: GOARCH-1, GOARCH-2, GOARCH-3, GOARCH-4, GOARCH-5, GOARCH-7, GOARCH-8, GOARCH-9, GOARCH-10, GOARCH-11,
  GOARCH-12, GOARCH-13, GOARCH-14, GOARCH-15, GOARCH-16, GOARCH-17, GOARCH-18, GOARCH-19, GOARCH-20, GOARCH-21,
  GOARCH-22, GOARCH-23, GOARCH-24, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-29, GOARCH-30, GOARCH-32,
  GOARCH-33, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, GOARCH-41, GOARCH-43, GO-2, GO-3,
  XPLAT-1.
- Reuse: the entities `Version`, `Cache`, `Rule`, `Result`, `Run`, `State` and `Entry` and their methods;
  `internal/platform/semver`.
- Changes: the free functions left in the three domains go where `Decisions` says.
  - `version.Parse` of the text of a version is a translation: the entry or the out files that read a version use
    `internal/platform/semver` and build `Version`.
  - `version.Compare` becomes a method of `Version`.
  - `Released` and `RetryAfter` become methods of `Cache` or move to UpdateVersion.
  - The helpers of `rule` become methods of `Rule`, `Run` or `Result`.
  - What is left of `state` becomes methods of `State` and `Entry`.

  Split the shell `run` of UpdateVersion (20 and 25) and that of CheckRule (cognitive 21) into unexported functions
  of their in files, with `Decide` called once from `run` and every condition per GOARCH-37.
  Split `decision` of UpdateVersion (22) into logic functions.
  The messages, the failures and the exit codes do not change.
- Tests: the tests of the three domains and both use cases pass with their cases; the ones that called a free
  function call its new place.
- Verification: `golangci-lint run --enable-only gocyclo,gocognit,revive ./internal/...`, expected: no finding.
  `go test ./internal/... ./tests/...`, expected: pass.

### Task 12: The limits enabled, GO-4 and GOARCH-44 with their checks, and the docs

- Covers: AC1, AC2, AC4, AC5, AC10, AC11, AC12.
- Depends on: Task 1, Task 2, Task 3, Task 4, Task 5, Task 6, Task 7, Task 8, Task 9, Task 10, Task 11.
- Files:
  - `.golangci.yml` (`linters.enable`, modify);
  - `ai/rules/go.yaml` (GO-4, modify), `ai/rules/architecture.yaml` (GOARCH-44, modify);
  - `tests/architecture/depguard_test.go` (`depguard`, `mustLint`, `lintRun`, modify);
  - `tests/architecture/go04_test.go` and `tests/architecture/goarch44_test.go` (create);
  - `tests/architecture/rules_checks_test.go` (`TestRulesChecks`, modify);
  - `tests/architecture/testdata/limits/` (create: `go.mod`, a Go file over the limits, a test file over them);
  - the domain files of `tests/architecture/testdata/bad/` that `TestGOARCH44` needs (create or modify);
  - `docs/architecture.md` and `CONTRIBUTING.md` (modify).
- Rules: GO-2, GO-3, GOARCH-41, GOARCH-42, GOARCH-43, XPLAT-1.
- Reuse: `depguard`, `mustLint`, `linted`, `realPath`, `installLint` and `lintIssue` of
  `tests/architecture/depguard_test.go`; `checkDepguard` as the shape of `TestGO4`; `checkRule` and `module` for
  `TestGOARCH44`.
- Changes: add gocyclo and gocognit to `linters.enable`; revive is already enabled.
  Add GO-4 to `ai/rules/go.yaml`.
  It says a non-test function has a cyclomatic complexity of at most 15 and a cognitive complexity of at most 20,
  and a non-test Go file has at most 400 lines.
  Past that, the code splits into siblings along GOARCH-1, never a `//nolint` nor an exclusion.
  It is decided by gocyclo, gocognit and `file-length-limit` of revive.
  Its scope is `*.go` and `.golangci.yml`, `deterministic: true`, check `tests/architecture/go04_test.go`.
  Add GOARCH-44 to `ai/rules/architecture.yaml` with the text of `Domains` in `Decisions`, where each kind of free
  function goes included.
  Its scope is `internal/app/*/*.go`, `deterministic: true`, check `tests/architecture/goarch44_test.go`.
  Generalize the run of golangci-lint of `depguard_test.go` to take the linters to enable, with the cache keyed by
  root and linters.
  It keeps failing on a finding of a linter it did not ask for, and ignores the revive findings other than
  `file-length-limit`.
  `TestGO4` fails on any finding of the limits on the module.
  On `testdata/limits`, it finds a function above 15 cyclomatic, one above 20 cognitive and a file above 400 lines.
  It finds nothing on the test file there.
  `TestGOARCH44` checks the domain files of the module and expects the bad cases of AC4.
  `TestRulesChecks` reads every `*.yaml` of `ai/rules/` but `retired.yaml`.
  `docs/architecture.md` describes the limits, why the tests are out, how code past them splits, and the domain
  without free functions.
  `CONTRIBUTING.md` names GO-4 next to GOARCH-41 to GOARCH-43 among the rules whose tests run golangci-lint.
- Tests: `TestGO4` and `TestGOARCH44` as above; `TestRulesChecks` now guards GO-4; the depguard tests pass on the
  generalized runner.
- Verification: `just test -run 'TestGO4|TestGOARCH44|TestRulesChecks|TestGOARCH4[123]'`, expected: pass.
  `golangci-lint run ./...`, expected: no finding.
  `just check`, expected: pass.

## Risks

- A refactor this wide changes behaviour by accident.
  The existing tests keep their cases, a test changes only where the function it calls moves, and `just test`,
  `just check` and a real session gate the end.
- A lazy loader that caches per directory serves a stale value if a command writes the file and reads it again in
  the same process.
  No command does it today, and `load.md` documents it.
- The hook runs the loaders in the directory of the event: the adapter passes the root it opened, as today, so
  worktrees and subfolders keep working.
- Tasks 7 to 11 touch callers in packages other tasks also touch: their `Depends on` orders them.
  Tasks 3 to 5 run in parallel with Task 2.
  Then Task 6, Task 7, Tasks 8 and 10 in parallel, then Task 9, Task 11 and Task 12.
- A function moved into the only use case that uses it is needed by a second one later: it becomes a domain method,
  or a load use case when it is a translation.
- Splitting to please a counter scatters code: each part is named after a step of what it does, and the review
  checks that it reads better.
- `enable-default-rules` behaves differently in another golangci-lint: the version is pinned in CI and in
  `installLint`.
- The four families of AnswerHook drift apart: the engine holds everything that is not specific to an event, and the
  router test pins which events reach each family.

## Verification

- `just test`: every test passes, the architecture tests included.
- `golangci-lint run ./...`: no finding, with the three limits enabled.
- `just check`: install, `polix config check`, `polix config fmt --check`, `polix rules check` and
  `polix doctor --offline` pass.
- `claude plugin validate plugins/polix`: passes.
- End to end, after `just install`, in this repository:
  1. In a session of Claude Code, edit a Go file, end the turn and commit.
     The hooks answer as before: format, lint and tests on stop, the review gate on commit, the guidance on start.
  2. Break `polix.yaml` and run `polix config check`: the same problems as before; restore it.
  3. Run `polix graph`, `polix rules check` and `polix doctor --offline`: the same output as before the change.
  4. Add a free function to a domain file and run `just test -run TestGOARCH44`: it fails naming it; revert.
  5. Grow a function past 15 cyclomatic and run `golangci-lint run ./...`: gocyclo reports it; revert.
  6. Grow a non-test file past 400 lines: revive reports it; grow a `_test.go`: nothing is reported; revert.
  7. Make the entry `Command` of a use case call its port and run `just test -run TestGOARCH32`: it fails naming
     `Command`; revert.
