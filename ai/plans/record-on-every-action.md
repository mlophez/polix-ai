# Let every action of an entry of polix.yaml record a fact

## Overview

Today only a `run` can leave a fact behind when it passes, and a standalone `record` entry can record one.
A gate such as "ask for a review before the turn ends" cannot remember that it already asked, so it blocks on every
stop until the review ends or the loop guard gives up.

After this change any entry can carry `record` next to its action: deny, allow, require, run or context.
The fact is recorded when the entry applies its action, on the content it saw.
A `run` records only when its command passes, as today, but it is now written the same way as every other action.
The old place of `record` inside `run` disappears: there is one way to write it.

An entry in audit mode records nothing, so trying an entry never changes how the others behave.
In warn mode, and when the loop guard lets a stop through, the entry did apply, so it records.

A deny or require that records the same fact its own `unless` checks becomes "once per content".
That is the point for the motivating case, and the docs say it plainly; polix check does not warn about it.

## Context

The stop of this repository requires a review by polix:reviewer, which runs in the background.
Until it ends, every stop blocks again on the same content, and only the loop guard lets the turn end.
The outcome is a `record` that any entry can carry, so that the request can be made once per content.

## Decisions

- `record` is a key of the entry, combinable with any action; an entry with only `record` stays the record action.
- One form only: `record` inside `run` is removed, and polix check reports it with a hint to move it to the entry.
- No compatibility shim: polix has no published release yet, so only the files of this repository use the old form.
- The fact is recorded when the entry applies its action, on the content hash it saw.
- For `run`, applying means its command or its checks passed and the content did not change during the run.
- Audit mode records nothing, for every action, `run` included.
  This changes today's behaviour of a passing run in audit mode, which recorded.
- Warn mode records, since the entry applied with a warning.
- When the loop guard lets a stop or subagent_stop through instead of blocking, the entry records its fact.
- A deny or require that records the fact of its own `unless` is only documented, never warned nor rejected.
- A `deny` and an `allow` that stop the evaluation record before it stops.
- Assumption: a `context` records whenever the entry applies, even when every rule it would inject was already
  injected in the session.
- Placeholders are allowed in `record`, as in the record action today.

## Approach

The schema in `internal/workflow` keeps `Entry.Record` as the one place of a fact.
`Run` loses its `Record` field and `runYAML` its key `record`, so a run no longer carries a fact.
The validator stops counting `record` as an action when the entry has another action.
`Entry.Action` returns that other action, so the engine, the graph and the log keep one action name per entry.
An entry whose only key is `record` keeps `record` as its action, as today.
An empty `record` is a problem for every entry that has one, with its placeholders checked.
A `record` key inside `run` is reported as a problem that says to write `record` next to `run` instead.

The engine in `internal/hook/answer_hook_logic.go` records after applying the action, in one place.
`answerHookRun.apply` decides, per action, whether the action applied.
Deny, allow, require and context applied when the entry matched and the mode is not audit.
A run applied when it passed on stable content, which `answerHookRun.runEntry` checks with the hash before and after.
A blocked deny or require, a warned one and one let through by the loop guard all count as applied.
`answerHookRun.block` already switches on the mode; it reports to `apply` whether the entry applied.
`answerHookRun.recordFact` stays the only writer of facts, so an unknown hash still records nothing and warns.
The step shape (`answerHookRun.waiting`) is respected: nothing is recorded while a step is pending.

`polix graph` writes ` → record <fact>` after any action that carries one, not only after `run`.

The contract that changes is the schema of `polix.yaml`: `record` moves from `run` to the entry.
Error handling: every misuse is a problem of polix check with the line of the entry.

### Discarded alternatives

- `record` nested in each action (`require: { text, record }`): it changes the shape of every action and is longer.
- Keeping `run: { record }` as an alias: two ways to write the same thing, rejected by the user.
- A warning of polix check for a gate that records its own `unless`: it needs a new channel of warnings for one case.

## Acceptance criteria

- AC1: An entry with `require` and `record` on stop is valid, blocks once and records the fact on the current content.
- AC2: On the next stop with the same content and `unless: { done: <that fact> }`, the entry is skipped.
- AC3: `deny` with `record` records the fact when it blocks, also in warn mode, and in enforce stops the evaluation.
- AC4: `allow` and `context` with `record` record the fact when they apply.
- AC5: `run` with `record` at the entry records only when the run passes on unchanged content, as before.
- AC6: An entry in audit mode with `record` records nothing, for every action.
- AC7: A stop let through by the loop guard records the fact of the entry.
- AC8: polix check reports `record` inside `run` with a hint to move it next to `run`, and an empty `record`.
- AC9: An entry with only `record` behaves as the record action of today.
- AC10: `polix graph` shows the fact of any entry that records one.
- AC11: The polix.yaml of this repository, examples/polix.yaml, the reference of the skill workflow, README.md and
  docs/use-cases.md use the new form, and docs/polix.md documents it.

## Out of scope

- A warning of polix check for a deny or require that records the fact of its own `unless`.
- Any change to `done`, to the content hash or to the loop guard itself.

## Tasks

### Task 1: Move `record` to the entry in the schema, the run of the engine and the files of this repository

- Covers: AC5, AC8, AC9.
- Files: `internal/workflow/workflow_entry.go` (`Entry.actions`, `Entry.Action`, `Run`, `validator.entry`,
  `validator.action`, `validator.run`, modify), `internal/workflow/workflow_parse.go` (`runYAML`,
  `runYAML.run`, `entryYAML.entry`, modify), `internal/workflow/workflow.go` (comment of `Actions`, modify),
  `internal/hook/answer_hook_logic.go` (`answerHookRun.runEntry`, modify), `polix.yaml` (modify),
  `examples/polix.yaml` (modify), `plugins/polix/skills/workflow/reference.yaml` (modify),
  `internal/workflow/workflow_entry_test.go`, `internal/workflow/workflow_parse_test.go`,
  `internal/workflow/workflow_test.go`, `internal/hook/answer_hook_logic_test.go`,
  `internal/hook/answer_hook_out_test.go` (modify).
- Rules: GOARCH-4, GOARCH-9, GOARCH-13, GOARCH-16, GOARCH-18, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28,
  GOARCH-30, GOARCH-35, GOARCH-38, GO-2, GO-3, XPLAT-1, DOC-1.
- Reuse: `validator.unknownKeys`, `validator.placeholders` and `contains` from `internal/workflow/workflow_entry.go`.
- Changes: `Run` drops its field `Record` and `runYAML` drops the key, so `record` inside `run` is an unknown key of
  run; `validator.run` reports it with a message that says to write `record` next to `run` in the entry.
  `Entry.actions` leaves `record` out when the entry has another action, so `Entry.Action` returns that action and
  the problem "several actions" no longer fires for an action plus `record`.
  `validator.action` checks an empty `record` and its placeholders for every entry that has one.
  `answerHookRun.runEntry` reads the fact from `Entry.Record` instead of `Run.Record`, with the same hash check.
  The polix.yaml of this repository, examples/polix.yaml and its copy in the plugin move every `record` of a run
  next to its `run`, and their comments on run and record describe the new form.
- Tests: in `workflow_entry_test.go`, an entry with an action and `record` is valid and its `Action` is that action;
  an entry with only `record` has the action `record`; an empty `record` with any action is a problem.
  In `workflow_test.go`, `record` inside `run` is a problem that names the fix, and an unknown placeholder in the
  `record` of an entry with another action is reported.
  In `workflow_parse_test.go`, a run with `record` at the entry fills `Entry.Record` and the line of the entry.
  The tests of the engine that write `run: { ..., record }` move to the new form and keep their expectations.
- Verification: `just test`, expected: every package passes, `plugins` included (the reference matches the example).

### Task 2: Record the fact after any action that applied, by mode and loop guard

- Covers: AC1, AC2, AC3, AC4, AC6, AC7.
- Files: `internal/hook/answer_hook_logic.go` (`answerHookRun.apply`, `answerHookRun.block`,
  `answerHookRun.runEntry`, modify), `internal/hook/answer_hook_logic_test.go` (modify),
  `internal/hook/answer_hook.md` (modify).
- Rules: GOARCH-4, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-30, GOARCH-31, GOARCH-34, GOARCH-35, GO-2, GO-3, DOC-1.
- Reuse: `answerHookRun.recordFact`, `answerHookRun.expand`, `answerHookRun.hash` and `Entry.EffectiveMode`.
- Changes: `answerHookRun.apply` records the fact of the entry, through `recordFact`, after the action applied and
  before it returns, also when the action stops the evaluation (a deny that blocks, an allow).
  It records nothing in audit mode, nor while a step is pending.
  `answerHookRun.block` tells `apply` whether the entry applied: blocked, warned or let through by the loop guard
  apply; audit does not.
  `answerHookRun.runEntry` records only after a passing run on unchanged content, and never in audit mode.
  `answer_hook.md` says in its Logic and Flow sections when the fact of an entry is recorded.
- Tests: in `answer_hook_logic_test.go`, a stop require with `record` blocks once and records, and a second stop on
  the same content with `unless: { done }` passes; a deny with `record` records in enforce and in warn mode, and in
  enforce stops the evaluation; allow and context with `record` record; audit mode records nothing for a require nor
  for a passing run; the loop guard after `max_stop_blocks` records; an unknown hash records nothing and warns.
- Verification: `just test -run TestAnswerHook`, expected: pass.

### Task 3: Show the fact of any action in polix graph

- Covers: AC10.
- Files: `internal/workflow/graph_workflow_logic.go` (the function that writes the label of an entry, modify),
  `internal/workflow/graph_workflow_logic_test.go` (modify), `internal/workflow/graph_workflow.md` (modify).
- Rules: GOARCH-4, GOARCH-21, GOARCH-23, GOARCH-31, GOARCH-34, GOARCH-35, GO-2, GO-3, DOC-1.
- Reuse: the label builder of `internal/workflow/graph_workflow_logic.go`.
- Changes: the label of an entry appends ` → record <fact>` after its action whenever `Entry.Record` is set and the
  action is not `record` itself, replacing the case that did it only for `run`.
  `graph_workflow.md` says that any action shows the fact it records.
- Tests: in `graph_workflow_logic_test.go`, a require with `record` and a run with `record` show the arrow and the
  fact; a lone `record` keeps its current label.
- Verification: `just test -run TestGraphWorkflow`, expected: pass.

### Task 4: Document `record` on every action

- Covers: AC11.
- Files: `docs/polix.md` (actions, modes and the example of the stop of this repository, modify),
  `docs/use-cases.md` (modify), `README.md` (modify).
- Rules: GO-2.
- Reuse: none.
- Changes: `docs/polix.md` describes `record` as a key of the entry valid with every action, when each action
  records, that audit records nothing while warn and the loop guard do, and that a deny or require that records
  the fact of its own `unless` asks once per content instead of enforcing.
  It removes `run: { ..., record }` and the sentence that a passing run records in every mode.
  Its example of the stop of this repository names the review entry and the subagent_stop.
  `docs/use-cases.md` and `README.md` move every `record` of a run next to its `run`, and the review recipe of
  use-cases shows a require that records the request once per content.
- Tests: none; the docs carry no tests.
- Verification: `rg -n 'record' docs README.md examples plugins/polix/skills/workflow/reference.yaml`, expected:
  no `record` inside a `run` mapping.

## Risks

- The installed polix rejects the new form, and an invalid polix.yaml blocks the edits and commands of the agent:
  change the code first, run `just install`, and only then migrate polix.yaml, which the main session may edit.
- The old form in a polix.yaml of another repository fails polix check after the update: the problem names the fix,
  and no release has shipped the old form.
- A gate that records its own `unless` silently stops enforcing: the docs say it, and the review recipe shows it.
- Audit mode stops recording for a passing run: an entry relied on that only while it was being tried; the docs
  say it.

## Verification

- `just test`: every package passes.
- `golangci-lint run ./...`: 0 issues.
- `just check`: install, `polix check`, `polix rules check` and `polix doctor --offline` pass.
- `claude plugin validate plugins/polix`: valid.
- End to end, in this repository after `just install`:
  1. Add `record: review_asked` to the review require of stop, with `not: { done: review_asked }` in its `if`.
  2. Change a file and end a turn: the stop blocks once with the request, and `polix log` shows the fact.
  3. End the turn again on the same content: the stop does not block.
  4. Let polix:reviewer end: `polix log` shows the fact `reviewed` recorded by subagent_stop.
