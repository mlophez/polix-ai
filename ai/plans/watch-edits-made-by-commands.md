# Find the files a shell command changed, restore the protected ones and evaluate the others as edits

## Overview

Today polix only sees an edit when the agent uses its edit tool.
When the agent changes a file from the shell, polix tries to guess it from the text of the command line.
That guess catches the usual file commands and redirections, but never a Python script, a nested shell or a search
that deletes what it finds.

With this change polix stops guessing and looks at what actually happened.
Right before a command runs, it records a snapshot of the working tree; right after, it compares.
Each file the command changed is then handled as if the agent had edited it with its edit tool:

- A file the workflow protects is put back as it was before the command, and the agent is told.
- Any other file goes through the edit entries of the workflow: it is formatted, and the rules of its path reach the
  agent.
  As the edit already happened, a block of those entries reaches the agent as a notice instead of stopping it.

The snapshot costs a pass of git over the working tree, so it is only taken when the workflow has something to apply
to an edit.
A command that moves the branch (a pull, a checkout, a reset) is left alone, and a command that changes more than 25
files only gets a notice, so a dependency install or a code generator does not flood the session.
The current check of the command line stays: it still stops the obvious cases before they run.

## Context

`pre_edit`, `post_edit` and `guardrails.protected_files` only see the edits of the edit tool and what `lineWrites`
recognises in a command line, a heuristic that misses interpreters, wrapped shells and computed paths
(docs/polix.md, Protections).
The content hash is already a git tree of the whole working tree, and `TreeDiff` already lists the paths that differ
between two of them, so the effect of a command can be measured exactly instead of guessed.
The outcome: every file a command changes is either restored, when protected, or evaluated as an edit.

## Decisions

- Scope: only the detection by effect; the extension of the static heuristic is a separate plan (see `Out of scope`).
- A protected file a command changed is restored from the snapshot, or removed when the snapshot did not hold it.
  The post_command then answers with a refusal naming it, which Claude Code shows to the agent after the tool ran.
- Each other changed path becomes a `pre_edit` and then a `post_edit` event of the same call.
  Their entries run as on an edit, but a `block` and a failing `run` reach the agent as context, logged as `inform`.
  The protections of an edit are not applied again to them: post_command already applied `protected_files`, and the
  state of polix lives in the git directory, which no tree holds.
- The snapshot is taken only with a valid `polix.yaml` that has `guardrails.protected_files` or at least one entry on
  `pre_edit` or `post_edit` (the expanded `format_on_edit` included), and only for a `pre_command` that no protection
  denied.
- The snapshot is an observed log line with the new outcome `snapshot`, the content hash, the HEAD commit and a key.
  The key is the id Claude Code gives the call of the tool (`tool_use_id`), the same on PreToolUse and PostToolUse,
  so two commands running at once pair with their own snapshot.
- Assumption: Codex gives no such id that polix knows, so without one the key is the command line.
- A command after which HEAD points to another commit than in its snapshot (pull, checkout, reset, rebase, commit) is
  not evaluated: the changes are git's, and restoring a protected file would undo a merge or a checkout.
  It is logged as a `skip` decision of action `builtin`.
- Assumption: past 25 changed paths, none becomes an edit event; the agent gets one context naming the count, and the
  decision is logged as `inform` of action `builtin`.
  The restore of protected files is not bounded.
- A post_command also reads the snapshot after PostToolUseFailure, since a failing command may have written files.
- A missing snapshot, an unknown hash or two trees that cannot be compared never block: the post_command answers as
  today, and a comparison that fails adds a warning for the user.
- The restore is a new step that acts, carried out by the shell through a new method of the port, and it counts as a
  run for the epoch, since it rewrites files.

## Approach

Domain.
`agent.Event` gains `ToolUse`, the id the agent gives the call of a tool, and `ByCommand`, set on an edit event that
polix derived from the effect of a command.
`state` gains the outcome `OutcomeSnapshot`, listed in `state.Outcomes` and among the hashed outcomes, so `Valid`
requires its hash; it carries the HEAD commit as the lines read by branch do.
`hook.Step` gains `Restore`, the paths to put back, with `Source`, the tree to take them from; `Valid` requires both
together, and `asks` counts `Restore` as one more thing a step may ask.

Platform.
`internal/platform/git` gains `Repo.Restore`: the paths the tree holds are written to the working tree from it,
without touching the real index; the paths it does not hold are removed from the working tree.

Logic of AnswerHook.
A new engine file, `logic_watch.go`, holds what the command family calls.
On `pre_command`, after the protections, `snapshot` writes the observed `snapshot` line when the workflow watches
(see Decisions), with the content hash of the current epoch.
On `post_command`, `watch` looks up the latest `snapshot` line of the session, the actor and the key with
`state.State.Latest`, asks for the current hash and the paths that differ between both trees (the existing `Compare`
step), compares HEAD, and splits the paths into the protected ones (`anyPathFold` on `ProtectedFiles`) and the rest.
The protected ones go to one `Restore` step, then to one refusal and one `deny` decision of
`guardrails.protected_files` per path; the rest, up to 25, go to `Output.Follow` as `pre_edit` and `post_edit` events
with `ByCommand`, `Path` and an absolute `AbsPath` under the root.
`Output` gains `Follow`, the events the shell evaluates next in the same call.
`Decide` sets a new field of the pass, `byCommand`, for an event with `ByCommand`, and also `informed` for it.
`edit` skips `protectEdit` for such an event, and the verb `block` honours `byCommand` by informing instead of
refusing, logged as `inform`, as a failing `run` already does through `informed`.

Shell of AnswerHook.
`run` evaluates its events from a queue instead of the fixed list: the `Follow` of each output is appended to it, and
every event goes through the same `Locate`, `Decide` and `carry` loop, its answer merged into the answer of the call.
`carry` routes a `Restore` step to `act`, which calls the new `Port.Restore` and notes its outcome.
The Claude format reads `tool_use_id` from the input into `ToolUse`; the Codex format leaves it empty.
`Adapter.Restore` wraps `Repo.Restore`.

Error handling.
A hash that cannot be computed on `pre_command` writes no snapshot, with the existing warning of an unknown hash.
No snapshot on `post_command` means nothing to compare.
A comparison that fails (a tree pruned by git gc, git failing) adds a warning for the user and evaluates nothing.
A restore that fails refuses with the path and the error, logs the `deny` with the error, and warns the user to put
the file back by hand.

### Discarded alternatives

- Extending only the parser of the command line: a script in a file or an interpreter can never be read from it.
- A file system watcher during the command: the hooks are one-shot processes, so it needs a daemon.
- `git status` with modification times instead of a tree: it cannot restore a content and misses a further change to a
  file already changed.
- Restoring with `git checkout <tree> -- <path>`: it also writes the real index of the person.
- Pairing only by the command line: two identical commands running at once would share one snapshot.

## Acceptance criteria

- AC1: With `protected_files` or an entry on `pre_edit` or `post_edit`, a `pre_command` that no protection denies writes
  one observed `snapshot` line with the content hash, the HEAD commit and the key; without any of them, or for a denied
  command, no hash is computed and no line is written.
- AC2: A command that changes a file of `protected_files` git does not ignore, as `python3 -c` appending to `AGENTS.md`
  or `bash -c 'echo x > CLAUDE.md'`, finds it restored after it runs, or removed when it did not exist before; the
  post_command answers with a refusal naming the path, logged as `deny` of `guardrails.protected_files`.
- AC3: For each other path a command changed, the same call evaluates a `pre_edit` and then a `post_edit` of that path:
  `format_on_edit` runs on it and an `inject: { rules: "{path}" }` of `pre_edit` reaches the agent as context.
- AC4: On those derived events a `block` and a failing `run` reach the agent as context and are logged as `inform`;
  they never refuse.
- AC5: When HEAD moved between the snapshot and the post_command, nothing is restored nor evaluated, and a `skip`
  decision of action `builtin` is logged.
- AC6: Past 25 changed paths, no edit event is derived, the agent gets one context with the count, and the decision is
  logged as `inform` of action `builtin`; protected paths among them are still restored.
- AC7: Without a snapshot, with an unknown hash or with trees that cannot be compared, the post_command answers as it
  does today, plus a warning for the user when the comparison failed.
- AC8: A restore that fails refuses with the path and the error and warns the user to put the file back by hand.
- AC9: A Claude Code call pairs its snapshot by `tool_use_id`; without it the command line is the key.
- AC10: The check of the command line before it runs is unchanged: `rm AGENTS.md` is still denied on `pre_command`.
- AC11: docs/polix.md, the document of AnswerHook and CHANGELOG.md describe the detection, its cost, its skips and its
  limits.

## Out of scope

- Extending the static heuristic of `lineWrites` and `parseCommand`: unwrapping `bash -c`, `sh -c` and `eval`, reading
  `find -delete` and `find -exec`, following `cd` within a line, skipping wrappers with a value, and spotting literal
  paths written by `python -c`.
  It is its own plan, independent of this one, to prevent rather than repair.
- Files git ignores: no tree holds them, so a protected file that git ignores is still only guarded by the check of the
  command line.
- The id of a tool call of Codex, whose input polix has never seen from a running Codex.
- The edits of the edit tool, which keep their current flow.

## Tasks

### Task 1: Domain values for snapshots, derived edits and the restore step

- Covers: none.
- Depends on: none.
- Files: `internal/app/agent/agent_event.go` (`Event`, modify), `internal/app/agent/agent_event_test.go` (modify),
  `internal/app/state/state_entry.go` (`OutcomeSnapshot`, `Outcomes`, `hashed`, modify),
  `internal/app/state/state_entry_test.go` (modify), `internal/app/hook/hook_step.go` (`Step`, `Valid`, `asks`,
  modify), `internal/app/hook/hook_step_test.go` (modify).
- Rules: GOARCH-1, GOARCH-2, GOARCH-4, GOARCH-9, GOARCH-13, GOARCH-16, GOARCH-18, GOARCH-25, GOARCH-26, GOARCH-27,
  GOARCH-28, GOARCH-29, GOARCH-30, GOARCH-35, GOARCH-38, GOARCH-39, GOARCH-41, GOARCH-44, GO-2, GO-3, GO-4, XPLAT-1.
- Reuse: none.
- Changes: `Event` gains `ToolUse` and `ByCommand`, documented as in `Approach`; `Valid` rejects `ByCommand` on an
  event that is not an edit or that has no `Path`.
  `state` gains `OutcomeSnapshot` (`snapshot`), its comment saying what it records and that `Key` is the tool-use id
  or the command line; it joins `Outcomes` and the hashed outcomes.
  `Step` gains `Restore` and `Source`; `Valid` reports a restore without its source or a source without paths, and
  `asks` counts `Restore`.
- Tests: an edit event with `ByCommand` and a path is valid, a command event with `ByCommand` is not; a `snapshot`
  entry without hash is rejected and one with hash accepted; a step with `Restore` and `Source` is one step, one with
  only one of them is invalid, and one with `Restore` and `Run` asks for two things.
- Verification: `just test -run 'TestEvent|TestEntry|TestStep'`, expected: pass.

### Task 2: Restore paths of the working tree from a tree

- Covers: AC2, AC8.
- Depends on: none.
- Files: `internal/platform/git/git_tree.go` (`Repo.Restore`, modify),
  `internal/platform/git/git_tree_test.go` (create).
- Rules: GOARCH-1, GOARCH-7, GOARCH-13, GOARCH-17, GOARCH-19, GOARCH-35, GOARCH-38, GOARCH-39, GOARCH-42, GO-2, GO-3,
  GO-4, XPLAT-1.
- Reuse: `Repo.git`, `Repo.paths` and `splitPaths` from `internal/platform/git`.
- Changes: `Repo.Restore` takes a tree and paths relative to the root with slashes.
  It lists which of them the tree holds, writes those to the working tree from the tree without touching the real
  index, and removes the others from the working tree, a missing file being no error.
  It returns the first error, naming the path, and never leaves the real index changed.
- Tests: against a temporary repository, a file changed after `ContentHash` comes back with its content and mode, a
  file created after it is removed, a file in a folder the command removed comes back with its folder, the real index
  is byte for byte the same, and a tree git does not have is an error.
- Verification: `just test -run TestRestore`, expected: pass on the three systems of CI.

### Task 3: Snapshot on pre_command, comparison and restore on post_command

- Covers: AC1, AC2, AC5, AC6, AC7, AC8, AC10.
- Depends on: Task 1.
- Files: `internal/app/hook/answer/logic_watch.go` (create), `internal/app/hook/answer/logic_watch_test.go` (create),
  `internal/app/hook/answer/logic_command.go` (`command`, modify), `internal/app/hook/answer/logic.go` (`Output`,
  modify), `internal/app/hook/answer/logic_log.go` (`readByHead`, modify), `internal/app/hook/answer/logic_facts.go`
  (modify, only if a reader is missing), `internal/app/hook/answer/logic_test.go` (modify).
- Rules: GOARCH-1, GOARCH-3, GOARCH-4, GOARCH-9, GOARCH-10, GOARCH-13, GOARCH-18, GOARCH-22, GOARCH-23, GOARCH-25,
  GOARCH-28, GOARCH-29, GOARCH-34, GOARCH-35, GOARCH-38, GOARCH-39, GOARCH-41, GO-2, GO-3, GO-4, XPLAT-1, DOC-1.
- Reuse: `pass.hash`, `pass.head`, `pass.currentState`, `pass.sameScope`'s `Compare` step through
  `hook.Facts.TreeDiff`, `pass.write`, `pass.deny`, `pass.guardrails`, `anyPathFold`, `state.State.Latest`,
  `pass.act`.
- Changes: `logic_watch.go` holds `watches` (the workflow has `ProtectedFiles` or entries on `pre_edit` or `post_edit`),
  `snapshot` and `watch`, as described in `Approach`, and the key of an event (`ToolUse`, else the command line).
  `command` calls `snapshot` on `pre_command` once the protections passed, and `watch` first on `post_command`, the
  entries of `post_command` still running after it.
  `watch` restores the protected paths with one `Restore` step, bumps the epoch after it, and refuses with one reason
  per path, `<path> is protected in polix.yaml: a command changed it and polix put it back.`, each logged as `deny`
  of `guardrails.protected_files`; a failed restore uses the reason of AC8 and adds the warning.
  `readByHead` also holds for `snapshot`, so the line carries HEAD.
  `Output` gains `Follow`, left empty by this task.
- Tests: in `logic_watch_test.go` and `logic_test.go`, with domain values only: no snapshot without a watching
  workflow, with an invalid one or on a denied command; a snapshot line with hash, HEAD and key otherwise; a
  post_command with a changed protected path asks for the restore with the snapshot tree, then refuses and logs;
  with the restore failing, the refusal and the warning; HEAD moved, a skip and nothing else; no snapshot, no compare
  step; an unknown hash or a failed comparison, no refusal and a warning for the latter; two snapshots of different
  keys pair with their own post_command; `rm AGENTS.md` still denied on `pre_command`.
- Verification: `just test -run 'TestDecide|TestWatch'`, expected: pass.

### Task 4: Evaluate the other changed paths as edits made by a command

- Covers: AC3, AC4, AC6.
- Depends on: Task 3.
- Files: `internal/app/hook/answer/logic_watch.go` (`watch`, modify), `internal/app/hook/answer/logic_watch_test.go`
  (modify), `internal/app/hook/answer/logic.go` (`Decide`, `pass`, modify), `internal/app/hook/answer/logic_edit.go`
  (`edit`, modify), `internal/app/hook/answer/logic_entries.go` (`apply`, modify),
  `internal/app/hook/answer/logic_test.go` (modify).
- Rules: GOARCH-1, GOARCH-3, GOARCH-4, GOARCH-9, GOARCH-13, GOARCH-18, GOARCH-23, GOARCH-25, GOARCH-28, GOARCH-29,
  GOARCH-34, GOARCH-35, GOARCH-38, GOARCH-39, GOARCH-41, GO-2, GO-3, GO-4, XPLAT-1, DOC-1.
- Reuse: `agent.Event.Locate`, `absolute` from `logic_writes.go`, `pass.inject`, `pass.log`.
- Changes: `watch` fills `Output.Follow` with a `pre_edit` and then a `post_edit` per changed path that is not
  protected, in the order of the tree diff, each with `ByCommand`, the session, the actor, the agent, the cwd and the
  path located inside the root; past 25 paths it fills none, informs the agent with the count and logs the `inform`.
  `Decide` sets `byCommand` and `informed` on a pass whose event has `ByCommand`.
  `edit` skips `protectEdit` for such an event and keeps the skip of `outsideEdit`.
  `apply` turns a `block` into context on a pass with `byCommand`, logged as `inform`, so the evaluation goes on; the
  loop guard and `once` are unchanged.
- Tests: a post_command with two changed paths returns four follow events in order; 26 paths return none and one
  context; a derived `pre_edit` gets its `inject: { rules }` as context and is not denied for a protected glob; a
  derived `pre_edit` whose entry blocks answers with context and logs `inform`; a derived `post_edit` runs
  `format_on_edit` and a failure is context; an edit of the edit tool still refuses on the same entry.
- Verification: `just test -run 'TestDecide|TestWatch'`, expected: pass.

### Task 5: Shell, adapter and Claude input for the restore and the follow events

- Covers: AC2, AC3, AC9.
- Depends on: Task 1, Task 2, Task 4.
- Files: `internal/app/hook/answer/in.go` (`Port`, `run`, `carry`, `act`, `hookInput`, `hookInput.event`, modify),
  `internal/app/hook/answer/in_input.go` (create, only if in.go would pass 400 lines: the methods of `hookInput` move
  there), `internal/app/hook/answer/in_claude.go` (modify), `internal/app/hook/answer/out.go` (`Adapter.Restore`,
  modify), `internal/app/hook/answer/in_test.go` (modify), `internal/app/hook/answer/in_claude_test.go` (modify),
  `internal/app/hook/answer/out_test.go` (modify), `internal/app/hook/answer/answer.md` (`## Request`, `## Port`,
  `## Flow`, modify).
- Rules: GOARCH-1, GOARCH-3, GOARCH-5, GOARCH-12, GOARCH-13, GOARCH-15, GOARCH-16, GOARCH-20, GOARCH-21, GOARCH-22,
  GOARCH-24, GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, GO-2, GO-3, GO-4,
  XPLAT-1, DOC-1.
- Reuse: `Repo.Restore` from `internal/platform/git`, `hook.Evaluation.Note`, `agent.Answer.Merge`.
- Changes: `Port` gains `Restore`, taking the source tree and the paths and returning an error; `Adapter.Restore` wraps
  `Repo.Restore`.
  `carry` leaves a `Restore` step to `act`, which notes its outcome with the error.
  `run` keeps a queue of events, starting from those of the hook, and appends the `Follow` of each output to it; the
  conditions stay within GOARCH-37 (lengths compared with zero, fields of the output).
  `hookInput` reads `tool_use_id`, and `event` copies it into `ToolUse`; the Codex format passes none.
  answer.md names `Restore` in `## Port`, the tool-use id in `## Request` and the queue in `## Flow`.
- Tests: in `in_test.go`, with the fake of the port, a post_command whose output follows two events evaluates them in
  the same call and merges their contexts; a `Restore` step reaches the fake with its tree and paths, and its error
  reaches the logic.
  In `in_claude_test.go`, `tool_use_id` lands in `ToolUse` on PreToolUse, PostToolUse and PostToolUseFailure.
  In `out_test.go`, `Adapter.Restore` puts a protected file back in a temporary repository.
- Verification: `just test`, expected: pass, the architecture tests included.

### Task 6: Document the detection of the edits made by commands

- Covers: AC11.
- Depends on: Task 5.
- Files: `docs/polix.md` (How a hook call is answered, Protections, What polix observes, State, Known limits, modify),
  `internal/app/hook/answer/answer.md` (`## Logic`, `## Errors`, modify), `CHANGELOG.md` (Unreleased, modify).
- Rules: GO-2, GOARCH-14, GOARCH-21, DOC-1.
- Reuse: none.
- Changes: docs/polix.md explains when the snapshot is taken and what it costs, the pairing by the tool-use id, the
  restore of protected files, the derived `pre_edit` and `post_edit` and why their blocks only inform, the skips (HEAD
  moved, past 25 paths, no snapshot) and the new observed outcome `snapshot`.
  The list of what the heuristic does not see says that those commands are now caught after they run when the workflow
  watches, and Known limits adds files git ignores, commands running at once whose changes mix, and Codex pairing by
  the command line.
  answer.md adds the watch to the rules of `## Logic` and the failures to `## Errors`.
  CHANGELOG.md adds an entry under `### Added`.
- Tests: none; `polix rules check` and the architecture tests check the sections of answer.md.
- Verification: `just check`, expected: no problem reported.

## Risks

- Latency of every command: two content hashes per command in a watching repository; mitigated by watching only when
  the workflow applies something to edits, and measured in the end to end check below.
- Commands running at once in the same session mix their changes, so an edit may be attributed to the wrong command;
  restoring stays correct because every snapshot holds the file before any of them, and the limit is documented.
- A generator or an install writing files git does not ignore floods the session; bounded by the 25 paths.
- A person editing a protected file while the agent's command runs could see it put back; the window is the length of
  the command, and the refusal names the file.
- in.go is at 375 lines and GO-4 caps it at 400; Task 5 moves the methods of `hookInput` to `in_input.go` if needed.
- `git gc` pruning the loose tree of a snapshot: only after two weeks by default, far beyond the life of a command; a
  pruned tree is a failed comparison (AC7).

## Verification

- `just test`: pass.
- `golangci-lint run ./...`: no finding.
- `just check`: no problem from `polix config check`, `polix rules check` and `polix doctor --offline`.
- `claude plugin validate plugins/polix`: valid.
- End to end, after `just install`, in a scratch repository with `protected_files: [AGENTS.md]`, a `format_on_edit` on
  `*.go` and a `pre_edit` entry `inject: { rules: "{path}" }`:
  1. Ask Claude Code to run `python3 -c "open('AGENTS.md','a').write('x')"`: AGENTS.md is back to its content and the
     agent gets the refusal naming it.
  2. Ask it to run a `bash -c` that writes a badly formatted Go file: the file ends formatted and the rules of its path
     reach the agent.
  3. Run `git pull` or `git checkout` of another branch: nothing is restored and `polix log` shows the `skip`.
  4. In `polix log`, compare the times of the `pre_command` and `post_command` decisions of one command with and
     without a workflow that watches.
