# Add the verb notify, the event subagent_start and the status message of the hooks, valid for both agents

## Overview

Polix learns three things that Claude Code and Codex both support in the same way.

- While a hook of polix runs, both agents show "polix" next to their spinner, so the user knows who the wait is on.
- A new event, `subagent_start`, lets a workflow give context to a subagent the moment it is born.
  Today a subagent that only reads, such as a reviewer, may never get the rules, since it never edits nor runs a
  command.
- A new verb, `notify`, tells the user something without spending the context of the agent.
  The messages polix already has for the user, such as the loop guard letting the agent stop, take the same way.
  Today they go to stderr, where neither agent reliably shows them.

`polix.yaml` stays independent of the agent: a combination is allowed only when it works in both, and none slips
through.
`polix config check` rejects a verb on an event where it cannot act, an entry that can never run, and a notice that a
later
gate of the same event could swallow.
The hook, in turn, only ever writes to each agent the fields that agent accepts on that event, and a table of tests
proves it.

## Context

Claude Code and Codex accept more than the two answers polix gives today, exit 2 to block and `additionalContext` to
inject.
Codex parses the JSON of a hook strictly: an unknown field, or `hookSpecificOutput` on an event that does not take it,
fails the hook and drops the whole answer, which lets the action through.
The outcome is three features from the intersection of both agents, with a validation that keeps `polix.yaml` neutral.

## Decisions

- Only what both agents support is built: `statusMessage`, `SubagentStart` with `additionalContext`, and
  `systemMessage`.
  `ask`, `continue: false`, `updatedInput`, `additionalContext` on a stop, `onFailure` and `PostToolBatch` are left out.
- `polix.yaml` is neutral: the domain holds one table of the verbs each event allows, the intersection of both agents.
  `polix config check` reports as a problem every combination outside it; there are no warnings for dubious
  combinations.
- The text of `statusMessage` is the fixed `polix`, the same on every handler polix registers, the update one included.
- Every message for the user, the internal warnings and the notices of `notify`, goes as `systemMessage` on exit 0.
  They no longer go to stderr, whose destination depends on the agent and the event.
- A blocked answer is exit 2, whose stdout neither agent reads: its messages for the user are not shown.
  They are written to the log as a `skip` decision that says so; check makes it impossible for a notice of `notify`.
- A notice of `notify` is once per session and expanded text, whatever the actor, and never due again in that session.
  A compaction or a subagent does not show the user the same notice twice; `always: true` shows it every time.
- On `subagent_start` every inject acts every time it applies, as on `start`: the subagent starts with a new context.
- `subagent_start` counts for the once-only injects of its subagent type only.
  It is written as an observed `started` line with that type as the actor.
  A `started` line of the main session still resets every actor.
- A missing `SubagentStart` hook in settings written by an older polix is a warning of `polix doctor`, as
  `PostToolUseFailure` is.
  Only the context at the start of subagents is lost, and `polix install` adds it.
- An entry is unreachable when an earlier entry of its event is a `block` without `when`, `unless` or `once`.
  `stop` and `subagent_stop` are excluded, since the loop guard lets later entries run after `max_stop_retries`.
- An entry may block when it is a `block`, or a `run` on an event outside `workflow.Informed`.
  A `notify` followed in its event by such an entry is a problem: the gate would swallow the notice whenever it acts.
- The release is still 0.3.0, unreleased, so `requires: 0.3.0` of `examples/polix.yaml` already covers the new keys.

## Approach

The domain `internal/app/workflow` gets the new event `subagent_start` and the new verb `notify`.
The events, verbs and conditions, with a new table of the verbs each event allows, move from `workflow.go` to a sibling
`workflow_vocabulary.go`, which keeps `workflow.go` under the 400 lines of GO-4.
The table of verbs replaces the checks of each verb against its event in `workflow_entry.go`.
A new pass over the entries of each event in `Workflow.Problems` reports the two problems that depend on the order of
the
entries: an unreachable entry and a notice before an entry that may block.
Each problem names the lines of both entries, and the line of the shortcut when an entry was expanded from one.

The verb `notify` has a short form, its message, and a long form with `message`, required, and `always`.
Placeholders are allowed in the message.
It is decoded in `internal/app/workflow/load`, ordered by `polix config fmt` and drawn by `polix workflow graph`.

`agent.Answer` keeps its field `Warnings`, its messages for the user, which `notify` also fills.
The shared render of both agents in `internal/app/hook/answer/in_context.go` writes one JSON object on exit 0.
The object holds `systemMessage` when the answer has messages for the user.
It holds `hookSpecificOutput` with `additionalContext` and `hookEventName` only when the answer has contexts and the
native
event takes them.
With neither, stdout stays empty; with a block, it is exit 2 with the reasons on stderr and stdout empty.

The adapters read `SubagentStart` of Claude Code and of Codex as `subagent_start`, its actor from `agent_type`.
Both add `SubagentStart` to their events that take context.
The pass routes `subagent_start` to the family of `start` and `prompt`.
There it has no guidance and no requirements, injects every time, and writes `started` for its actor when the workflow
injects once.
`pass.due` resets a once-only inject of a subagent on a `started` line of its own type, besides those of the session.

A notice is logged as an observed `notified` line, a new outcome of `internal/app/state`, keyed by the line of its entry
and the expanded text, without actor.
It is written only once the answer is not blocked, as the injects of a prompt are.

The setup domain `internal/app/setup` gives `HookHandler` a status message.
`internal/platform/agentconf` writes it as the key `statusMessage` of each handler and reads it back.
`Setup.Hooks` registers `SubagentStart` for both agents with the status message `polix` on every handler.

Errors: a workflow with a problem of the new checks is invalid, and the hooks block edits and commands as with any
problem.
Stops are the exception: they only warn, which is now a `systemMessage`.
A failing write of the `notified` line leaves the notice due again; it is never an error of the call.

### Discarded alternatives

- Warnings of `polix config check` for dubious combinations: the user asked that only valid combinations be allowed.
- Blocking with a JSON decision instead of exit 2, to carry `systemMessage` with a block: it changes every gate.
  Claude Code also shows the reason of a blocked prompt to the user and not to the agent.
- Adding the messages for the user to the reason of a block: they would reach the agent, not the user.
- A different status message per event: the user chose the fixed `polix`.
- Keeping the internal warnings on stderr: their destination depends on the agent and the event, and is often nowhere.

## Acceptance criteria

- AC1: `polix install` writes `"statusMessage": "polix"` on every handler of polix in `.claude/settings.json` and
  `.codex/hooks.json`, the update one included.
  `examples/claude-settings.json` and `examples/codex-hooks.json` equal what it writes in an empty repository.
  Rerunning it on settings without the key adds it and keeps every other hook.
- AC2: the verbs each event allows are one table of the domain, identical for both agents.
  `start`, `post_edit`, `post_command` and `subagent_start` take `run`, `inject` and `notify`.
  `prompt`, `pre_edit`, `pre_command` and `pre_commit` take all four verbs.
  `stop` and `subagent_stop` take `run`, `block` and `notify`.
  `polix config check` reports every other pair as a problem naming the event, the verb and the line.
- AC3: `polix config check` reports as a problem an entry that follows a `block` without `when`, `unless` or `once` in
  the same event, other than `stop` and `subagent_stop`, naming both lines.
- AC4: a `SubagentStart` of Claude Code or of Codex is evaluated as `subagent_start` with the subagent type as actor.
  Its injects and its failing runs reach the subagent as `additionalContext` of `SubagentStart`.
- AC5: after a `subagent_start` of a type, the once-only injects of later events are due again for that type only.
  The main session and the other subagent types keep theirs.
- AC6: `polix install` registers `SubagentStart` for both agents.
  `polix doctor` reports a missing `SubagentStart` hook of Claude Code as a warning that asks to run `polix install`.
- AC7: every message for the user goes as `systemMessage` in the JSON of an exit 0 and never to stderr.
  On a blocked answer the messages are not written and the log has a `skip` decision for them.
- AC8: for every neutral event and every answer the table of AC2 allows, the render of each agent writes only the fields
  that agent accepts on that native event.
  A test of both adapters proves it: contexts, messages, both, a block, and nothing.
- AC9: `notify: <message>` and `notify: { message, always }` show the expanded message to the user.
  It shows once per session and text, whatever the actor and across a `start`, or every time with `always: true`.
  Each notice shown is logged, and `polix log` shows it.
  `polix config fmt` writes `always` before `message`, and `polix workflow graph` draws the verb.
- AC10: `polix config check` reports as a problem a `notify` followed in its event by an entry that may block, naming
  both lines.
  An entry may block when it is a `block`, or a `run` on an event outside `workflow.Informed`.
- AC11: `docs/polix.md`, `docs/reference.md`, `examples/polix.yaml`, the documents of the use cases touched,
  `README.md` and `CHANGELOG.md` describe the three features and the new problems of `polix config check`.

## Out of scope

- `ask`, `continue: false`, `updatedInput`, `onFailure`, `sessionTitle`, `PostToolBatch` and `pre_compact`: Codex does
  not
  support them as Claude Code does, so they would break the neutrality of `polix.yaml`.
- `inject` on `stop` and `subagent_stop`: Codex takes no `hookSpecificOutput` there.
- A test against a running Codex: the adapter of Codex stays experimental and untested, as `docs/polix.md` says.
- The built-in guidance and the requirements on `subagent_start`: they are told to the main session.

## Tasks

### Task 1: Register the status message polix on every hook of both agents

- Covers: AC1.
- Depends on: none.
- Files: `internal/app/setup/setup_hooks.go` (`HookHandler`, `Setup.Hooks`, modify),
  `internal/platform/agentconf/agentconf_hooks.go` (`Handler`, `hookHandler`, `ParseHooks`, `encodeGroups`, modify),
  `internal/app/setup/install/out.go` (`Adapter.groups`, modify), `internal/app/setup/refresh/out.go` (modify),
  `internal/app/setup/diagnose/out.go` (modify), `examples/claude-settings.json` (modify),
  `examples/codex-hooks.json` (modify), `.claude/settings.json` (modify), `internal/app/setup/install/install.md`
  (modify),
  `docs/polix.md` (modify), `CHANGELOG.md` (modify).
- Rules: GOARCH-1, GOARCH-3, GOARCH-13, GOARCH-19, GOARCH-21, GOARCH-24, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-29,
  GOARCH-30, GOARCH-36, GOARCH-42, GO-2, GO-4, XPLAT-1, XPLAT-2, DOC-1.
- Reuse: `agentconf.MergeHooks` from `internal/platform/agentconf/agentconf_hooks.go`; `HookHandler.Alone` from
  `internal/app/setup/setup_hooks.go`.
- Changes: `HookHandler` gains the field `Status`, the text the agent shows while the hook runs, empty for none.
  `Setup.Hooks` sets it to a new constant `StatusMessage`, `polix`, on the call and on the update handler of both
  agents.
  `agentconf.Handler` gains the same field.
  `hookHandler` writes it as `statusMessage`, omitted when empty, after `timeout`, and `ParseHooks` reads it back.
  The adapters of install, refresh and doctor copy the field between `setup.HookHandler` and `agentconf.Handler`.
  `MergeHooks` already replaces the handlers of polix, so rerunning install refreshes the key and keeps every other
  hook.
  The examples and the `.claude/settings.json` of this repository are regenerated with `just install` and
  `polix install`.
  `install.md`, the section on install of `docs/polix.md` and the changelog, under Added, say that the hooks show
  `polix`.
- Tests: in `internal/platform/agentconf/agentconf_hooks_test.go`, a file without the key gets it on merge, and another
  hook of the group keeps its own fields.
  Reading a file back returns the status of each handler.
  In `internal/app/setup/setup_hooks_test.go`, every handler of `Setup.Hooks` of both agents carries `polix`.
  The tests of `tests/setup` that keep the examples equal to what install writes pass with the regenerated files.
- Verification: `just test`, expected: every test passes; `just check`, expected: no problem.

### Task 2: Allow only the verbs each event supports in both agents and reject unreachable entries

- Covers: AC2, AC3.
- Depends on: none.
- Files: `internal/app/workflow/workflow.go` (modify), `internal/app/workflow/workflow_vocabulary.go` (create),
  `internal/app/workflow/workflow_entry.go` (`validator.inject`, `validator.block`, `validator.action`, modify),
  `internal/app/workflow/workflow_order.go` (create), `internal/app/workflow/workflow_vocabulary_test.go` (create),
  `internal/app/workflow/workflow_order_test.go` (create), `internal/app/workflow/workflow_entry_test.go` (modify),
  `internal/app/setup/check/check.md` (modify), `docs/polix.md` (modify), `docs/reference.md` (modify),
  `CHANGELOG.md` (modify).
- Rules: GOARCH-1, GOARCH-2, GOARCH-4, GOARCH-13, GOARCH-18, GOARCH-21, GOARCH-35, GOARCH-38, GOARCH-41, GOARCH-44,
  GO-2, GO-4, DOC-1.
- Reuse: `validator.add` and `validator.byLine` from `internal/app/workflow/workflow.go`; `workflow.Informed`.
- Changes: the event constants, `Events`, the verb constants, `Actions`, `Conditions`, `Measures`, `Builtins`,
  `conditionEvents` and `Informed` move unchanged to `workflow_vocabulary.go`.
  This keeps `workflow.go` under the 400 lines of GO-4 once the new names arrive.
  A new table `EventVerbs` maps each event of `Events` to the verbs it allows, the intersection of both agents.
  It holds `run` and `inject` on `start`, `post_edit` and `post_command`, `run`, `inject` and `block` on `prompt`,
  `pre_edit`, `pre_command` and `pre_commit`, and `run` and `block` on `stop` and `subagent_stop`.
  `validator.action` reports a verb the table does not allow on its event as one problem, naming the event, the verb,
  the
  verbs it allows and why.
  This replaces the checks of `validator.inject` and `validator.block` against the event and keeps their hints:
  use inject or a run on an event that already happened, and the agent cannot receive context on a stop.
  `workflow_order.go` adds a validator pass over the entries of each known event, called from `validator.events` after
  the
  entries.
  Outside `stop` and `subagent_stop`, it reports each entry that follows a `block` without `when`, `unless` or `once`,
  as "never runs: the entry of line M always blocks the event", at the line of the later entry.
  An entry expanded from a shortcut is named by its shortcut, as the other problems name it.
  `check.md`, the sections on entries and on `polix config check` of `docs/polix.md`, `docs/reference.md` and the
  changelog, under Changed, list the verbs of each event and the new problem.
- Tests: in `workflow_vocabulary_test.go`, every event of `Events` has a row in `EventVerbs`, and every verb of a row is
  in `Actions`.
  In `workflow_entry_test.go`, each pair outside the table is a problem with its line: `block` on each event of
  `Informed`, `inject` on `stop` and `subagent_stop`.
  Each pair inside it is valid.
  In `workflow_order_test.go`, an entry after an unconditional `block` is a problem.
  It is no problem after a `block` with `when`, with `unless` or with `once`, nor on `stop` and `subagent_stop`.
  A shortcut followed by an entry is named by the shortcut.
- Verification: `just test`, expected: every test passes; `polix config check examples/polix.yaml`, expected: no
  problem.

### Task 3: Evaluate the start of a subagent as the event subagent_start in both agents

- Covers: AC4, AC5, AC6.
- Depends on: Task 1, Task 2.
- Files: `internal/app/workflow/workflow_vocabulary.go` (`Events`, `EventVerbs`, `Informed`, modify),
  `internal/app/hook/answer/in_claude.go` (`claude.parse`, `claudeContextEvents`, modify),
  `internal/app/hook/answer/in_codex.go` (`codex.parse`, `codexContextEvents`, modify),
  `internal/app/hook/answer/logic.go` (`route`, modify), `internal/app/hook/answer/logic_session.go` (`pass.session`,
  modify),
  `internal/app/hook/answer/logic_inject.go` (`pass.due`, modify), `internal/app/workflow/workflow.go` (`InjectsOnce`,
  modify),
  `internal/app/setup/setup_hooks.go` (`HookEvents`, `ClaudeHookEvents`, `Setup.Hooks`, modify),
  `internal/app/setup/diagnose/logic_hooks.go` (`event`, modify), `examples/claude-settings.json` (modify),
  `examples/codex-hooks.json` (modify), `.claude/settings.json` (modify), `internal/app/hook/answer/in_claude_test.go`
  (modify),
  `internal/app/hook/answer/in_codex_test.go` (modify), `internal/app/hook/answer/logic_test.go` (modify),
  `internal/app/setup/setup_hooks_test.go` (modify), `internal/app/setup/diagnose/logic_test.go` (modify),
  `internal/app/hook/answer/answer.md` (modify), `internal/app/setup/install/install.md` (modify),
  `internal/app/setup/diagnose/diagnose.md` (modify), `docs/polix.md` (modify), `docs/reference.md` (modify),
  `CHANGELOG.md` (modify).
- Rules: GOARCH-3, GOARCH-4, GOARCH-12, GOARCH-18, GOARCH-20, GOARCH-21, GOARCH-23, GOARCH-34, GOARCH-35, GOARCH-36,
  GOARCH-37, GOARCH-38, GOARCH-41, GOARCH-44, GO-2, GO-4, XPLAT-2, DOC-1.
- Reuse: `hookInput.single` and `hookInput.actor` from `internal/app/hook/answer/in.go`; `pass.entries`,
  `pass.writeInjected` and `pass.write` of the answer package; `state.Query` with its field `Actor`.
- Changes: a new event constant `SubagentStart`, `subagent_start`, goes into `Events` after `prompt`.
  It goes into `Informed`, since it cannot be stopped in either agent, and into `EventVerbs` with `run` and `inject`.
  The conditions `path`, `command`, `writes` and `exit` stay rejected there through `conditionEvents`.
  `claude.parse` and `codex.parse` read the native `SubagentStart` with `hookInput.single` as `subagent_start`.
  The actor comes from `agent_type`, and both adapters add `SubagentStart` to their events that take context.
  `route` sends `subagent_start` to `pass.session`.
  There it skips the guidance and the requirements, sets `always` as on `start`, and writes the observed `started` line
  with the subagent type as actor.
  It writes it only when `ObservesStart` holds.
  `InjectsOnce` now skips `subagent_start` as it skips `start`, since every inject there acts every time.
  `pass.due` resets the once-only injects of the main session, and of every actor, on a `started` line of the session.
  It resets those of a subagent also on a `started` line of its own type, besides the `finished` it already reads.
  `HookEvents` and `ClaudeHookEvents` gain `SubagentStart` after `UserPromptSubmit`, without tools.
  `Setup.Hooks` registers the call of polix on it for both agents with the status message of Task 1.
  `event` of the doctor reports a missing `SubagentStart` as a warning, as it does for `PostToolUseFailure`.
  The examples and `.claude/settings.json` are regenerated.
  `answer.md`, `install.md`, `diagnose.md`, the sections on events and hooks of `docs/polix.md`, `docs/reference.md` and
  the changelog, under Added, describe the event, its verbs and what it resets.
- Tests: in `in_claude_test.go` and `in_codex_test.go`, a `SubagentStart` input with an `agent_type` parses to one
  `subagent_start` event of that actor.
  Its rendered contexts are a `hookSpecificOutput` with `hookEventName` `SubagentStart`.
  In `logic_test.go`, an inject on `subagent_start` acts on every call.
  A once-only inject of `pre_edit` already injected into `polix:reviewer` is due again after its `subagent_start`.
  The same inject stays not due for the main session and for `polix:builder`.
  A failing run on `subagent_start` informs and never blocks, and an invalid workflow only warns.
  In `setup_hooks_test.go`, both agents register `SubagentStart`.
  In the tests of `internal/app/setup/diagnose`, settings without `SubagentStart` give a warning and not a fail.
- Verification: `just test`, expected: every test passes; `just check`, expected: no problem, and the doctor reports the
  hooks registered.

### Task 4: Show every message for the user as systemMessage, writing only the fields each agent accepts

- Covers: AC7, AC8.
- Depends on: Task 3.
- Files: `internal/app/hook/answer/in_context.go` (`contextOutput`, `contextEvents.render`, modify),
  `internal/app/hook/answer/logic_entries.go` (`pass.evaluate`, `pass.warn`, modify),
  `internal/app/hook/answer/logic_session.go` (`pass.session`, modify), `internal/app/hook/answer/logic_stop.go`
  (`pass.stop`, modify),
  `internal/app/hook/answer/in_context_test.go` (create), `internal/app/hook/answer/logic_test.go` (modify),
  `internal/app/hook/answer/answer.md` (modify), `docs/polix.md` (modify), `CHANGELOG.md` (modify).
- Rules: GOARCH-3, GOARCH-4, GOARCH-12, GOARCH-16, GOARCH-18, GOARCH-20, GOARCH-21, GOARCH-34, GOARCH-35, GOARCH-36,
  GOARCH-37, GOARCH-38, GOARCH-41, GO-2, GO-4, DOC-1.
- Reuse: `agent.Answer` and its field `Warnings` from `internal/app/agent/agent_answer.go`; `pass.log` of the answer
  package.
- Changes: `contextOutput` gains `systemMessage`, omitted when empty, and its `hookSpecificOutput` becomes omitted when
  there is no context.
  `contextEvents.render` writes, on exit 0, one JSON object with the messages for the user joined in `systemMessage`.
  The object holds the contexts in `hookSpecificOutput` only when the native event is in its events that take context.
  It writes nothing to stdout when neither is there, and nothing to stderr.
  A block keeps exit 2 with the reasons and the contexts on stderr, as today, and writes no message for the user.
  Before that answer is returned, the logic logs each message of a blocked answer as a `skip` decision that says the
  event was blocked and the message not shown.
  The prefix `polix:` of each message stays, so the user knows who speaks.
  `answer.md`, the sections on the answer to the agent of `docs/polix.md` and the changelog, under Changed, say that the
  messages for the user are a `systemMessage` and are not shown on a blocked event.
- Tests: `in_context_test.go` holds a table over every neutral event, its native event in each agent, and every answer
  `EventVerbs` allows on it: contexts, messages, both, a block, and none.
  For each case it checks the exit code, that stdout is empty or one JSON object, and its keys.
  The keys must be within the documented fields of that agent on that native event: `systemMessage` on all.
  `hookSpecificOutput` may only hold `additionalContext` and `hookEventName`, and only on the events of
  `claudeContextEvents` or `codexContextEvents`.
  It also checks that stderr is empty on exit 0.
  In `logic_test.go`, the warning of the loop guard reaches `Warnings`.
  A protection that blocks with a pending warning logs it as not shown.
- Verification: `just test`, expected: every test passes.

### Task 5: Add the verb notify, shown to the user once per session and text

- Covers: AC9, AC10, AC11.
- Depends on: Task 2, Task 3, Task 4.
- Files: `internal/app/workflow/workflow_vocabulary.go` (`Actions`, `EventVerbs`, modify),
  `internal/app/workflow/workflow_entry.go` (`Action`, `NotifyKeys`, `validator.action`, modify),
  `internal/app/workflow/workflow_order.go` (modify), `internal/app/workflow/load/out_workflow.go` (modify),
  `internal/app/workflow/load/out_unmarshal.go` (modify), `internal/app/workflow/format/logic_order.go` (modify),
  `internal/app/workflow/graph/logic.go` (modify), `internal/app/state/state_entry.go` (modify),
  `internal/app/hook/answer/logic_entries.go` (`pass.apply`, modify), `internal/app/hook/answer/logic_notify.go`
  (create),
  `internal/app/hook/answer/logic_notify_test.go` (create), `internal/app/workflow/workflow_entry_test.go` (modify),
  `internal/app/workflow/workflow_order_test.go` (modify), `internal/app/workflow/load/out_workflow_test.go` (modify),
  `internal/app/workflow/format/logic_order_test.go` (modify), `internal/app/workflow/graph/logic_test.go` (modify),
  `internal/app/hook/answer/answer.md` (modify), `internal/app/workflow/load/load.md` (modify),
  `internal/app/workflow/format/format.md` (modify), `internal/app/workflow/graph/graph.md` (modify),
  `internal/app/workflow/log/log.md` (modify), `examples/polix.yaml` (modify), `docs/polix.md` (modify),
  `docs/reference.md` (modify), `README.md` (modify), `CHANGELOG.md` (modify).
- Rules: GOARCH-1, GOARCH-2, GOARCH-3, GOARCH-4, GOARCH-13, GOARCH-16, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-25,
  GOARCH-26, GOARCH-27, GOARCH-29, GOARCH-30, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-38, GOARCH-41, GOARCH-44, GO-2,
  GO-4, DOC-1.
- Reuse: `pass.expand`, `pass.currentState`, `pass.write`, `pass.log` and `pass.warnLog` of the answer package; the
  decoding of `injectYAML` in `out_unmarshal.go`, which `notifyYAML` follows; `state.Query`.
- Changes: a new verb constant `VerbNotify`, `notify`, goes last in `Actions` and on every row of `EventVerbs`.
  `Action` gains `Notify`, a new domain struct `Notify` with `Message`, `Always` and `Keys`.
  `Notify.Valid` reports an unknown key and an empty message, and `NotifyKeys` lists `always` then `message`.
  `validator.action` validates it and its placeholders as the other verbs.
  The pass of `workflow_order.go` also reports each `notify` followed in its event by an entry that may block.
  An entry may block when it is a `block`, or a `run` on an event outside `Informed`.
  The message names both lines and says to move the notice after the gate or into its message.
  The load use case decodes `notify` as a string, the message, or a map with `message` and `always`, as `injectYAML`.
  `polix config fmt` orders its keys by `NotifyKeys`, and `polix workflow graph` draws it with its message.
  `internal/app/state` gains the outcome `notified`.
  `logic_notify.go` carries out the verb.
  It expands the message, which is skipped when it expands to nothing.
  It shows the message unless the log has a `notified` line of the session with the line of the entry and that text,
  whatever the actor, or `always` is set.
  It adds the message to the answer with `Answer.Warn` and logs a `notify` decision.
  It keeps it to be written as an observed `notified` line once the answer of the event is not blocked, as the injects
  of
  a prompt are.
  A log that cannot be read shows it every time with the warning of `pass.warnLog`.
  `pass.apply` calls it for `VerbNotify`, and a notice never stops the evaluation.
  `examples/polix.yaml` shows one `notify` with its comment, and `subagent_start` with an inject of the rules for a
  reviewer.
  The documents of the use cases, `docs/polix.md`, `docs/reference.md`, `README.md` where it lists the verbs and the
  changelog, under Added, describe the verb, its forms and how often it acts.
  They also describe the new problem of check and that the notices go to the user and never to the agent.
- Tests: in `workflow_entry_test.go`, both forms are valid on every event.
  A notify without a message, with an unknown key, or with a non boolean `always` is a problem.
  In `workflow_order_test.go`, a `notify` before a `block`, or before a `run` on `pre_commit`, is a problem with both
  lines.
  It is valid after them, before a `run` on `post_edit`, and on `stop` after the gates.
  In `out_workflow_test.go`, both forms decode.
  In `logic_order_test.go`, `always` comes before `message`, and `graph/logic_test.go` draws the verb.
  In `logic_notify_test.go`, a notice shows once per session and text across two actors and after a `start`.
  It shows every time with `always`, shows again when a placeholder changes its text, and shows every time with an
  unreadable log.
  It is not written as `notified` on a blocked prompt.
- Verification: `just test`, expected: every test passes; `polix config check examples/polix.yaml`, expected: no
  problem;
  `golangci-lint run ./...`, expected: no finding.

## Risks

- Codex drops a whole answer for one field it does not accept, and lets the action through: the table test of Task 4
  pins
  the fields of each agent on each native event, from the documentation of hooks of both agents.
- The problem of unreachable entries, and of a notice before a gate, can make an existing `polix.yaml` invalid, and its
  hooks then block edits: the changelog says so under Changed, and the message says how to fix it.
- An older Claude Code or Codex that does not know `statusMessage` or `SubagentStart`: both ignore an unknown key and an
  unregistered event in their hooks file, so the hook only loses the text or the event.
- Two subagents of the same type running at once reset the once-only injects of each other on `subagent_start`: one of
  them gets a context twice, which costs tokens but loses nothing.
- Moving the internal warnings from stderr to `systemMessage` makes them visible to users who never saw them: their text
  is kept, prefixed with `polix:`.

## Verification

- `just test`: every test passes, the architecture tests included.
- `golangci-lint run ./...`: no finding.
- `just check`: `polix config check`, `polix rules check` and `polix doctor --offline` report no problem.
- `polix config check examples/polix.yaml`: no problem.
- In a Claude Code session of this repository, after `just install` and `polix install`:
  1. The spinner shows `polix` while a hook runs.
  2. With an inject on `subagent_start` for `Explore`, the subagent, asked, quotes the injected text before any tool
  use.
  3. With a `notify` on `prompt`, the user sees the notice once, a second prompt does not show it, and `polix log` lists
  it.
  4. A `notify` placed before a `block` of `pre_commit` makes `polix config check` report both lines.
