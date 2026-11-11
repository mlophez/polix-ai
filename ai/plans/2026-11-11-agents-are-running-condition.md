# Condition agents_are_running, and run_on_stop waits for the subagents still running

- Status: planned
- Risk: high: it adds fields and an outcome to the log, reads new fields of the hook input, and changes the schema
  of polix.yaml and the behaviour of run_on_stop.
- Reference: none.
- Created: 2026-11-11.
- Size: 4 tasks, about 1150 changed lines.

## Overview

polix gains a condition, `agents_are_running`, that tells whether subagents of the session are still running, on any
event: a commit, an edit or the end of a turn can wait until every subagent has ended.

polix writes to its log when each subagent starts and when it ends.
Because a subagent that is interrupted may never report its end, polix also corrects that record with the list of
work still in flight that Claude Code sends when a turn stops: a subagent missing from that list counts as ended.
A new session, a resume or a clear also closes every subagent left open; a compaction does not, since background
subagents survive it.
Codex, which sends no such list, relies on the log alone.

The checks of `run_on_stop` now wait by default: while a subagent is still running they are skipped, the user sees a
notice on every stop that skips them, and they run on the next stop after the subagents end.
An item can opt out with `wait_agents: false`.

## Request

ahora que tenemos el sub_agent start y sub_agent stop. podemos crear una nueva condicion que evalue si hay subagents corriendo, esto enriquece los flujos porque puedo no lanzar algo hasta que no terminen todos los subagentes, por ejemplo el los run_on_stop

- wait_subagents: true|false por defecto a true
- no hay otra solucion para esto? el tema es que me deja sin test desactivados durante la session
- No tiene claude algun tipo de socket para preguntar?
- pero entonces en un pre commit no podría usarlo, que pasa con con codex.
  Porque tomas assumption sin preguntar?
- pongamos agents_ y dame alternativas acabadas en ed
- wait_agents: false, y agents_awaited: true
- no me gusta,me gusta mas actor: session, agents_are_run: true
- pon are_running

## Context

`subagent_start` and `subagent_stop` write `started` and `finished` lines to the log only when a once-only inject or
a `finished` condition needs them (`workflow.Workflow.ObservesStart`, `answer.pass.observeFinished`), and the lines
carry the type of the subagent but not its `agent_id`, so two subagents of one type cannot be told apart.
Claude Code does not document that `SubagentStop` fires for an interrupted subagent, and `Stop` does not fire on a
user interrupt, so start and end lines alone can leave a subagent open forever.
The hooks reference of Claude Code documents a `background_tasks` array in the input of `Stop` and `SubagentStop`, a
snapshot of the in-flight tasks of the parent session with `id`, `type`, `agent_type` and `status`, and `agent_id`
on `SubagentStart`, `SubagentStop` and the tool events inside a subagent; polix reads none of them today, nor the
`source` of `SessionStart`.

## Constraints

- polix.yaml means the same in Claude Code and Codex: the condition works with the log alone when the input carries
  no `background_tasks` (`docs/polix.md`, Known limits).
- A missing or malformed `background_tasks`, `agent_id` or `source` never fails the hook call.
- The log stays readable by an older polix and an older log by the new one: new fields are optional, and lines
  without `agent_id` are paired by type (`internal/app/state`, lines are kept as read).
- An item of `run_on_stop` without `wait_agents` waits (request: "por defecto a true").
- The messages polix writes are in English (GO-2).
- The architecture rules of `ai/rules/architecture.yaml`: `state.Entry`, `state.State` and `agent.Event` are domain
  structs (GOARCH-25 to GOARCH-30), the decoding of JSON stays in the in and out files (GOARCH-44), the logic stays
  pure (GOARCH-4, GOARCH-18), and every use case document follows its code (GOARCH-21, GOARCH-36).

## Decisions

- The condition is `agents_are_running: true|false`, a boolean, over any subagent type, named in the interview
  over `subagents_running`, `agents_awaited` and `agents_are_run`, and over `wait_subagents` as the name of both.
- It is valid on every event: the log records each start and end of a subagent, and each `Stop` and `SubagentStop`
  that carries `background_tasks` reconciles the log with it (interview).
- Codex counts only by the log, with no reconciliation (interview).
- A `Stop` or `SubagentStop` of Claude Code without `background_tasks`, or with a malformed one, counts by the log
  without reconciling (interview).
- On `subagent_stop` the subagent that stops does not count itself (interview); the same holds for any event of a
  subagent, identified by its `agent_id`, since the answer excludes the one that acts.
- A `start` of the session whose `source` is `startup`, `resume` or `clear` closes the subagents open in the session;
  `compact` does not (interview).
  A `start` without `source` (Codex, or an older Claude Code) closes them too, since it cannot be told from a new
  session and closing never leaves checks off (request: "me deja sin test desactivados").
- Every entry of `background_tasks` of `type` `subagent` counts as running, whatever its `status` (interview).
- `run_on_stop` items take a new key `wait_agents`, a boolean, `true` when left out (interview,
  renamed from `wait_subagents`).
- The notice shows on every stop that holds a check back, once per answer (interview), with the text
  `run_on_stop waits for the subagents still running: its checks run on the next stop after they end.`
- Assumption: `requires:` of `examples/polix.yaml` is left as it is; the release that ships the key bumps it.

## Approach

**Input.**
`hookInput` (`internal/app/hook/answer/in.go`) decodes `agent_id`, `source` and `background_tasks`, the last one
leniently: only entries of `type` `subagent` are kept, and a value that does not decode counts as absent, without
error.
`agent.Event` gains the `agent_id` of the actor (empty for the main session), the `source` of a start, and the
subagents in flight with whether the input carried the array at all, filled only on `stop` and `subagent_stop`.

**Log.**
`state.Entry` gains `AgentID`, encoded as `agent_id`, omitted when empty, in the line types of
`internal/app/state/load/out.go` and `internal/app/hook/answer/out_log.go`.
The `started` line of `start` takes the `source` as its key; the `started` line of `subagent_start` and the
`finished` line of `subagent_stop` carry the `agent_id`.
A new outcome `ended`, without hash, keyed by nothing and carrying the `agent_id` and type of the subagent, records a
subagent closed by a reconciliation.
`workflow.Workflow` gains a method that tells whether any entry uses `agents_are_running` (in `when`, `unless`, `not`
or `any`, expanded shortcuts included), and `ObservesStart` and `answer.pass.observeFinished` write their lines also
when it does; the `finished` line it adds has no hash, so the condition `finished` never reads it.
A `subagent_stop` that is refused writes nothing (the subagent goes on); one the loop guard lets through writes its
`finished` line, as today when the workflow injects once.

**Query.**
`state.State` gains a method that returns the subagents still running in a session: walking the observed lines in
order, a `started` of `subagent_start` opens one; a `finished` or an `ended` closes the one of the same `agent_id`,
or, without one, the oldest open of the same type; a `started` of `start` whose key is not `compact` closes all.

**Condition.**
`workflow.Conditions` gains `agents_are_running`, valid on every event; the validator (`workflow_cond.go`) reports a
value that is not a boolean with `agents_are_running needs true or false`.
`answer.pass.condition` decides it from the in-flight subagents of the event when the input carried the array, else
from the query over the log, leaving out the `agent_id` of the actor; `cost` ranks it with `finished`, since it may
read the log.
A log that cannot be read counts as no subagent running, with the warning of `warnLog`.

**Reconciliation.**
On `stop` and `subagent_stop` carrying the array, when the workflow uses the condition, polix writes an `ended` line
for each subagent the log has open in the session and the array does not list, other than the actor.
The order of `pass.stop` is: the reconciliation first, then the entries of the event (the expanded `run_on_stop`
among them), then, once the answer is known, the `finished` line of a `subagent_stop` that is not refused.
The entries never depend on the `ended` lines of the same call: with the array known the condition reads the array.

**run_on_stop.**
`workflow.GuardrailCheck` gains whether the item waits, set by the loader
(`internal/app/workflow/load/out_guardrails.go`) to `true` unless `wait_agents: false`; `RunOnStopKeys` gains
`wait_agents`.
`Guardrails.Expand` adds `agents_are_running: false` to the `when` of each waiting item and, when at least one item
waits, one entry on `stop` after the run entries of `run_on_stop`: a `notify` with `always`, the text of
`Decisions`, and a `when` of `agents_are_running: true` with one `changed` holding the globs of every waiting item,
or no `changed` when one of them has none; it takes the line of the first waiting item and the path
`guardrails.run_on_stop`.

### Discarded alternatives

- Only `background_tasks`, valid on `stop` and `subagent_stop`: no orphans, but the condition is useless on
  `pre_commit` and in Codex.
- Only the log: an interrupted subagent holds every check back until the session restarts.
- A time limit on an open subagent: arbitrary, and a long subagent would stop counting while alive.
- Liveness from the last tool call of a subagent: the hooks of polix see only edits and commands, so a subagent that
  only reads looks dead.
- Ask Claude Code through a socket or a command: none is documented for subagents; `claude agents --json` lists
  background sessions, not subagents.
- A `running: <type|list>` condition: the user chose the boolean.

## Acceptance criteria

- AC1: on `pre_commit` of the main session, an entry with `when: { agents_are_running: true }` applies after a
  `SubagentStart` of `agent_id` `a1` and before its `SubagentStop`, and stops applying once `a1` stops;
  `agents_are_running: false` does the opposite ("en un pre commit").
- AC2: two subagents of the same type, `a1` and `a2`, started and only `a1` stopped, still count as running.
- AC3: without `agent_id` in the inputs (Codex), a start and a stop of one type pair by type.
- AC4: on `stop`, a `background_tasks` that lists a subagent makes `agents_are_running` true, and one that lists none
  makes it false even when the log has an open subagent; that open subagent then gets an `ended` line and no longer
  counts on a later `pre_commit` (orphan closed).
- AC5: a `Stop` without `background_tasks`, or with a malformed one, decides by the log and writes no `ended` line.
- AC6: on `subagent_stop`, and on any event of a subagent, the subagent that acts does not count, so the last
  subagent to stop sees `agents_are_running: false`.
- AC7: a `start` with `source` `startup`, `resume` or `clear`, or without `source`, closes the open subagents of the
  session; one with `source` `compact` does not.
- AC8: `polix config check` reports an `agents_are_running` that is not a boolean at the line of the entry.
- AC9: an item of `run_on_stop` without `wait_agents`, or with `wait_agents: true`, does not run while a
  subagent is running, and runs on a later stop when none is ("no lanzar algo hasta que no terminen todos los
  subagentes").
- AC10: an item with `wait_agents: false` runs whether or not subagents are running, as today.
- AC11: each stop that holds back at least one item whose `changed` matches shows the user the notice of `Decisions`
  once; a stop with no subagent running shows none.
- AC12: `polix config check` reports a `wait_agents` that is not a boolean at the line of the item.
- AC13: a workflow that does not use `agents_are_running` and has no `run_on_stop` answers as before the change,
  and writes the same log lines but for the new `agent_id` of the lines of a subagent and the `source` as the key of
  the `started` of `start`.
- AC14: `docs/polix.md`, `examples/polix.yaml`, its plugin copy, `README.md` and `CHANGELOG.md` describe the
  condition, the reconciliation, the key, the notice and the limits of Codex.

## Out of scope

- Filtering by subagent type, and a placeholder with the subagents still running.
- Background tasks other than subagents (shells, monitors, workflows) of `background_tasks`.
- A lock on the log: parallel subagents appending at once stay a known limit.

## Starting point

- Read first: `internal/app/state/state.go` and `state_entry.go`: the entries, outcomes and queries of the log.
- Read first: `internal/app/hook/answer/logic_session.go`, `logic_stop.go` and `logic_log.go`: where `started` and
  `finished` are written and how an entry is built (`pass.entryOf`).
- Read first: `internal/app/hook/answer/logic_condition.go`: how a condition is evaluated and ranked by cost.
- Read first: `internal/app/workflow/workflow_cond.go`, `workflow_vocabulary.go` and `workflow.go`
  (`ObservesStart`, `ObservedSubagents`): the validation of conditions and what makes polix observe.
- Read first: `internal/app/workflow/workflow_shortcuts.go` (`Guardrails.Expand`) and
  `internal/app/workflow/load/out_guardrails.go`: how `run_on_stop` becomes entries.
- Read first: `internal/app/hook/answer/in.go` (`hookInput`, `hookInput.event`) and `in_claude.go`: the decoding of
  the hook input.
- Baseline: `just test`, expected: every package passes, with no failures.

## Progress

- [ ] Task 1: agents_are_running from the starts and ends of subagents in the log
- [ ] Task 2: Reconcile the log with background_tasks on stop and subagent_stop
- [ ] Task 3: run_on_stop waits for the subagents still running, with wait_agents and a notice
- [ ] Task 4: Docs, examples and changelog of agents_are_running and wait_agents

## Tasks

### Task 1: agents_are_running from the starts and ends of subagents in the log

- Covers: AC1, AC2, AC3, AC6, AC7, AC8, AC13.
- High risk: the format of the log (a new field `agent_id`, a key on the `started` of `start`), input from outside
  (`agent_id`, `source`) and the schema of polix.yaml; guarded by round-trip tests of the log, the kept tests of the
  existing queries and the differential check.
- Files: `internal/app/state/state_entry.go` (`Entry`, modify), `internal/app/state/state.go` or a new
  `internal/app/state/state_running.go` (the query of the running subagents, create or modify),
  `internal/app/state/load/out.go` (the line type, modify), `internal/app/hook/answer/out_log.go` (the line type,
  modify), `internal/app/agent/agent_event.go` (`Event`, modify), `internal/app/hook/answer/in.go` (`hookInput`,
  `hookInput.event`, modify), `internal/app/hook/answer/logic_log.go` (`pass.entryOf`, modify),
  `internal/app/hook/answer/logic_session.go` (`pass.session`, modify), `internal/app/hook/answer/logic_stop.go`
  (`pass.observeFinished`, modify), `internal/app/workflow/workflow.go` (`ObservesStart` and the new method that
  tells whether the workflow uses the condition, modify), `internal/app/workflow/workflow_cond.go` (the validator,
  and a walk of nested conditions like `appendFinished`, modify), `internal/app/workflow/workflow_vocabulary.go`
  (`Conditions`, modify), `internal/app/hook/answer/logic_condition.go` (`pass.condition`, `cost`, modify),
  `internal/app/hook/answer/answer.md` and `internal/app/state/load/load.md` (modify).
- Rules: GOARCH-1, GOARCH-2, GOARCH-3, GOARCH-4, GOARCH-13, GOARCH-16, GOARCH-18, GOARCH-21, GOARCH-25, GOARCH-26,
  GOARCH-27, GOARCH-28, GOARCH-29, GOARCH-30, GOARCH-35, GOARCH-37, GOARCH-44, GO-2, GO-4, XPLAT-1.
- Reuse: `state.Query` and `State.Observed` for the walk; `Cond.Nested` and the shape of `Cond.appendFinished` for the
  walk of nested conditions; the hook helpers of `internal/app/hook/answer/out_test.go` that feed inputs to the
  hook in a temporary repository.
- Tests:
  - `new` in the test file of the new query of `internal/app/state`: start `a1`, start `a2` of the same type, finish
    `a1` leaves `a2` running (AC2); lines without `agent_id` pair by type, the oldest first (AC3); a `started` of
    `start` with key `startup`, `resume`, `clear` or empty closes all, one with key `compact` closes none (AC7);
    lines of another session never count.
  - `new` in `internal/app/state/load/out_test.go`: a line with `agent_id` is read back with it, and a line without
    it reads as before.
  - `new` in `in_claude_test.go`: `agent_id` of `SubagentStart`, `SubagentStop` and a `PreToolUse` inside a subagent,
    and `source` of `SessionStart`, reach the event; their absence gives empty values and no error.
  - `new` in `out_test.go`: with `on.pre_commit` holding a `block` under `when: { agents_are_running: true }`, a commit
    of the main session blocks after `SubagentStart` of `a1` and not after its `SubagentStop` (AC1); the same under
    `agents_are_running: false` inverts both.
  - `new` in `out_test.go`: on `subagent_stop` of `a1`, with only `a1` started, a `block` under
    `agents_are_running: false` blocks (AC6); a refused `subagent_stop` leaves `a1` running on the next `pre_commit`.
  - `new` in `out_test.go`: a `SessionStart` with `source` `compact` keeps `a1` running, one with `clear` closes it.
  - `new` in `workflow_cond_test.go`: `agents_are_running: yes` reports `agents_are_running needs true or false` at
    the line of the entry (AC8), and the condition is accepted on `pre_commit`, `pre_edit` and `stop`.
  - `new` in `workflow_test.go`: `ObservesStart` is true for a workflow whose only use of observation is a
    `agents_are_running` nested in an `any` of an `unless`.
  - `kept`: the tests of `due` (once-only injects), of `finished` and of `polix log` pass unchanged, and a workflow
    without the condition writes no `started` nor `finished` line it did not write before (AC13).
- Changes: `Entry` gains `AgentID`, both line types encode it as `agent_id` with `omitempty`.
  `hookInput` decodes `agent_id` and `source`; `Event` gains them and `pass.entryOf` copies the `agent_id` into the
  line.
  `pass.session` writes the `source` as the key of the `started` of `start`; `ObservesStart` and
  `observeFinished` also write when the workflow uses the condition.
  `state.State` gains the query of `Approach`; `Conditions` gains `agents_are_running`, validated as a boolean,
  evaluated in `pass.condition` from the query without the `agent_id` of the actor, ranked with `finished`.
  `answer.md` and `load.md` document the new fields and the condition.
- Verification: `just test -run 'State|Running|Claude|Codex|Cond|Session|Stop|Commit|Log'`, expected: pass.
  Differential check: export `HEAD` with `git archive HEAD | tar -x -C <tmp>/base`, build both binaries into
  temporary folders, and in two temporary git repositories with the `polix.yaml` of this repository (which has no
  `agents_are_running` yet, since Task 3 adds it through `run_on_stop`, so remove its `run_on_stop` for this check)
  feed each binary the same sequence of `SessionStart`, `SubagentStart`, `PreToolUse` of a commit, `SubagentStop`
  and `Stop` through `polix hook claude`: stdout, stderr and exit codes must be equal, and the log lines too,
  `at`, the `agent_id` of the lines of the subagent and the key of the `started` of `start` aside (AC13).

### Task 2: Reconcile the log with background_tasks on stop and subagent_stop

- Covers: AC4, AC5, AC6.
- High risk: input from outside (`background_tasks`) and a new outcome of the log; guarded by the parse tests with a
  missing, empty and malformed array and the differential check.
- Files: `internal/app/hook/answer/in.go` (`hookInput`, an unexported decoding type of the array, modify),
  `internal/app/agent/agent_event.go` (`Event`, modify), `internal/app/state/state_entry.go` (`OutcomeEnded`,
  `Outcomes`, modify), the query of Task 1 in `internal/app/state` (closing on `ended`, modify),
  `internal/app/hook/answer/logic_stop.go` (`pass.stop`, the reconciliation, modify),
  `internal/app/hook/answer/logic_condition.go` (the source of the condition, modify),
  `internal/app/hook/answer/answer.md` (modify).
- Rules: GOARCH-3, GOARCH-4, GOARCH-13, GOARCH-16, GOARCH-18, GOARCH-21, GOARCH-25, GOARCH-27, GOARCH-30, GOARCH-37,
  GOARCH-44, GO-2, GO-4.
- Reuse: the query of Task 1; `pass.write` for the `ended` lines; the hook helpers of `out_test.go`.
- Tests:
  - `new` in `in_claude_test.go`: a `Stop` with `background_tasks` holding one `subagent` and one `shell` gives one
    subagent in flight and the array known; without the array, with an empty one and with a string in its place
    gives the array unknown or empty as written, and no error.
  - `new` in `in_codex_test.go`: a Codex `Stop` without the array gives it unknown.
  - `new` in `out_test.go`: after `SubagentStart` of `a1` with no `SubagentStop`, a `Stop` whose `background_tasks`
    is empty answers `agents_are_running: false`, writes one `ended` line for `a1`, and a later `pre_commit` sees
    no subagent running (AC4); a `Stop` that lists `a1` answers true and writes no `ended` line.
  - `new` in `out_test.go`: the same `Stop` without `background_tasks` answers by the log (true) and writes no
    `ended` line (AC5).
  - `new` in `out_test.go`: a `SubagentStop` of `a1` whose `background_tasks` lists `a1` and nothing else answers
    `agents_are_running: false` (AC6).
  - `new` in the test file of the query: an `ended` line closes the subagent of its `agent_id`.
  - `kept`: the loop guard tests of `logic_stop` and the existing `finished` tests pass unchanged.
- Changes: `hookInput` decodes the array leniently; `Event` gains the subagents in flight and whether the array was
  known, filled on `stop` and `subagent_stop`.
  `OutcomeEnded` joins `Outcomes`, not hashed; the query closes a subagent on it.
  `pass.stop`, when the workflow uses the condition and the array is known, writes `ended` for each subagent open
  in the log and absent from the array, other than the actor, before the entries are evaluated.
  The condition uses the array when known, leaving out the `agent_id` of the actor, else the query.
  `answer.md` documents the reconciliation.
- Verification: `just test -run 'Claude|Codex|Stop|Running|Commit'`, expected: pass.
  Differential check: with the binary built from `HEAD` in Task 1 and the working tree, feed a `Stop` without
  `background_tasks` and one with a malformed array in a repository whose `polix.yaml` has no `agents_are_running`:
  equal output, exit code and log lines, `at` aside.

### Task 3: run_on_stop waits for the subagents still running, with wait_agents and a notice

- Covers: AC9, AC10, AC11, AC12.
- High risk: existing behaviour that changes (every `run_on_stop` item now waits, and the workflow starts writing
  the starts and ends of subagents) and the schema of polix.yaml; guarded by the updated expansion tests, the kept
  tests without subagents and the differential check.
- Files: `internal/app/workflow/workflow_shortcuts.go` (`GuardrailCheck`, `RunOnStopKeys`, `Guardrails.Expand`,
  `GuardrailCheck.entry`, a builder of the notify entry like `Cond.shortcutBlock`, modify),
  `internal/app/workflow/load/out_guardrails.go` (the items of `run_on_stop`, modify),
  `internal/app/workflow/load/load.md` (modify).
- Rules: GOARCH-3, GOARCH-4, GOARCH-13, GOARCH-24, GOARCH-25, GOARCH-27, GOARCH-28, GOARCH-29, GOARCH-30, GOARCH-44,
  GO-2, GO-4.
- Reuse: `Cond.shortcutRun`, `Cond.shortcutBlock` as the model of the notify builder, `nodeYAML.check` of
  `out_guardrails.go`.
- Tests:
  - `new` in `workflow_shortcuts_test.go`: an item without `wait_agents` expands into a run entry whose `when`
    holds its `changed` and `agents_are_running: false`, followed by one notify entry with `always`, the text of
    `Decisions`, `agents_are_running: true` and one `changed` with the globs of every waiting item; an item with
    `wait_agents: false` expands as today and adds no notify; two waiting items, one without `changed`, give one
    notify entry without `changed`.
  - `new` in `out_guardrails_test.go`: `wait_agents: false` and `true` load; `wait_agents: "no"` reports a
    problem naming `wait_agents` at the line of the item (AC12).
  - `new` in `internal/app/hook/answer/out_test.go`: with a failing `run_on_stop` item and a changed Go file, after
    `SubagentStart` of `a1`, a `Stop` without `background_tasks` does not block, shows the notice once and logs no
    run; after `SubagentStop` of `a1`, the next `Stop` blocks with the failure (AC9, AC11).
  - `new` in `out_test.go`: the same item with `wait_agents: false` blocks while `a1` runs (AC10).
  - `new` in `out_test.go`: two waiting items whose `changed` both match show the notice once in the answer.
  - update the expected strings of the existing expansion tests of `workflow_shortcuts_test.go` and
    `out_guardrails_test.go` (`guardrails.run_on_stop line=18 when=map[changed:[*.go]] ...`) to the new `when` and
    the notify entry: the behaviour the request asks for.
  - `kept`: `polix config check` on the `polix.yaml` of this repository and on `examples/polix.yaml` reports nothing,
    and the tests of `internal/app/workflow/graph` still render the `run_on_stop` entries.
- Changes: `GuardrailCheck` gains whether it waits; the loader sets it to `true` unless `wait_agents: false`, only
  for `run_on_stop`, and reports a value that is not a boolean; `RunOnStopKeys` gains `wait_agents`.
  `Guardrails.Expand` adds `agents_are_running: false` to each waiting item and appends the single notify entry of
  `Approach`.
  `load.md` documents the key and its default.
- Verification: `just test -run 'Shortcut|Guardrail|Stop|Graph'`, expected: pass.
  Differential check: with the binary built from `HEAD` in Task 1 and the working tree, in a repository whose
  `polix.yaml` has a failing `run_on_stop` item and a changed Go file, feed a lone `Stop`: equal output and exit
  code; then a `SubagentStart` and a `Stop`: the base blocks, the working tree does not and shows the notice,
  which AC9 and AC11 ask for.

### Task 4: Docs, examples and changelog of agents_are_running and wait_agents

- Covers: AC14.
- High risk: none.
- Files: `docs/polix.md` (Conditions, What polix observes, the shortcut `run_on_stop`, the log format, Known
  limits, modify), `examples/polix.yaml` (the comments of conditions and of `run_on_stop`, modify),
  `plugins/polix/skills/workflow/reference.yaml` (the copy of `examples/polix.yaml`, modify), `README.md` (the
  `run_on_stop` of the demo and its prose, modify), `CHANGELOG.md` (`Unreleased`, modify).
- Rules: DOC-1, GO-2.
- Reuse: none.
- Tests: none: docs and comments; the existing test that keeps `reference.yaml` equal to `examples/polix.yaml` and
  `polix config check` over the example guard them.
- Changes: `docs/polix.md` adds `agents_are_running` to the conditions; describes in What polix observes the
  `agent_id` of the lines, the `source` key of the `started` of `start`, the outcome `ended` and the
  reconciliation; adds `wait_agents` and the notice to `run_on_stop`; and Known limits says that Codex counts by
  the log alone, that an interrupted subagent stays open until the next stop with `background_tasks` or a new
  session, that only subagents count among the background tasks, and that their `status` is not read.
  `examples/polix.yaml` and its plugin copy show `wait_agents` and list the condition.
  `README.md` says that `run_on_stop` waits for running subagents.
  `CHANGELOG.md` records under `Unreleased` the condition (Added) and the new default of `run_on_stop` (Changed).
- Verification: `just test -run 'Reference|Example'`, expected: pass.

## Risks

- Claude Code changes or drops `background_tasks`: the lenient decoding falls back to the log; orphans then last until
  a new session, as in Codex.
- The `id` of a task is not the `agent_id` of the subagent: the reconciliation would close live subagents and the
  self exclusion would fail; the parse tests pin the field names, and Known limits names it to check against a
  running Claude Code during the end to end verification.
- Parallel subagents append to the log at once without a lock: a lost `finished` line leaves a subagent open until
  the next reconciliation; already a known limit.
- A user who relied on `run_on_stop` running while a background subagent edits: `wait_agents: false` restores it,
  and the changelog records the change.
- The log grows by two lines per subagent in every workflow with `run_on_stop`: small next to one decision per
  hook call.

## Verification

- `just test`: every package passes.
- `golangci-lint run ./...`: no findings.
- `just check`: config, rules and doctor pass.
- `claude plugin validate plugins/polix`: valid.
- End to end, in a session of Claude Code with this `polix.yaml`: launch a subagent in the background that edits a
  Go file and let the main turn end: `run_on_stop` is skipped and the notice shows; when the subagent ends and the
  main session stops again, the checks run.
  Then interrupt a foreground subagent with Esc and end the next turn: the `Stop` reconciles it (an `ended` line in
  `polix log`) and `run_on_stop` runs.

## Iterations

none
