# Check the programs a repository requires and tell the AI when one is missing

## Overview

A repository governed by polix often depends on programs its workflow runs: a linter, a test runner, a formatter.
When one of them is not installed, the agent finds out late, when a check fails at the end of its turn.

This change adds a new top-level key to `polix.yaml`, `requirements`, a plain list of program names.
polix looks each of them up on the PATH, the same way the hooks would run them, and:

- on the first prompt of a session, tells the AI which programs are missing and asks it to install them, or to ask
  the user to, before doing anything else;
- in `polix doctor`, shows a warning for each missing program.

It never blocks the agent: it only informs.
The key lives at the root of the file, right before `guardrails`, because it is not a shortcut that expands into
entries of `on`.

## Context

`polix.yaml` can run any program through `guardrails` and `on`, but nothing checks that those programs exist on the
machine of the session, so a missing one shows up as a failing check late in the turn.
The outcome is a declarative list of required programs that polix checks on the first prompt of each session and in
`polix doctor`, informing the AI and the user without blocking.

## Decisions

- The key is `requirements`, at the root of `polix.yaml`, not inside `guardrails`: `guardrails` holds shortcuts and
  protections of the hook, and this is neither.
- `polix config fmt` places `requirements` right before `guardrails`.
- The form is a list of program names (`requirements: [go, golangci-lint]`), with no reason, version nor block per
  item; a single string or a map is reported with the expected form, as the shortcuts of `guardrails` do.
- polix checks only the declared programs: it never derives them from the commands of the workflow nor checks
  versions.
- Missing programs never block anything: the hook informs the AI and `polix doctor` warns.
- The hook informs on the `prompt` event, never on `start`: a once-only context, due again after a `start` of the
  session (new session, resume, clear or compaction), as the once-only injects of `on` are.
- The key of that once-only context includes the missing programs, so it is due again when the set of missing
  programs changes within the session.
- `polix doctor` gives one `warn` finding per missing program, so `polix doctor --hook` at session start also tells
  the agent about them; it never makes `polix doctor` fail.
- The lookup of the hook goes through a new method of the port of AnswerHook, which then has 20 methods, one more than
  the `interfacebloat` maximum of 19.
  As GO-3 asks, the finding is silenced on the line of the port with a `//nolint:interfacebloat` and its reason; the
  linter and its maximum stay as they are.
- The messages polix writes are in English (GO-2).
- Assumption: this repository adopts the key with `requirements: [go, golangci-lint]`, the programs its
  `run_before_done` and `format_on_edit` run.
- Assumption: `polix graph` does not draw `requirements`, since it draws only what the hook enforces.

## Approach

Workflow (domain, `internal/app/workflow`): `Workflow` gains `Requirements`, the program names in file order.
The validator of the settings reports an empty name as `requirements: a program name cannot be empty`.
`TopKeys` gains `requirements`, so an older polix reports it as unknown and asks to update when `requires` is set.
`Workflow.InjectsOnce` also holds when `Requirements` is not empty, so the hook writes the `started` line on `start`
and the context of the requirements is due again after it.

Loading (`internal/app/workflow/load`): a new unexported decoding type in `out_requirements.go` reads the key.
It accepts only a sequence of scalars; any other form is an error of the file at the line of the node, with the
expected form, such as `line 3: requirements takes a list of program names, as [go, golangci-lint]`.
The decoding of the workflow copies the names into `Workflow.Requirements`.

Formatting (`internal/app/workflow/format`): the order of the top-level keys puts `requirements` right before
`guardrails`.

Hook domain (`internal/app/hook`): `Step` gains `Programs`, which asks which of these programs are not on the PATH.
`Facts` gain `Missing` and `HasMissing`, and `Evaluation` gains `KnowMissing`, as for the other facts.
The fact is read once per call and kept for the next events of the call.

AnswerHook (`internal/app/hook/answer`): `Port` gains `Missing`, which takes program names and returns those the PATH
does not run, in the same order.
`Adapter` gains a lookup function field, set to `exec.LookPath` by the entry, as the adapter of DiagnoseSetup does.
The shell carries out the new step through `Missing` and feeds it back with `KnowMissing`.
The family `session`, on `prompt` only and before the entries, asks for the missing programs when `Requirements` is
not empty.
When some are missing, it adds a once-only context through the same `due` and `injected` mechanism as the once-only
injects, at line 0, keyed by `requirements:` followed by the missing names.
The text is `These programs that polix.yaml requires are not on the PATH: <names>. Before doing anything else, install
them, or ask the user to install them if you cannot.`
A refused prompt discards it as it discards the other once-only injects, and the answer never blocks because of it.

DiagnoseSetup (`internal/app/setup/diagnose`): `setup.Inspection` gains `Requirements`, by program, why the PATH does
not run it.
The shell fills it inside a repository with the existing port method `LookPath`, once per declared program.
The logic adds the area `requirements` right after the workflow: one `warn` per missing program, `<program> is not on
the PATH: polix.yaml requires it`, or one `ok` line `<n> programs of requirements on the PATH` when all are found;
nothing when the workflow declares none.

Error handling: a lookup that fails means a missing program, never an error of the shell; an invalid workflow keeps its
current behaviour (the hook warns that it is unusable and the doctor reports its problems), with no requirements.

### Discarded alternatives

- Inside `guardrails`: `guardrails` holds shortcuts that expand into `on` and protections of the hook; requirements
  are neither.
- Blocking edits, commands or the prompt while a program is missing: the user wants polix only to inform.
- Items with `reason`, `block` or `version`: more than the user needs now; a list of names is enough.
- Deriving the programs from the commands of the workflow: false positives with shell builtins and relative paths.
- Raising `interfacebloat` or disabling it: GO-3 forbids disabling a linter for a finding, and the `//nolint` keeps the
  warning visible in the code.

## Acceptance criteria

- AC1: `polix config check` accepts `requirements: [go, golangci-lint]`, and reports a single string, a map, an item
  that is not a scalar and an empty name, the form errors at their line with the expected form.
- AC2: `polix config fmt` moves `requirements` right before `guardrails`, keeping the order of its items.
- AC3: On a `prompt` with a declared program not on the PATH, `polix hook claude` exits 0 and injects a context that
  names the missing programs and asks the AI to install them, or ask the user to, before anything else.
- AC4: A later `prompt` of the same session does not inject it again; after a `start` of the session, or when the set
  of missing programs changes, the next `prompt` injects it again; a refused prompt does not record it as injected.
- AC5: Without `requirements`, the hook asks for no lookup; with every program on the PATH, it injects nothing.
- AC6: A `start` event never injects the context of the requirements.
- AC7: `polix doctor` shows one `warn` finding of the area `requirements` per missing program and an `ok` line when
  all are found, and does not fail because of them; `polix doctor --hook` lists the missing ones.
- AC8: `golangci-lint run ./...` passes with `interfacebloat` still enabled at a maximum of 19.
- AC9: `docs/polix.md`, `docs/reference.md`, `examples/polix.yaml`, its copy in the skill `workflow` and
  `CHANGELOG.md` describe the key, and the `polix.yaml` of this repository declares `go` and `golangci-lint`.
- AC10: On Windows a name without extension is found through `PATHEXT` (`go` finds `go.exe`) by the hook and by
  `polix doctor`, checked by tests with the real lookup in the CI matrix, and the docs say how the lookup works on
  each system.

## Out of scope

- Versions of the programs, install hints per program and blocking modes: not requested.
- Requirements other than programs (environment variables, files): not requested.
- Drawing `requirements` in `polix graph`.

## Tasks

### Task 1: Read, validate and format the key requirements

- Covers: AC1, AC2.
- Depends on: none.
- Files: `internal/app/workflow/workflow.go` (`Workflow`, `validator.settings`, `Workflow.InjectsOnce`, modify),
  `internal/app/workflow/workflow_load.go` (`TopKeys`, modify), `internal/app/workflow/load/out_requirements.go`
  (create), `internal/app/workflow/load/out_workflow.go` (`workflowYAML`, `workflowYAML.workflow`, modify),
  `internal/app/workflow/load/load.md` (modify), `internal/app/workflow/format/logic_order.go` (`topKeys`, modify),
  `internal/app/workflow/format/format.md` (the list of the top-level keys in order, modify),
  `internal/app/workflow/workflow_test.go`, `internal/app/workflow/load/out_requirements_test.go` (create),
  `internal/app/workflow/load/out_workflow_test.go`, `internal/app/workflow/format/logic_order_test.go`.
- Rules: GOARCH-1, GOARCH-3, GOARCH-4, GOARCH-13, GOARCH-16, GOARCH-21, GOARCH-24, GOARCH-25, GOARCH-30, GOARCH-35,
  GOARCH-38, GOARCH-41, GOARCH-44, GO-2, GO-4, DOC-1.
- Reuse: `validator.add` from `internal/app/workflow/workflow.go`; the form checks of `nodeYAML` in
  `internal/app/workflow/load/out_guardrails.go` as the model of the error of another form.
- Changes: `Workflow` gains the field `Requirements`, documented as the programs the repository requires on the PATH,
  checked by the hook on the first prompt and by `polix doctor`.
  `validator.settings` reports an empty or blank name as `requirements: a program name cannot be empty`.
  `Workflow.InjectsOnce` also returns true when `Requirements` is not empty, and its comment says why.
  `TopKeys` gains `requirements` after `max_stop_retries`.
  `out_requirements.go` declares the unexported decoding type of the key, with `UnmarshalYAML`: it accepts a sequence
  whose items are all scalars and keeps their values in order; a scalar, a map or an item that is not a scalar is an
  error at the line of its node, `requirements takes a list of program names, as [go, golangci-lint]`.
  `workflowYAML` gains the field `Requirements` of that type with the tag `requirements`, and `workflow` copies it.
  `topKeys` of the format puts `requirements` right before `guardrails`, and its comment says so.
  `load.md` describes the key, and `format.md` lists `requirements` right before `guardrails` in the order of the
  top-level keys.
- Tests: in `workflow_test.go`, an empty name is a problem and `InjectsOnce` holds with requirements and without
  injects; in `out_requirements_test.go`, a list is decoded in order, and a scalar, a map and a nested list each give
  the error at the right line; in `out_workflow_test.go`, a file with the key loads with `Requirements` filled; in
  `logic_order_test.go`, a file with `requirements` after `on` is formatted with it right before `guardrails`.
- Verification: `just test -run 'Requirements|InjectsOnce|Order'`, expected: the new and existing tests pass.

### Task 2: Add the step and the fact of the missing programs to the hook domain

- Covers: none.
- Depends on: none.
- Files: `internal/app/hook/hook_step.go` (`Step`, `Step.Valid`, `Step.asks`, modify),
  `internal/app/hook/hook_facts.go` (`Facts`, modify), `internal/app/hook/hook_evaluation.go`
  (`Evaluation.KnowMissing`, create in the file), `internal/app/hook/hook_step_test.go`,
  `internal/app/hook/hook_evaluation_test.go`.
- Rules: GOARCH-13, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-30, GOARCH-35, GOARCH-38, GOARCH-41,
  GOARCH-44, GO-2, GO-4.
- Reuse: `Evaluation.KnowForeign` and `Evaluation.KnowBranch` from `internal/app/hook/hook_evaluation.go` as the model.
- Changes: `Step` gains `Programs`, commented as which of these programs are not on the PATH; `asks` counts it when it
  is not nil, and `Valid` rejects a non-nil empty list.
  `Facts` gain `Missing`, the programs the PATH does not run, and `HasMissing`, whether they were looked up.
  `Evaluation.KnowMissing` returns a copy of the evaluation whose facts hold the missing programs, copied, with
  `HasMissing` set.
- Tests: in `hook_step_test.go`, a step with programs asks one thing, a step with programs and another field is not
  valid, and an empty non-nil list is not valid; in `hook_evaluation_test.go`, `KnowMissing` sets both facts and does
  not share the slice with the caller.
- Verification: `just test -run 'Step|KnowMissing'`, expected: pass.

### Task 3: Inform the AI of the missing programs on the first prompt

- Covers: AC3, AC4, AC5, AC6, AC8, AC10.
- Depends on: Task 1, Task 2.
- Files: `internal/app/hook/answer/in.go` (`Port`, `carry`, `Command`, modify), `internal/app/hook/answer/out.go`
  (`Adapter`, `Adapter.Missing`, modify), `internal/app/hook/answer/logic_facts.go` (new accessor, modify),
  `internal/app/hook/answer/logic_session.go` (`pass.session`, new method for the requirements, modify),
  `internal/app/hook/answer/answer.md` (modify), `.golangci.yml` (comment of `interfacebloat`, modify),
  `docs/architecture.md` (the line on `interfacebloat`, modify), `internal/app/hook/answer/logic_test.go`,
  `internal/app/hook/answer/in_test.go`, `internal/app/hook/answer/out_test.go`.
- Rules: GOARCH-3, GOARCH-4, GOARCH-5, GOARCH-12, GOARCH-18, GOARCH-20, GOARCH-21, GOARCH-24, GOARCH-32, GOARCH-33,
  GOARCH-34, GOARCH-36, GOARCH-37, GO-2, GO-3, GO-4, DOC-1, XPLAT-1.
- Reuse: `proc.LookFunc` from `internal/platform/proc/proc.go`; `pass.due`, `pass.injected` and `pass.writeInjected`
  from `internal/app/hook/answer/logic_inject.go`; `pass.ask` and the accessors of `logic_facts.go`.
- Changes: `Port` gains `Missing(ctx, programs)`, which returns the programs the PATH does not run, in their order;
  the declaration of `Port` carries `//nolint:interfacebloat // <reason>`, the reason being that the hook reads every
  fact through one port and the step shape needs one method per fact.
  `carry` carries out a step with `Programs` through `Missing` and `KnowMissing`.
  `Adapter` gains a lookup field of type `proc.LookFunc`, which `Command` sets to `exec.LookPath`; `Adapter.Missing`
  returns each program whose lookup fails.
  `logic_facts.go` gains an accessor that returns the missing programs from the facts or asks the `Programs` step.
  `pass.session`, on `prompt` only and before the entries, calls a new method that, when `Requirements` is not empty,
  gets the missing programs and, when there are some and the context is due for the session at line 0 with the key
  `requirements:` and the missing names, adds the context of the Approach to the answer and keeps it in
  `pass.injected`; a skip is logged as the skips of the once-only injects are.
  `answer.md` names `Missing` in `## Port` and describes the context in the family `session`.
  The comment of `interfacebloat` in `.golangci.yml` and the line of `docs/architecture.md` say the port of AnswerHook
  passes the maximum with its `//nolint` and that no other interface may.
- Tests: in `logic_test.go`, a prompt with a missing program injects the context and asks one `Programs` step; a
  second prompt of the session does not inject it; a prompt after a `start` does; a prompt with another set of
  missing programs does; a refused prompt does not write it as injected; a `start` never asks nor injects it; a
  workflow without requirements asks no `Programs` step; every program found injects nothing; in `in_test.go`, the fake
  port answers `Missing` and the shell carries the step; in `out_test.go`, `Adapter.Missing` with a fake lookup
  returns only the programs it fails on, in order, and `Adapter.Missing` with the real `exec.LookPath` finds `go`
  written without extension and reports a made-up name as missing, so the CI matrix checks the lookup through
  `PATHEXT` on Windows.
- Verification: `just test -run 'Prompt|Requirements|Missing'` and `golangci-lint run ./...`, expected: pass, with no
  `interfacebloat` finding.

### Task 4: Warn about the missing programs in polix doctor

- Covers: AC7, AC10.
- Depends on: Task 1.
- Files: `internal/app/setup/setup_finding.go` (`Inspection`, modify), `internal/app/setup/diagnose/in.go`
  (`run`, `inspectRepository`, `repository`, modify), `internal/app/setup/diagnose/logic.go` (`Decide`, new function
  for the requirements, modify), `internal/app/setup/diagnose/diagnose.md` (modify),
  `internal/app/setup/diagnose/logic_test.go`, `internal/app/setup/diagnose/in_test.go`,
  `internal/app/setup/diagnose/out_test.go`.
- Rules: GOARCH-4, GOARCH-18, GOARCH-20, GOARCH-21, GOARCH-25, GOARCH-30, GOARCH-32, GOARCH-34, GOARCH-37,
  GOARCH-41, GOARCH-44, GO-2, GO-4, DOC-1, XPLAT-1.
- Reuse: the port method `LookPath` and the function `found` of `internal/app/setup/diagnose`.
- Changes: `Inspection` gains `Requirements`, by program, why the PATH does not run it (nil when it does).
  `inspectRepository` looks up each program of the loaded workflow with `LookPath` and keeps the result in
  `repository`, which `run` passes to the inspection.
  `Decide` adds, right after `workflowFile`, the findings of the area `requirements`: one `warn` per missing program in
  the order of the file, `<program> is not on the PATH: polix.yaml requires it`, or one `ok`, `<n> programs of
  requirements on the PATH`, when all are found; none without requirements.
  `diagnose.md` describes the area in `## Logic` and the lookup in `## Flow`.
- Tests: in `logic_test.go`, a missing program gives a `warn` and does not set `Failed`, all found give the `ok` line,
  no requirements give no finding of the area; in `in_test.go`, the fake port is asked `LookPath` for each declared
  program and `--hook` prints the missing ones; in `out_test.go`, `Adapter.LookPath` with the real `exec.LookPath`
  finds `go` written without extension on every system of the CI matrix.
- Verification: `just test -run 'Requirements|Decide'`, expected: pass.

### Task 5: Document the key and adopt it in this repository

- Covers: AC9, AC10.
- Depends on: Task 1, Task 2, Task 3, Task 4.
- Files: `docs/polix.md` (modify), `docs/reference.md` (modify), `examples/polix.yaml` (modify),
  `plugins/polix/skills/workflow/reference.yaml` (modify), `README.md` (modify when it lists the keys of polix.yaml),
  `CHANGELOG.md` (modify), `polix.yaml` (modify).
- Rules: GO-2, XPLAT-1.
- Reuse: none.
- Changes: `docs/polix.md` adds `requirements` to the example of `polix.yaml` and a section that says what it checks,
  when the hook informs (first prompt, again after a start or when the missing set changes, never blocking), what
  `polix doctor` warns, and that a file using it sets `requires`.
  `docs/reference.md` adds it to the file at a glance and a `### requirements` under the top-level keys, with its
  type, its form errors and an example.
  Both say how a program is looked up: on the PATH the hooks give polix, as polix itself runs a command of `run`; on
  Windows through the extensions of `PATHEXT`, so names are written without extension (`golangci-lint`, never
  `golangci-lint.exe`, which is missing on macOS and Linux); a shell alias or function, a script without extension
  that only Git Bash runs, and a program found only in the current folder count as missing.
  `examples/polix.yaml` gains an annotated `requirements`, and `reference.yaml` of the skill `workflow` stays a byte
  copy of it.
  `CHANGELOG.md` gains an entry under `[Unreleased]` `Added`.
  The `polix.yaml` of this repository gains `requirements: [go, golangci-lint]`, written after running `just install`
  so the hooks know the key, then formatted with `polix config fmt`.
- Tests: the existing test that keeps `reference.yaml` equal to `examples/polix.yaml` and the test that validates
  `examples/polix.yaml` cover it.
- Verification: `just check`, expected: config, format, rules and doctor pass, the doctor with an `ok` line of
  `requirements`.

## Risks

- The hooks of this repository run the installed polix: editing `polix.yaml` before `just install` makes it invalid
  for the old binary and blocks edits and commands: Task 5 runs `just install` first.
- A program installed outside the PATH of the hooks (a shell alias, a function of the profile) counts as missing:
  the docs say the lookup is that of the hooks, not of the interactive shell.
- On Windows a script without extension that Git Bash runs, or a name written with `.exe`, behaves differently from
  macOS and Linux: the docs ask for names without extension, and a test with the real lookup runs in the CI matrix.
- At the startup of a session the doctor and the first prompt both mention a missing program: accepted, the doctor
  speaks of the machine and the prompt asks to install before working.
- The lookup adds a PATH search to each call that evaluates a prompt with requirements: only on `prompt`, once per
  call, and only when the key is set.

## Verification

- `just test`: every unit, architecture and end to end test passes.
- `golangci-lint run ./...`: no finding.
- `just check`: config, format, rules and doctor pass on this repository.
- `claude plugin validate plugins/polix`: the plugin is valid.
- End to end: in a scratch repository with `requirements: [go, does-not-exist]`, run `polix doctor` and see one `warn`
  for `does-not-exist`; pipe a `UserPromptSubmit` input of Claude Code to `polix hook claude` and see the context
  naming `does-not-exist` with exit 0; pipe it again with the same session and see no context; pipe a `SessionStart`
  and then the prompt again and see it once more.
