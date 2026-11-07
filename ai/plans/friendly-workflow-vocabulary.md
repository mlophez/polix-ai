# Give polix.yaml a friendlier vocabulary and let polix remember only the events the workflow asks about

## Overview

Writing a polix.yaml today asks the author to learn how the engine works inside.
There are three ways to block, each with its own rule on whether the next entries run, a shortcut that ends the
evaluation without blocking, three modes that change what a block does, and facts that the author declares by hand
and reads back by name.

After this change an entry has two parts: when it applies and what it does.

- The conditions go under `when` (was `if`) and `unless`.
- What the entry does goes under `action`, with one of three verbs: `run` a command, `inject` text for the AI, or
  `block` with a message for the AI.
- A `block` blocks every time; `once: true` turns it into a reminder that blocks once per content.
- A `run` that passed is not run again until the files change, and an `inject` reaches each agent once per session;
  `always: true` makes them act every time.
- The first entry that blocks ends the evaluation, whatever verb it is.
- Modes go away: a warning is an `inject`, and a failing command after an edit or a command is feedback for the AI.
- The event `stop` is now `turn_stop`, and the measures take a plain number.

The facts are no longer declared.
polix writes to its log only the events the workflow will ask about, each with its session, agent, branch and the
content of the files it happened on, and the conditions are questions about those lines.
The only new condition is `finished: <subagent>`: that subagent ended and the files its entry looks at did not change
since, so a review of the Go code survives an edit of the docs.
Only what polix sees for itself counts: its own commands and the end of a subagent.
A command the AI runs never counts as passed, because polix cannot know what it really ran.

The old names stop working, and `polix check` names the new spelling of each one.
This repository, the example, the docs and the plugin move to the new vocabulary.

## Context

The friction of writing a polix.yaml is the main obstacle to adopting polix, and presets will build on this vocabulary.
The log today mixes four kinds of bookkeeping (facts, injected rules, stop blocks, decisions) that each condition reads
in its own way, and the modes multiply what each action can do.
The outcome is a polix.yaml a newcomer reads without the docs, and one kind of observed line that the repeat of the
verbs, the loop guard and `finished` query.

## Decisions

- One plan for the whole change, so this repository and the example migrate their polix.yaml once.
  Presets (`use:`) come in a later plan.
- Hard break with hints: only the new names are accepted, and each old name is reported with its replacement.
  The project is not launched yet and almost no polix.yaml exists outside this repository and Logalty.
- The hints cover only names that existed: the event `stop`, the entry keys `if`, `why`, `mode`, `deny`, `require`,
  `allow`, `run`, `context` and `record`, the condition `done` and a measure written as a map with `at_least`.
- Events keep their names except `stop`, which becomes `turn_stop`; `subagent_stop` stays a separate event, so the
  gates of a turn never run when a subagent ends.
- The keys of an entry are `when`, `unless` and `action`, and `action` is required, a map with exactly one of `run`,
  `inject` or `block`.
- `why` is removed: the engine never read it; a YAML comment or the message does its job.
- `mode` is removed with its three values:
  - `warn` on a block becomes an `inject` of the same text, valid on the same events but the stops.
  - `warn` on a run becomes a plain `run` on `post_edit` or `post_command`, where a failure is feedback.
  - `audit` has no replacement: trying a gate is done in a branch, and its decisions are in the log once enforced.
- A `block` blocks every time its entry applies, since almost every documented block is a gate that must hold.
  `once: true` makes it a reminder: it blocks the first attempt of an actor on a content and lets the next through.
- A `run` and an `inject` act once by default and every time with `always: true`.
  The user chose a block that repeats by default over one repeat rule for the three verbs: a forgotten key leaves a
  gate, which is safe, instead of a reminder the AI passes by retrying.
- `block` takes a message, or a map with `message` and `once`; valid on `prompt`, `pre_edit`, `pre_command`,
  `pre_commit`, `turn_stop` and `subagent_stop`.
- `run` takes a command as a string or a list, or a map with exactly one of `command` or `rules`, plus `message` and
  `always`.
  A string is split on spaces into arguments, as a `command` string is today, and never goes through a shell.
  The message goes before the output of a failure.
- A failing `run` blocks on the events where `block` is valid.
  On `session_start`, `post_edit` and `post_command` it never blocks: its message and output reach the AI as context.
- `inject` takes a text, or a map with exactly one of `file`, `text` or `rules`, plus `always`; it is not valid on
  `turn_stop` or `subagent_stop`.
- `always` on an `inject` of `session_start` is accepted and has no effect, since that event always injects; it is
  documented, not reported, because `polix check` has no channel for warnings.
- The log keeps the kind `decision`, the evidence of what polix answered, and replaces the kinds `fact`, `injected`
  and `stop_block` with one kind `observed`.
  An observed line holds the session, the agent, the actor, the event, the branch, the content hash when a query
  needs it, the line of the workflow entry, a key and an outcome: `finished`, `passed`, `blocked`, `injected` or
  `started`.
- polix writes an observed line only when the workflow or the evidence needs it:
  - `finished`: a `subagent_stop` of a subagent type named in some `finished` of the workflow, with the hash, or of
    any subagent when the workflow has a once-only `inject`, without it.
  - `passed`: any `run` of polix that passed, with or without `always`, with the hash pinned before and after the run
    and equal, so the log shows the checks that were green on each content.
  - `blocked`: a block of an entry with `once: true`, or any block on `turn_stop` or `subagent_stop` for the loop
    guard, with the hash.
  - `injected`: a once-only `inject` that reached the AI, without hash.
  - `started`: a `session_start`, only when the workflow has a once-only `inject` or an `inject: { rules }`.
- Commands of the AI are never recorded and never count as passed.
  The text of a command does not prove what ran (aliases, another directory), a background command reports success
  before it ends, and the command may change the files it checked.
  So `passed` is not a condition: the repeat of `run` already spares a command polix ran green on the same content,
  and the order of the entries, with the single stop rule, keeps a later gate from running before a failing check.
- The scope of an entry is the set of files its `when` names in `changed` (the nearest `changed`, as the measures take
  it), or the whole tree when it names none.
  `finished` and a `once` block compare the content on that scope, not on the whole tree: an edit outside the scope
  (README.md, docs/, a plan) keeps a review of the Go code and a reminder already given.
- The content hash is the tree `git write-tree` writes, which stays in the object database of git, so polix compares
  the tree of an observed line with the current one later, listing the files that differ and matching them against
  the scope with the globs of polix.
  A tree git no longer has (pruned by `git gc`) or a comparison that fails counts as changed: the gate asks again.
- `finished: <subagent>` (or a list, all of them) holds when the latest observed `finished` of that type on the
  current branch has a tree equal to the current one on the scope of the entry.
  It is written when the answer to the `subagent_stop` does not refuse; a block let through by the loop guard does not
  write it, so the loop guard never turns a failing subagent into a finished one.
  A subagent type both observed by `finished` and reset by a once-only `inject` gets one line, with the hash.
- The repeat of a `run` is keyed by what it runs on the current branch and hash, whoever's entry it is: the expanded
  arguments joined by single spaces, or for `run: { rules: ... }` the word `rules` and the expanded target.
  A `{path}` or `{dir}` in the command makes the key differ per path, so it repeats once per path and content.
- A `block` with `once: true` is keyed by the session, the actor and the line of its entry on the current branch, and
  is skipped when the tree of its latest `blocked` line equals the current one on the scope of the entry: a new
  session, which never saw the reminder, is reminded once again.
- A once-only `inject` is keyed by the session, the actor, the line of its entry and the expanded text or file path;
  `inject: { rules }` keeps one key per rule id.
  A text with a placeholder that changes on every event (a measure, `{command}`) makes a new key each time and is
  injected every time; `{path}` gives once per path, as intended; the docs say so next to the repeat rule.
  It is due again after a `started` of the session (a new or compacted context) or, for a subagent, after a
  `finished` of its actor in the session (its next run starts with a new context).
- The loop guard counts the observed `blocked` lines of the entry, event, actor, session and hash.
- One stop rule: the first entry that actually blocks (a `block`, or a failing `run` on an event where it blocks, not
  let through by the loop guard) ends the evaluation.
- `allow` is removed: an exception is written as a `not` in `when`, or in `unless`.
  `contains` inside `any` is rejected, since there it holds on its own and makes the `any` always true; the docs show
  the exception of a plan with `when: { not: { path: ... } }`.
- The measures `changed_lines`, `changed_files` and `risk_score` take a non-negative integer meaning "at least".
- `version` stays 1 and `requires` keeps its value (`0.1.0` in the example, none in this repository).
  There is no release yet and a development build skips the check of `requires`, so bumping it would protect nobody;
  the break is handled by migrating every checkout together (Task 4).
- Old `fact`, `injected` and `stop_block` lines stay in the log, are ignored by every query and are shown by
  `polix log` with their kind; after the upgrade the reminders, the injects and the loop guard start from zero.
- In this repository the review gate and the docs reminder keep their order: with the scope, editing README.md or
  docs/ after the review neither invalidates it nor repeats the reminder, whose `changed` does not name them.
- The use cases that only showed `audit` ("Try a guardrail before enforcing it", "A record of every infrastructure
  command") are removed; "The main session orchestrates, the builder builds" runs the tests with a `run` on
  `subagent_stop` instead of trusting the builder ran them.

## Approach

The vocabulary lives in `internal/workflow`, the evaluation in `internal/hook`, the log in `internal/state`.

Domain (`internal/workflow/workflow.go`, `workflow_entry.go`):
`Entry` loses `If`, `Why`, `Mode`, `Deny`, `Allow`, `Require`, `Run`, `Context` and `Record`, and gains `When` and
`Action`; `EffectiveMode` and the mode constants go.
`Action` is a new struct with one optional field per verb, `Block`, `Run` and `Inject`, its `Valid`, and the method
that names the verb it holds.
`Block` is a new struct with `Message` and `Once`; `Context` is renamed `Inject` and gains `Always`; `Run` gains
`Message` and `Always`.
The constant `Stop` becomes `TurnStop`, valued `turn_stop`; the agent adapters map the stop of a turn to it.
`Actions` lists run, inject and block; `Conditions` loses `done` and gains `finished`.
`Measured` reads a non-negative integer.
The workflow exposes which subagent types its `finished` conditions name and whether it has a once-only `inject`, so
the engine knows what to observe.
A table maps each old name to its hint, per place (event, entry key, condition, measure value); the validator
appends it to the problem of the unknown name, keeping the file line.

State (`internal/state/state.go`, `state_entry.go`, `state_line.go`):
The kinds `fact`, `injected` and `stop_block` give way to `observed`, with the outcomes listed in Decisions.
`Entry` loses `Fact` and `Rule` and gains `Key`; `Entry.Valid` requires an outcome and, for `passed`, `blocked` and a
`finished` with hash, a non-empty hash.
`State` replaces `Done`, `Injected` and `StopBlocks` with one query over observed lines, by outcome and optionally
session, actor, branch, hash, line and key, returning the matches in order; an empty hash in the query never matches.
The callers derive from it: the latest `finished` of a subagent type and the latest `blocked` of a key (whose trees the
engine compares on the scope), whether a run key passed, how many times an entry blocked (the loop guard), and
whether an inject is due (no `injected` after the last `started` or actor `finished` of the session).

Comparing trees (`pkg/git/git.go`, `internal/hook/hook_step.go`, `answer_hook_out.go`, `hook_evaluation.go`):
`Repo` gains a method that lists the paths that differ between two trees, through `git diff-tree` by name, and fails
when a tree is missing.
`Step` gains a request to compare an observed tree with the tree of the current epoch, answered by the adapter with
those paths or the error; the facts of the evaluation keep the answers per tree and epoch, as they keep the hash.
The logic matches the paths against the scope with `pkg/glob`, so the globs mean the same as in `changed`.

Parser (`internal/workflow/workflow_parse.go`):
`entryYAML` decodes `when`, `unless` and `action`; a new `actionYAML` keeps its keys.
`run`, `inject` and `block` decode a scalar (or a sequence, for `run`) as the short form and a map as the long form,
with `argsYAML` and `stringsYAML`; any other node is a parse error naming the key.

Validator (`internal/workflow/workflow_entry.go`, `workflow.go`):
`action` must hold exactly one known key; `block` is rejected on `session_start`, `post_edit` and `post_command`, and
`inject` on `turn_stop` and `subagent_stop`; `once` and `always` must be booleans, each only on its verbs;
`finished` takes a string or a list of strings; `contains` is rejected inside `any`; placeholders are checked in
`block`, the `message` of `run` and every field of `inject`.

Engine (`internal/hook/answer_hook_logic.go`):
`apply` dispatches on the verb, and `block` loses its handling of modes.
The content hash is computed only when a query or an observed line needs it: the repeat of `run`, `passed`, a
`once` block, the loop guard and `finished`; an `inject` and a `started` line never compute it.
A `run` without `always` is skipped when its key passed on the branch and hash; any run that passes on unchanged
content writes `passed`.
A failing `run` on `session_start`, `post_edit` or `post_command` informs the AI with its message and output and goes
on; on the other events it blocks through `block`.
A `block` with `once: true` is skipped when the latest `blocked` of its session, actor and line has a tree equal to
the current one on the scope, and writes `blocked` when it blocks; on `turn_stop` and `subagent_stop` every block
writes `blocked` for the loop guard, which keeps comparing the whole hash.
A once-only `inject`, outside `session_start`, is skipped while not due and writes `injected` when it injects.
Each skip is logged as a `skip` decision and never blocks.
On `session_start`, polix writes `started` when the workflow injects once.
On `subagent_stop`, after the entries, polix writes `finished` when the workflow observes that subagent and the answer
does not refuse.
A failing `run` with a `message` refuses or informs with the message, a blank line and the output.

Log command (`internal/workflow/log_workflow_in.go`) prints the key and outcome of observed lines; graph
(`graph_workflow_logic.go`) labels edges with the new verbs, `once` and `always`.

Errors: every problem of the file is reported by `polix check` and, on a hook call, by the invalid-workflow path.
An unknown hash never satisfies `finished` and never skips a `run` or a `block`: a broken git repeats them.
A tree that cannot be compared (pruned, or git failing) counts as changed, with the error in the decision log.
A log that cannot be read makes every verb act every time, with the warning the loop guard already gives.

### Discarded alternatives

- Recording every event of the agent: the hash of every edit and command adds latency to every hook and the log grows
  with lines nobody asks about.
- Commands of the AI counting as passed: the command text, the directory, background runs and files rewritten by the
  command make it a fact the AI can produce without the check holding.
- A `passed` condition fed only by the runs of polix: the repeat of `run` and the stop rule already cover it.
- One repeat rule for the three verbs, with `always: true` on every gate: a forgotten key lets the AI pass a gate by
  retrying.
- Keeping `warn` and `audit`: `inject` already gives a warning, and a dry run is better served by a branch.
- Aliases for the old names, or `version: 2` with two parsers: two vocabularies to maintain before any launch.
- Keeping `deny` and `require` apart, or `allow`: more than one way to end the evaluation.
- Keeping the kinds `fact`, `injected` and `stop_block` and adding more: one bookkeeping per condition.
- A hash per scope computed when the line is written: the subagent ends before polix knows which entries will ask, so
  the whole tree is kept and the scope applied when asking.
- Bumping `requires`: no release exists and development builds skip the check.

## Acceptance criteria

- AC1: A polix.yaml written with `when`, `unless`, `action` (`run`, `inject` or `block`, short and long forms),
  `once`, `always`, `finished`, the event `turn_stop` and numeric measures passes `polix check`.
- AC2: `polix check` on a polix.yaml that uses `if`, `why`, `mode`, `deny`, `require`, `allow`, `run` or `context` as
  entry keys, `record`, `done`, the event `stop` or `at_least` reports each one with its line and the new spelling.
- AC3: An entry without `action`, with an empty `action`, with two verbs, with an unknown key in `action` or in the
  long form of a verb, with a `once` or `always` that is not a boolean, or with `once` on `run` or `inject` or
  `always` on `block` is reported by `polix check`.
- AC4: `block` is rejected on `session_start`, `post_edit` and `post_command`; `inject` on `turn_stop` and
  `subagent_stop`; an entry of `turn_stop` never runs when a subagent ends.
- AC5: A `run` with neither or both of `command` and `rules`, an `inject` with not exactly one of `file`, `text` and
  `rules`, or a `block` without `message` is reported; a `run` string is split on spaces.
- AC6: On any event the first entry that actually blocks ends the evaluation: a `turn_stop` whose first `run` fails
  does not run the second one.
- AC7: A `block` or failing `run` let through by the loop guard does not end the evaluation; `inject`, a skipped verb,
  a passing `run` and a failing `run` on `session_start`, `post_edit` or `post_command` never do.
- AC8: A `block` blocks every attempt; with `once: true` it blocks the first attempt of an actor in a session on a
  content and lets the second through; an edit inside the scope of the entry makes it block again, an edit outside
  does not; another actor and a new session are reminded on their own.
- AC9: A failing `run` with a `message` answers with the message followed by the output of the command.
- AC10: The measures take a non-negative integer; a negative number or a map is reported, the map with the hint.
- AC11: The polix.yaml of this repository, `examples/polix.yaml` and `plugins/polix/skills/workflow/reference.yaml` use
  only the new vocabulary, pass `polix check`, and the two examples are identical.
- AC12: README.md, docs/ and the skill and agent that write polix.yaml use only the new vocabulary, describe the repeat
  rules, the observed lines and the single stop rule, state that commands of the AI never count, and show a gate, a
  reminder with `once: true` and an exception written with `not` in `when`.
- AC13: `polix graph` shows the new verbs; `polix log` shows observed lines with their outcome and key, a verb not
  repeated as a `skip` decision, and old `fact`, `injected` and `stop_block` lines with their kind.
- AC14: A `run` without `always` that passed is not run again on the same branch and content; an edit makes it run
  again; a failing run runs again on the next event; a run with `{path}` repeats per path; a run of `rules` is keyed by
  its target.
- AC15: A `run` with `always: true` runs on every event; any run of polix that passed on unchanged content writes
  `passed`, with or without `always`; a run during which the content changed and a run with an unknown hash write
  none.
- AC16: `finished: <subagent>` holds after that subagent ended and after an edit outside the scope of the entry; not
  after an edit inside it, nor without a `changed` after any edit; not when its `subagent_stop` was refused or let
  through by the loop guard; not when its tree is missing from git; never for another type.
- AC17: A command of the AI, green or not, writes no observed line and never makes a `run` skip or `finished` hold.
- AC18: A once-only `inject` reaches each actor once per session for each text it expands to; it reaches the AI again
  after a `session_start` of the session, and a subagent again after that subagent type finished; with
  `always: true` it injects on every event; `inject: { rules }` injects each rule once per actor and session.
- AC19: polix writes observed lines only for what the workflow asks: no `finished` for a subagent the workflow does
  not observe, no `started` without a once-only inject, no `blocked` for a block without `once` outside the stops, and
  no content hash computed for an `inject` or `started`.
- AC20: The loop guard lets the agent stop after `max_stop_blocks` blocks of an entry on the same content, counted
  from observed `blocked` lines.
- AC21: A failing `run` on `session_start`, `post_edit` or `post_command` never blocks: its message and output reach
  the AI as context.
- AC22: `polix check` rejects `contains` inside `any`.
- AC23: A once-only `inject` whose text holds a measure placeholder is injected on every event whose measure changed,
  and the docs say so.

## Out of scope

- Presets (`use:`): the next plan, built on this vocabulary.
- Requiring the verdict of the reviewer, not only that it finished: it needs the output of the subagent in the hook
  input.
- Any condition on commands the AI ran: discarded for the reasons in Decisions.
- A dry run of a gate in place of `audit`, an interactive `polix init`, a `polix explain` simulator and a migration
  command for old files.
- Renaming the top-level keys and the other events.
- A channel for warnings in `polix check`.
- Rotating or compacting the log: its growth stays a known limit of docs/polix.md.

## Tasks

### Task 1: New vocabulary without modes, the observed line and finished

- Covers: AC1, AC2, AC3, AC4, AC5, AC9, AC10, AC13 (graph and observed lines), AC16, AC17, AC19 (finished), AC21,
  AC22.
- Depends on: none.
- Files: `examples/polix.yaml` (modify), `plugins/polix/skills/workflow/reference.yaml` (modify, a copy of the example),
  `docs/use-cases.md` (the polix.yaml snippets only, modify),
  `internal/workflow/workflow.go` (`TurnStop`, `Actions`, `Conditions`, the mode constants, the table of hints,
  modify), `internal/workflow/workflow_entry.go` (`Entry`, `EffectiveMode`, `Action`, `Block`, `Run`, `Inject`,
  `Measured`, the validator, modify), `internal/workflow/workflow_parse.go` (`entryYAML`, `actionYAML`, `runYAML`,
  `contextYAML`, `decodeNames`, modify), `internal/workflow/graph_workflow_logic.go` and `graph_workflow.md` (modify),
  `internal/workflow/log_workflow_in.go` and `log_workflow.md` (modify),
  `internal/state/state.go`, `state_entry.go` and `state_line.go` (modify),
  `internal/hook/answer_hook_logic.go` (`apply`, `block`, `recordEntry`, `runEntry`, `condition`, `answerHookCost`,
  modify), `internal/hook/answer_hook.md` (modify), `internal/hook/hook_step.go` (`Step`, modify),
  `internal/hook/hook_evaluation.go` (the facts of the comparison of trees, modify),
  `internal/hook/answer_hook_out.go` (`AnswerHookAdapter`, modify), `pkg/git/git.go` (`Repo`, modify),
  `internal/agent/agent_claude.go`, `agent_codex.go` and `agent_event.go` (the stop of a turn, modify), and the tests
  listed below.
- Rules: GOARCH-1, GOARCH-2, GOARCH-4, GOARCH-9, GOARCH-13, GOARCH-16, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-25,
  GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-29, GOARCH-30, GOARCH-31, GOARCH-35, GOARCH-36, GOARCH-38, GOARCH-39,
  GOARCH-40, GOARCH-7, GOARCH-19, GO-2, GO-3, XPLAT-1, XPLAT-2, DOC-1.
- Reuse: `argsYAML` and `stringsYAML` for the short forms, `mapKeys`, `validator.unknownKeys` and
  `validator.placeholders` from `internal/workflow`; `answerHookRun.hash`, `answerHookRun.write`, the epoch check
  of `runEntry` and the scope of `matches` from `internal/hook/answer_hook_logic.go`; `Repo.ContentHash` and `run`
  from `pkg/git/git.go`; `glob.AnyPath` from `pkg/glob`.
- Changes: Do not run `just install` or `just check` in this task: the installed binary is global, and a binary that
  rejects the old vocabulary locks this worktree and every other checkout of the repository until Task 4.
  The old vocabulary stops parsing at once, so the tests stay red until the task ends: work package by package
  (`internal/workflow`, then `internal/state`, then `pkg/git` and `internal/hook`, then the fixtures of the others)
  with `just test -run` on each.
  Restructure the domain as the Approach says, with `Once` and `Always` parsed and validated but not yet used by the
  engine, `Stop` renamed `TurnStop` in the domain and the adapters, and the modes removed.
  Replace the kind `fact` with `observed`, add `Key`, drop `Fact`, add the query over observed lines and drop `Done`;
  `injected` and `stop_block` stay as they are until Task 3.
  The parser decodes the new keys and forms; the old keys reach the validator as unknown keys and get their hint.
  The validator rejects `contains` inside `any`.
  In the engine, `apply` dispatches on the verb; `deny` and `require` become `block` and the evaluation ends on a
  `block` that actually blocks; `block` drops the handling of modes; a failing `run` on `session_start`, `post_edit`
  or `post_command` informs the AI instead of blocking; a `subagent_stop` of an observed type whose answer does not
  refuse writes `finished` with the hash; the measures compare the number directly.
  `Repo` gains the listing of the paths that differ between two trees, `Step` the request to compare an observed tree
  with the current one, and the adapter answers it; `finished` holds when the latest `finished` of the type on the
  branch has no differing path inside the scope of the entry, and a failed comparison does not hold.
  Until Task 3 every verb acts every time and a passing `run` writes nothing.
  A failing `run` with a `message` answers with the message, a blank line and the output.
  `polix log` prints the outcome and key of observed lines and the kind of old ones; the graph uses the new verbs.
  Rewrite `examples/polix.yaml` annotated with every option: both forms of each verb, `once`, `always`, a gate
  released by `finished`, a reminder with `once: true`, an exception with `not` in `when`, a lint `run` on
  `post_edit` as feedback, `turn_stop` and numeric measures, with its `requires` unchanged, and a comment that
  `finished` and `once` compare only the files of the `changed` of their entry; copy it to the reference.
  Rewrite the polix.yaml snippets of `docs/use-cases.md`: `record` on `subagent_stop` becomes `finished`, `record`
  after a `run` disappears, a `deny` or `require` that recorded its own fact becomes a `block` with `once: true`, any
  other a plain `block`, `mode: warn` on a block an `inject`, `mode: warn` on a run a plain `run`, `allow` a `not` in
  `when`, the builder runs `just test` with a `run` on `subagent_stop`, and the two snippets of `audit` go with their
  sections; the prose waits for Task 5.
- Tests: `internal/workflow/workflow_parse_test.go`: the short and long forms of the verbs, a run string split on
  spaces, `once`, `always`, `when` and `action` decode; a verb with a node of the wrong kind fails.
  `internal/workflow/workflow_entry_test.go` and `workflow_test.go`: every problem of AC3, AC4, AC5, AC10 and AC22,
  the shapes of `finished`, and one case per old name of AC2 checking the hint and the line.
  `internal/workflow/graph_workflow_logic_test.go` and `log_workflow_in_test.go`: the labels and lines of AC13.
  `internal/state/state_test.go`, `state_entry_test.go` and `state_line_test.go`: the validity of an observed line,
  the query (no match for another branch, another hash or an empty hash), and an old `fact` line read and ignored.
  `internal/hook/answer_hook_logic_test.go`: AC9, AC16 (a review of `*.go` that holds after an edit of README.md and
  not after an edit of a Go file, an entry without `changed` that any edit invalidates, a failed comparison), AC17 (a
  green `post_command` writes nothing), AC19 for `finished`, AC21 on each of the three events, and numeric measures on
  both sides of the threshold.
  `pkg/git/git_test.go`: the paths that differ between two trees of a scratch repository, none for equal trees, and
  an error for a missing tree.
  `internal/hook/answer_hook_out_test.go`: the adapter answers the comparison of a real tree and of a missing one.
  Rewrite every polix.yaml fixture of the tests of `internal/hook`, `internal/workflow`, `internal/setup`,
  `internal/rule`, `internal/diff`, `internal/version` and `internal/agent`, found with `rg` on the old keys; the cases
  of `allow` become exceptions with `not` in `when`, and the cases of `warn` and `audit` are rewritten or removed.
- Verification: `just test`, expected: every test passes, the architecture tests included.

### Task 2: One stop rule for the evaluation of an event

- Covers: AC6, AC7.
- Depends on: Task 1.
- Files: `internal/hook/answer_hook_logic.go` (`apply`, `runEntry`, `handle`, modify), `internal/hook/answer_hook.md`
  (modify), `internal/hook/answer_hook_logic_test.go` and `hook_evaluation_test.go` (modify).
- Rules: GOARCH-4, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-31, GOARCH-36, GO-2, DOC-1.
- Reuse: the method `block` of `answerHookRun`, which reports whether the event actually blocked.
- Changes: Do not run `just install` or `just check` in this task, for the reason given in Task 1.
  A failing `run` ends the evaluation when `block` reports that the event actually blocked, as a `block` does; let
  through by the loop guard, or on an event where it only informs, the evaluation goes on.
  Update the comments of `apply` and `handle` and the use case document to state the single rule.
- Tests: a `turn_stop` with two failing runs blocks with only the first output and never runs the second; a failing
  run let through by the loop guard lets the next entry run; two failing runs on `post_edit` both reach the AI; a
  `block` before an `inject` on `pre_edit` drops the inject; an `inject` before a `block` reaches the agent with the
  block.
  Update the cases that expected every failure of a `turn_stop` in one answer.
- Verification: `just test`, expected: every test passes.

### Task 3: Repeat rules on the observed lines

- Covers: AC8, AC13 (the skip), AC14, AC15, AC18, AC19, AC20, AC23.
- Depends on: Task 1, Task 2 (both edit `internal/hook/answer_hook_logic.go`).
- Files: `internal/state/state.go`, `state_entry.go` and `state_line.go` (modify),
  `internal/workflow/workflow.go` (what the workflow observes, modify),
  `internal/hook/answer_hook_logic.go` (`apply`, `runEntry`, `block`, `context`, `contextRules`, `handle`, modify),
  `internal/hook/answer_hook.md` (modify), `internal/workflow/log_workflow.md` (modify), and their tests.
- Rules: GOARCH-2, GOARCH-4, GOARCH-9, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-26, GOARCH-27, GOARCH-30, GOARCH-31,
  GOARCH-36, GOARCH-40, GO-2, XPLAT-1, DOC-1.
- Reuse: the query over observed lines and the comparison of trees on the scope of Task 1, `answerHookRun.hash`,
  `answerHookRun.branch`, `answerHookRun.expand` and the epoch check of `runEntry`.
- Changes: Do not run `just install` or `just check` in this task, for the reason given in Task 1.
  Drop the kinds `injected` and `stop_block` and the methods `Injected` and `StopBlocks`; their callers use the query.
  Any `run` that passes on unchanged content writes `passed` with its key: the expanded arguments, or `rules` and the
  expanded target; a `run` without `always` is skipped when its key passed on the branch and hash.
  A `block` with `once: true` is skipped when the latest `blocked` of its session, actor and line has no differing
  path inside the scope of the entry, and writes `blocked` when it blocks; every block on `turn_stop` and
  `subagent_stop`, a failing `run` included, writes `blocked`, and the loop guard counts them by entry, event, actor,
  session and whole hash.
  A once-only `inject`, outside `session_start`, is skipped while not due and writes `injected` when it injects;
  `inject: { rules }` does the same per rule id.
  `session_start` writes `started` and `subagent_stop` writes `finished` without hash for any subagent when the
  workflow has a once-only inject, so the inject becomes due again.
  Compute the hash only for `passed`, the repeat of `run`, a `once` block, the loop guard and `finished`.
  Log each skip as a `skip` decision of the entry.
  When the log cannot be read, act every time, with the warning of the loop guard.
  Update the use case documents with the repeat rules, the scope, the skip and the observed lines, and the note that
  a placeholder that changes on every event makes a once-only inject act every time.
- Tests: `internal/state/state_test.go` and `state_entry_test.go`: the validity of each outcome and the query by each
  field.
  `internal/hook/answer_hook_logic_test.go`: AC8, AC14, AC15, AC18, AC19 and AC20, among them a second `turn_stop`
  on the same content that runs nothing, a run with `always: true` that writes `passed` every time, a run that
  rewrites files and writes no `passed`, a reminder with `once: true` that blocks once and lets the second commit
  through, the same reminder not repeated after an edit outside its scope and repeated after one inside it, the same
  reminder blocking a subagent and a new session on their own, a plain `block` blocking both commits without writing
  `blocked`, an inject with `{path}` once per path, an inject with `{changed_lines}` injected again when the measure
  changes, an inject due again after `session_start`, an inject reaching a subagent again after it finished, and an
  `inject` that computes no hash.
- Verification: `just test`, expected: every test passes.

### Task 4: Migrate the workflow of this repository

- Covers: AC11.
- Depends on: Task 1, Task 2, Task 3 (the installed binary must hold the whole engine).
- Files: `polix.yaml` (modify).
- Rules: GO-2.
- Reuse: none.
- Changes: Install the new binary with `just install` while polix.yaml still has the old names: the installed polix
  treats it as invalid, which still lets the main session edit polix.yaml and run polix commands; rewrite polix.yaml
  right after, since every other checkout on the old vocabulary stays blocked until it gets this commit.
  The `post_edit` format entry drops `mode: warn` and keeps its `run`, whose failure now reaches the AI as context.
  The `stop` entries become `turn_stop`; the lint and test entries keep only `when` and their `run`, in their order.
  The docs reminder becomes a `block` with `once: true` after the review gate, as today; its `changed` does not name
  README.md nor docs/, so the docs edit it asks for does not repeat it, and the `changed` of the review gate names
  only Go files, so the same edit does not invalidate the review.
  The review gate becomes a plain `block` with `unless: { finished: polix:reviewer }`; the `subagent_stop` entry goes,
  since polix observes the reviewer by itself.
  The two `why` become comments, and the comments on the evaluation and the facts describe the repeat rules, the
  observed lines and the single stop rule.
- Tests: none; `polix check` validates the file.
- Verification: `just check`, expected: `polix check`, `polix rules check` and `polix doctor --offline` pass.

### Task 5: Docs, use cases and the plugin in the new vocabulary

- Covers: AC12.
- Depends on: Task 1 (it rewrites the snippets of `docs/use-cases.md` this task describes); it shares no file with
  Task 2 and Task 3 and may run at the same time.
- Files: `README.md` (modify), `docs/polix.md` (sections `polix.yaml`, `Measures of the change`, `How the entries of an
  event are evaluated`, `Loop guard`, `State`, modify), `docs/use-cases.md` (modify), `docs/architecture.md` (modify),
  `plugins/polix/skills/workflow/SKILL.md` (modify), `plugins/polix/agents/workflow-author.md` (modify),
  `plugins/polix/agents/rules-reviewer.md` and `plugins/polix/agents/auditor.md` (modify only if they quote the old
  vocabulary).
- Rules: GO-2, DOC-1.
- Reuse: none.
- Changes: Replace every snippet and mention of the old vocabulary and of the modes.
  In `docs/use-cases.md`, rewrite the prose of the cases whose snippets changed in Task 1, remove the sections "Try a
  guardrail before enforcing it" and "A record of every infrastructure command", and update the query of "Evidence of
  the process each change followed" to the observed lines.
  In `docs/polix.md`, rewrite the schema, the lists of events, conditions and actions, and the measures; describe that
  a `block` holds every time and `once: true` makes a reminder, and the repeat of `run` and `inject` with `always`;
  describe what a failing `run` does on each event; describe `finished`, the scope of an entry that `finished` and
  `once` compare, the observed lines, what polix writes and when, and why commands of the AI never count; replace the
  stop rules with the single one and its trade-off (one failure per answer, the order of the file as priority); show
  an exception with `not` in `when` and warn against `contains` inside `any`; rewrite the walk-through of the
  `turn_stop` of this repository; replace the kinds of the section on the state with `decision` and `observed`; add
  a note that the old names are rejected with their replacement.
  In README.md, the first example drops its `record` and `done`, the harness example uses `finished`, and the example
  with `mode: warn` is rewritten without it.
  In the skill and the agent that write polix.yaml, teach the new vocabulary, `once: true` only for reminders, and
  `inject` for a warning.
- Tests: none; `plugins/plugins_test.go` already checks the embedded plugin.
- Verification: `rg -n "deny:|require:|allow:|record:|done:|context:|why:|mode:|^  stop:|at_least|- if:" README.md
  docs plugins examples`, expected: no match outside the hint notes (polix.yaml is Task 4's); `claude plugin validate
  plugins/polix`, expected: valid.

## Risks

- A reminder written without `once: true` blocks every commit until its `unless` holds: the message says what to do,
  and the docs show `once: true` next to every reminder.
- Dropping `audit` removes the way to try a gate without enforcing it: the gate is tried in a branch, and the decision
  lines of the log show every block once enforced.
- The global binary locks every checkout still on the old vocabulary, and `requires` cannot help since development
  builds skip it: Tasks 1 to 3 never install it, Task 4 installs and migrates in one go, and the other worktrees are
  rebased on the migration commit, or have their polix.yaml rewritten by the main session, right after.
- A scope too narrow lets a review survive an edit it should have seen (a `changed` of `*.go` ignores go.mod unless
  named): the docs tell to name in `changed` every file the subagent reviews.
- A tree pruned by `git gc` asks for a review again: it happens only to trees older than the prune window, and fails
  on the safe side.
- A command whose result depends on something outside the files (network, clock) is skipped after passing once:
  `always: true`, documented next to the repeat rule.
- More turns on `turn_stop` when several gates fail: the repeat rule reruns only what did not pass.
- A subagent of the same type launched twice in a session gets a once-only inject again only because the first one
  finished; two of them running at once share it: documented as a limit.
- `passed` lines for every green run of polix grow the log: one line per run, the same as a decision; the growth of
  the log stays a limit of docs/polix.md.
- A fixture missed in a test still uses an old name: Task 1 lists every package with fixtures and finds them with `rg`.

## Verification

- `just test`: every test passes.
- `golangci-lint run ./...`: no findings.
- `just check`: `polix check`, `polix rules check` and `polix doctor --offline` pass on the migrated polix.yaml.
- `claude plugin validate plugins/polix`: valid.
- End to end in a new session of this repository: make lint and tests fail, end the turn; the answer holds only the
  lint failure; fix it and end the turn; it holds only the test failure; fix it, end the turn twice, and
  `polix log --tail 5` shows the second turn skipping both runs and the `passed` lines of the first.
- End to end on a large Go change: the review gate blocks every commit until polix:reviewer ends; the docs reminder
  blocks the next commit once; edit README.md; the next commit goes through with neither a new review nor a new
  reminder; edit a Go file and the review gate blocks again.
- End to end: run `go test ./...` from the AI and end the turn; the `turn_stop` still runs the tests.
- End to end with a scratch polix.yaml that uses `deny`, `allow`, `mode`, `record` and `stop`: `polix check` names
  each one with its line and its new spelling.
