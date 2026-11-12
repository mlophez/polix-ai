# Split the state log of polix into one JSON Lines file per session

- Status: built
- Risk: high: it changes where the state lives and what each hook reads, takes a file name from the agent's input,
  and narrows `finished` and the repeat of a `run` to the session.
- Reference: none.
- Created: 2026-11-12.
- Size: 3 tasks, about 750 changed lines.

## Overview

Today polix keeps everything it observes and decides in one log per repository.
That log grows forever, and every hook reads it whole.

After this change each session of the agent has its own file, with its subagents in it.
A hook reads and writes only the file of its own session.
What happened in another session no longer counts: a new session runs the checks again and asks again for the review,
even on the same content.
To keep the state of earlier work, the user resumes that session (`claude --resume`) instead of opening a new one.

An event that comes without a session id records nothing, and polix warns about it.
The old single log is left on disk and ignored by everyone.

`polix log` now shows the log grouped by session, the most recently active last.
By default it shows the last 5 sessions, each one whole.
`-n` now counts sessions, and `--json` prints only the raw lines.

## Request

I want to split the jsonl files by session.

- (On how `polix log` orders the entries) Grouped by session and by default the last 5.
- (On where the entries without a session id go) There cannot be entries without an id.
- I don't want the shared.jsonl.
- (On how `finished` and the repeat of a `run` see other sessions) The thing is, if the reviewer finished in one
  Claude session, when another session is opened it runs again for the same content. It only looks at its own
  session. If you want to continue the previous work, do a claude --resume, right?

## Context

`AnswerHook` appends every line to `<git-dir>/polix/log.jsonl` (`Adapter.Append` and `Adapter.logPath` in
`internal/app/hook/answer/out_log.go`).
`Adapter.LoadState` loads that whole file through the loader of `internal/app/state/load` on every event that needs
the state, and the queries of `state.State` then filter by session, except two.
The condition `finished` (`logic_condition.go`) and the repeat of a `run` (`passed` in `logic_run.go`) select lines
of any session by branch or HEAD commit.
`polix log` (`internal/app/workflow/log`) reads the same file and prints its last 20 entries in file order.

## Constraints

- The state stays under `<git-dir>/polix/` (`state.DirName`), so the protection of the state
  (`logic_protect.go`, `hook.Evaluation.StateDir`) keeps covering it unchanged: source the code.
- Each line is still written with one append call (`jsonl.Append`), with no lock: the main session and its parallel
  subagents append to the same file of their session, as they append to one log today: source the code.
- The format of a line does not change (`logLine` of `out_log.go`, `entryJSON` of `internal/app/state/load/out.go`),
  and `state.MaxLine` and `state.MaxReason` keep their values: source the code and docs/polix.md.
- The domain declares no function that is not a method (GOARCH-44), so the name of the file of a session is a method
  of a domain type, not a free function: source GOARCH-44.
- The domain and the logic stay pure (GOARCH-4, GOARCH-18, GOARCH-41): the files are listed and read in the
  adapters; `crypto/sha256` and `time.Parse` are allowed in them.
- Paths go through `filepath`, and the tests run on Linux, macOS and Windows: source XPLAT-1.
- A change of behaviour updates docs/polix.md, and the use case documents in the same change: source DOC-1, GOARCH-21
  and GOARCH-36.
- Tests and the programs they run write only to temporary directories: source the plan rules of the project.

## Decisions

- One file per session, at `<git-dir>/polix/sessions/<name>.jsonl`, holding every line of the session, its subagents
  included, which carry the session id of the main session: answer of the user.
- No shared file: a hook reads only the file of its own session, for every verb and condition, `finished` and the
  repeat of a `run` included; a new session starts from an empty state, and `claude --resume` keeps the session: free
  answer of the user.
- The name of the file is the session id as is when it is made only of `A-Z`, `a-z`, `0-9`, `.`, `_` and `-`, at most
  128 characters long, and not only dots; otherwise the lowercase hex sha256 of the id: answer of the user (the
  "not only dots" part keeps the promise of no path traversal the user picked).
- An entry without a session is never written: `state.Entry.Valid` rejects it, the hook goes on and warns with the
  cause, and reading the state for an event without a session fails as a log that cannot be read does: answer of
  the user.
- The old `<git-dir>/polix/log.jsonl` is ignored by the hook and by `polix log`, and left on disk: answers of the user.
- `polix log` groups the entries by session, each session in file order, under a header `== session <id> ==`; the
  sessions are ordered by their last activity, the most recent last: answers of the user (the header is the preview
  the user picked).
- `-n`/`--tail` counts sessions, 5 by default, 0 for all, each session shown whole: answer of the user.
- `polix log --json` prints only the JSON lines of the same sessions in the same order, without headers: answer of the
  user.
- The rule that names the file of a session lives in the domain `internal/app/state`: answer of the user, as a method
  of a named type by GOARCH-44.
- Retention of old session files is out of scope: answer of the user.

## Approach

Domain `internal/app/state`:

- `LogName` gives way to `SessionsDir` (`sessions`), the folder of the session files inside `DirName`.
- A new named string type `Session`, the id of a session, with a method `File` that returns the base name of its
  file (the name rule of `Decisions` plus the extension `.jsonl`), or an error for an empty id.
- `Entry.Valid` also rejects an entry without `Session`.
- The package comment of `state.go` describes the files of the sessions instead of `log.jsonl`.

Hook (`internal/app/hook/answer`):

- The method `LoadState` of `Port` takes the session id; the shell passes the session of the event of the evaluation.
- `Adapter.LoadState` resolves the file of that session with `state.Session.File` and loads it with the loader of the
  log; an empty session returns the error of `File`, which the evaluation treats as a log that cannot be read.
- `Adapter.Append` writes each entry to the file of its own `Session`, after `Entry.Valid`.
- No query of `state.State` changes: they now run on the entries of one session only.

`polix log` (`internal/app/workflow/log`):

- `Port.ReadLog` returns one `state.State` per session file of the sessions folder, read through the loader; a missing
  folder means no sessions; files without the `.jsonl` extension are left out; a file that cannot be read is the
  error of the command, as today.
- `Decide` takes those states and the count of sessions, orders the sessions by the latest `at` of their entries
  (parsed as RFC 3339, so the offsets of the clock compare right; a session with no parseable `at` sorts first; ties
  by session id) and keeps the last `tail`, or all with 0 or less.
- `Response` holds the sessions to show, each with its id (the `session` of its first entry) and its entries, and the
  sum of the skipped lines of every file.
- The text output prints `== session <id> ==` before the entries of each session, in the format of an entry of today;
  `--json` prints only the lines.

### Discarded alternatives

- A shared file with copies of `finished` and `passed`: the user does not want it.
- Reading the files of the other sessions on demand for `finished` and `passed`: the user wants each session to look
  only at itself.
- Always the sha256 as file name: the session would not be recognizable in the folder.
- Migrating, or reading as legacy, the old `log.jsonl`: the user chose to ignore it.
- A file per branch for the lines read across sessions: needs the shared file the user does not want.
- An embedded database (SQLite, bbolt): discussed before the plan; not needed for the size of the state.

## Acceptance criteria

- AC1: Every line polix writes for an event goes to `<git-dir>/polix/sessions/<name>.jsonl` of the session of the
  event, the lines of its subagents included, and nothing is written to `<git-dir>/polix/log.jsonl`.
- AC2: A hook reads only the file of its own session: a line of another session never counts for any verb or
  condition (`inject`, `notify`, `block` with `once`, the loop guard, the watch of the edits, `agents_are_running`).
- AC3: A `finished` line of a subagent type, or a `passed` line of a `run`, written in another session on the same
  branch and the same content, does not make `finished` hold nor skips the `run`; in the same session they still do.
- AC4: The name of the file of a session is its id as is when the id has only `A-Z`, `a-z`, `0-9`, `.`, `_` and `-`,
  has at most 128 characters and is not only dots; otherwise the lowercase hex sha256 of the id; both end in `.jsonl`.
  An id with `/` or `\` (such as `../x`), with a space, or of 129 characters is hashed, and so are `.`, `..` and
  `...`.
- AC5: An entry without a session is never written: `state.Entry.Valid` rejects it, the hook goes on and warns with
  the cause, and an event without a session reads no state: a `run` acts, a `block` with `once` blocks, and the hook
  warns as when the log cannot be read.
- AC6: Neither the hook nor `polix log` reads `<git-dir>/polix/log.jsonl`, and polix never removes it.
- AC7: `polix log` prints the entries grouped by session, each session under the header `== session <id> ==` with
  its entries in file order, the sessions ordered by their last activity with the most recent last.
- AC8: `polix log` shows the last 5 sessions by default, `polix log -n 1` the last session, `polix log -n 0` all of
  them, each session whole.
- AC9: `polix log --json` prints only the JSON lines of the same sessions in the same order, without headers.
- AC10: `polix log` warns once with the total of the lines over `state.MaxLine` skipped in every file, and with no
  sessions folder it prints nothing and exits 0.
- AC11: docs/polix.md, docs/use-cases.md, examples/polix.yaml, its copy in the plugin, README.md, CHANGELOG.md and the
  documents of the use cases describe the files of the sessions, that `finished` and the repeat of a `run` count only
  within the session, and the new `polix log`.

## Out of scope

- Removing or archiving old session files: the user left retention for later.
- Migrating, reading or deleting the old `log.jsonl`: the user chose to ignore it.
- A flag of `polix log` to pick one session by id: not asked.

## Starting point

- Read first: `internal/app/state/state.go`: the queries over the entries and the constants of the state folder.
- Read first: `internal/app/hook/answer/out_log.go`: how the hook reads and writes the log.
- Read first: `internal/app/workflow/log/in.go` and `logic.go`: the command `polix log` today.
- Read first: docs/polix.md, section `State` and section `How often each verb acts`.
- Baseline: `just test`, expected: passes, 46 packages ok and no failure.

## Progress

- [x] Task 1: Name the file of a session and reject entries without a session. Evidence: the new tests TestSessionFileAsIs, TestSessionFileHashed, TestSessionFileEmpty and TestEntryValidWithoutSession (and the case added to TestAppend) failed to build before the change (`undefined: Session`) and pass after it; the existing tests of state and of hook/answer, given a session where they lacked one, stay green; `just test -run 'Session|Valid|Append'` passes, as does the whole `go vet ./... && go test ./...` and `golangci-lint run` on state and hook.
- [x] Task 2: Read and write the state per session, in the hook and in polix log. Evidence: the new tests TestAppendToTheSessionFile, TestLoadStateOfTheSession, TestOldLogIgnored (out_log_test.go), TestRepeatAndFinishedWithinTheSession, TestAgentsAreRunningWithinTheSession (out_test.go), TestStateOfTheSession (in_test.go), TestDecideOrdersTheSessions, TestDecideUnknownTimesAndTies, TestShellPrintsSessions, TestCommandTailDefault, TestAdapterReadLog, TestAdapterReadLogUnreadable and TestCommandSessions (workflow/log) failed to build before the change (the new signatures of `LoadState` and `ReadLog`, `Output.IDs`, `Response.Sessions`) and pass after it; the kept tests of hook/answer and the protection of `sessions/<id>.jsonl` stay green; `just test` passes (46 packages ok) and `golangci-lint run ./...` gives 0 issues; the differential check (base from `git archive HEAD` against the working tree, examples/polix.yaml, SessionStart, PreToolUse Edit, Stop twice in s1, Stop in s2) gives the same stdout, stderr and exit code on every call but the path of the temporary repository in the output of `polix doctor`, the base skips `rules changed` in s2 while the new binary runs it again (AC3), the new binary writes only `sessions/s1.jsonl` and `sessions/s2.jsonl`, and `polix log`, `-n 0`, `-n 1` and `--json` show the sessions under their headers in order, the last one alone, and only the lines (AC7 to AC10).
- [x] Task 3: Document the state per session. Evidence: the kept `TestWorkflowReference` stays green with the copy of `examples/polix.yaml` in the plugin (`just test -run TestWorkflowReference` passes, the two files are identical); `rg -n "log\.jsonl" docs examples plugins README.md internal` leaves in the docs only lines that say the old log is ignored and left on disk, and in the tests only the protection of the old path, the checks that it is neither written nor read, and temporary files of the jsonl and load tests.

## Tasks

### Task 1: Name the file of a session and reject entries without a session

- Covers: AC4, AC5 (the rejection by `Entry.Valid` and the warning of the hook).
- High risk: input from outside, the session id the agent sends becomes a file name; the tests of the name rule
  cover traversal, separators, length and the empty id.
- Files: `internal/app/state/state.go` (`SessionsDir`, the type `Session` and its method `File`, the package
  comment, modify), `internal/app/state/state_entry.go` (`Entry.Valid`, modify), `internal/app/state/state_test.go`,
  `internal/app/state/state_entry_test.go` and every other test that appends an entry without a session through
  `State.Append`, `Entry.Valid` or the adapter of the hook (modify).
- Rules: GOARCH-2, GOARCH-4, GOARCH-13, GOARCH-18, GOARCH-26, GOARCH-27, GOARCH-41, GOARCH-44, GO-2, GO-4, XPLAT-1.
- Reuse: `crypto/sha256` and `encoding/hex` of the standard library; `state.Entry.Valid` as it is.
- Tests:
  - `new` in `state_test.go`: `Session.File` of a UUID returns the UUID with `.jsonl`; of an id of 128 allowed
    characters returns it as is; fails today because `Session` does not exist.
  - `new` in `state_test.go`: `Session.File` hashes `a/b`, `a\b`, `../x`, `a b`, `.`, `..`, `...` and an id of 129
    characters, returning 64 lowercase hex characters and `.jsonl`, the same for the same id and different for
    different ids; returns an error for the empty id.
  - `new` in `state_entry_test.go`: `Entry.Valid` of an otherwise valid entry without `Session` returns an error that
    names the session; fails today because it is accepted.
  - `kept` in `state_entry_test.go` and `state_test.go`: every valid entry with a session stays valid, and
    `State.Append` keeps its behaviour on them (the existing tests, given a session where they lacked one).
  - `kept` in `internal/app/hook/answer`: an event with a session behaves as before (the existing tests).
- Changes:
  - Add `SessionsDir` and the type `Session` with `File`, implementing the name rule of `Decisions`; keep `LogName`
    for now, since Task 2 removes its last users.
  - Make `Entry.Valid` reject an entry without `Session`, and add the case to its comment.
  - Give a session to the entries of the tests that lacked one; the tests that check a rejection keep checking it.
- Verification: `just test -run 'Session|Valid|Append'`, expected: passes.

### Task 2: Read and write the state per session, in the hook and in polix log

- Covers: AC1, AC2, AC3, AC5 (the event without a session reads no state), AC6, AC7, AC8, AC9, AC10.
- High risk: the location of the state (data); the command line of `polix log`, whose `-n` changes meaning and whose
  output changes (a contract); existing behaviour of `finished` and of the repeat of a `run` across sessions that
  changes; concurrent appends of subagents to the file of their session.
  The new tests below guard each one, and the differential check compares the base and the new binary.
- Files:
  - `internal/app/hook/answer/in.go` (`Port.LoadState` and the step that loads the state, modify).
  - `internal/app/hook/answer/out_log.go` (`Adapter.LoadState`, `Adapter.Append`, `Adapter.logPath`, modify).
  - `internal/app/hook/answer/answer.md` (the state of the session, `Port`, the conditions read by branch, modify).
  - `internal/app/hook/answer/logic_run.go`, `logic_condition.go`, `logic_entries.go`, `logic_log.go` (the comments
    that say a line counts on the current branch or the same HEAD commit now say also within the session, modify).
  - `internal/app/hook/answer/in_test.go`, `out_test.go`, `out_log_test.go`, `logic_test.go`,
    `logic_protect_test.go` (the fake port and the paths of the log, modify).
  - `internal/app/workflow/log/in.go` (`Port.ReadLog`, `Request.Tail`, `Response`, the flag `--tail`, `print`,
    modify), `logic.go` (`Decide`, `Output`, modify), `out.go` (`Adapter.ReadLog`, modify), `log.md` (modify), and
    their tests `in_test.go`, `logic_test.go`, `out_test.go` (modify).
  - `internal/app/state/state.go` (remove `LogName`, modify), `internal/app/state/load/load.md` and
    `internal/app/state/load/in.go` (the comments that name `log.jsonl`, modify).
- Rules: GOARCH-3, GOARCH-4, GOARCH-5, GOARCH-11, GOARCH-12, GOARCH-13, GOARCH-14, GOARCH-16, GOARCH-18, GOARCH-20,
  GOARCH-21, GOARCH-22, GOARCH-23, GOARCH-24, GOARCH-25, GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35, GOARCH-36,
  GOARCH-37, GOARCH-38, GOARCH-41, GOARCH-45, GO-2, GO-4, XPLAT-1, DOC-1.
- Reuse: `state.Session.File` and `state.SessionsDir` from Task 1; the loader `state.Load` of
  `internal/app/state/load`; `jsonl.Append` of `internal/platform/jsonl`; `testenv` for the temporary repositories.
- Tests:
  - `new` in `out_log_test.go`: `Adapter.Append` of an entry of session `s1` writes one line to
    `<git-dir>/polix/sessions/s1.jsonl` and creates no `log.jsonl`; fails today because it writes `log.jsonl`.
  - `new` in `out_log_test.go`: with lines of sessions `s1` and `s2` appended, `Adapter.LoadState` of `s1` returns only
    the entries of `s1`, in file order; of `s3` returns an empty state; of the empty session returns an error and an
    empty state.
  - `new` in `out_log_test.go`: a `log.jsonl` left in `<git-dir>/polix/` is not read by `LoadState` and is still on
    disk after appends.
  - `new` in `out_test.go` (end to end through the adapter of the hook on a temporary repository): a `passed` line of a
    `run` written in session `s1` skips the `run` of an event of `s1` on the same content and does not skip it for an
    event of `s2`; a `finished` line of a subagent type written in `s1` makes `finished` hold for `s1` and not for
    `s2`.
  - `new` in `out_test.go`: a `subagent_start` and a `subagent_stop` of session `s1` write to the file of `s1`, and
    `agents_are_running` of `s2` does not see the subagent of `s1`.
  - `new` in `in_test.go`: the shell passes the session of the event to `LoadState`; for an event without a session
    a `block` with `once` blocks, a `run` acts and the hook warns, and nothing is appended.
  - `new` in `internal/app/workflow/log/logic_test.go`: `Decide` with three sessions orders them by their latest
    `at`, most recent last, also when two use different offsets (`+01:00` and `+02:00`); keeps the last `tail`
    sessions whole; keeps all with 0; puts a session with no parseable `at` first; breaks ties by session id.
  - `new` in `internal/app/workflow/log/in_test.go`: the text output prints `== session <id> ==` before the entries of
    each session; `--json` prints only the lines, without headers; the default `--tail` is 5; the warning of the
    skipped lines sums every file.
  - `new` in `internal/app/workflow/log/out_test.go`: `Adapter.ReadLog` returns one state per `.jsonl` file of the
    sessions folder, leaves out other files and `log.jsonl`, returns no state without the folder, and fails when a
    session file cannot be read.
  - `kept` in `internal/app/hook/answer`: every query of one session (inject once per session and actor, notify once
    per session, `block` with `once`, the loop guard, the watch of the edits, `agents_are_running`) behaves as before
    when all the lines are of the session of the event (the existing tests, writing to the session file).
  - `kept` in `logic_protect_test.go`: commands and edits that name `<git-dir>/polix/sessions/<id>.jsonl` are denied as
    the ones that named `log.jsonl` were.
- Changes:
  - Hook: `Port.LoadState` takes the session id after the context; the step of the state passes the session of the
    event of the evaluation.
    `Adapter.LoadState` builds the path from `state.DirName`, `state.SessionsDir` and `state.Session.File`, and returns
    the error of `File` for an empty session.
    `Adapter.Append` builds the same path from the `Session` of the entry; `logPath` takes the session.
  - `polix log`: `Port.ReadLog` returns a slice of `state.State`, one per session file, read through `state.Load`
    after listing the sessions folder in the adapter.
    `Decide` takes that slice and the count, and its `Output` holds the sessions to show in order.
    `Response` holds the sessions, each an id and its `Entry` values, and `Skipped`; `print` writes the header line
    before each session in the text output, and only the lines with `--json`.
    The flag `--tail`/`-n` defaults to 5 and its help says it counts sessions, 0 for all; the `Short` and `Long` of the
    command say it prints the last sessions of the state.
  - Remove `state.LogName`, and replace `log.jsonl` with the files of the sessions in the comments and documents listed
    in `Files`; `answer.md` says a hook reads only the state of its session and that `finished` and the repeat of a
    `run` count only within it; `log.md` describes the grouping, the order, `--tail` and `--json`.
- Verification: `just test`, expected: passes.
  Differential check:
  extract the base with `git archive HEAD | tar -x -C <base>` into a temporary directory, build `polix` from `<base>`
  and from the working tree into temporary directories, and in a temporary git repository with `examples/polix.yaml`
  feed both binaries, through `polix hook claude`, the same sequence of hook inputs of Claude Code for two sessions
  `s1` and `s2`: `SessionStart`, a `PreToolUse` of `Edit`, a `Stop` twice in `s1`, then a `Stop` in `s2`.
  Compare stdout, stderr and the exit code of each call: the only differences allowed are the `Stop` of `s2` running
  the checks a `passed` of `s1` skipped in the base (AC3), and the output of `polix log` (AC7 to AC9).
  Run `polix log`, `polix log -n 0` and `polix log --json` of the new binary there and check AC7 to AC10.

### Task 3: Document the state per session

- Covers: AC11.
- High risk: none.
- Files: `docs/polix.md` (the line of `polix log` in the commands, section `State`, the protection of the state, the
  repeat of a `run` in `How often each verb acts`, the condition `finished`, the lines that say a line counts on the
  same branch or HEAD commit, modify), `docs/use-cases.md` (the mention of `log.jsonl`, modify),
  `examples/polix.yaml` (the comment of what polix remembers, and any comment on `finished` or the repeat of a `run`
  across sessions, modify), `plugins/polix/skills/workflow/reference.yaml` (copied again from `examples/polix.yaml`,
  modify), `README.md` (the alt text of the overview image, modify), `CHANGELOG.md` (`Unreleased`, modify).
- Rules: GO-2, DOC-1.
- Reuse: none.
- Tests: `kept` `TestWorkflowReference` of `plugins/plugins_test.go`, which fails when the copy of the plugin
  differs from `examples/polix.yaml`; otherwise none, since docs are not observable by tests.
- Changes:
  - docs/polix.md: the state is `<git-dir>/polix/sessions/<name>.jsonl`, one file per session with its subagents, with
    the name rule; a hook reads only its session; an entry without a session is not written and the hook warns; the old
    `log.jsonl` is ignored and left on disk; `finished` and the repeat of a `run` count only within the session, on
    its branch or HEAD commit, and `claude --resume` keeps them; the examples of the protection name the sessions
    folder; `polix log` groups by session, `-n` counts sessions (5 by default) and `--json` prints only lines.
  - docs/use-cases.md, examples/polix.yaml, README.md: the same facts where they name the log.
  - CHANGELOG.md: under `Unreleased`, a `Changed` entry with the split per session, the narrowing of `finished` and
    of the repeat of a `run` to the session, the ignored `log.jsonl` and the new `polix log`.
- Verification: `just test -run TestWorkflowReference` and `rg -n "log\.jsonl" docs examples plugins README.md internal`,
  expected: the test passes and every match left says the old log is ignored.

## Risks

- `claude --resume` may give a new session id in some version of Claude Code or Codex, so a resumed session would
  start from an empty state: check it by hand in the end to end verification, and say it in docs/polix.md.
- Gates with `finished` and checks with `run` ask again in every new session, which costs time to users used to the
  old behaviour: the changelog states it under `Changed`.
- Tests that build entries without a session fail after Task 1: Task 1 gives them a session in the same task.
- The old `log.jsonl` keeps its size on disk forever: documented as ignored; removing it is up to the user.
- A hand-edited session file with lines of other sessions shows under the id of its first line in `polix log`:
  accepted, since only polix writes the files.

## Verification

- `just test`: passes.
- `golangci-lint run ./...`: no findings.
- `just check`: passes (install, `polix config check`, `polix rules check`, `polix doctor --offline`).
- `polix rules check`: passes, since the tasks follow rules with a `check` (GOARCH-1 to GOARCH-45, GO-4).
- End to end in this repository after `just install`: open a session, edit a Go file and stop, and check that
  `<git-dir>/polix/sessions/<session id>.jsonl` grows and `log.jsonl` does not; `polix log` shows that session last
  under `== session <id> ==`; open a second session, stop on the same content, and check that its checks run again;
  resume the first session with `claude --resume` and check that its lines go on in the same file.

## Iterations

### Iteration 01: 2026-11-12

- **Tasks**: Task 1 done; Task 2 done; Task 3 done (the first builder stopped after Task 1, a second one built Tasks 2
  and 3).
- **Departures**: `Entry.observedProblem` split out of `Entry.Valid` for GO-4; `state.FileExt` exported; first edits
  made with a Python heredoc; a session file without entries shows no session in `polix log` but its skipped lines
  count; `Output.IDs` in parallel with `Output.Sessions`; the fake `Append` calls `Entry.Valid`; the warning of an event
  without a session lives at the start of `Decide`, so it also warns when nothing is written, and a blocked call shows
  no warning (rule 15); `docs/assets/overview.svg` and `docs/reference.md` edited outside the `Files` of the plan.
- **Docs**: docs/polix.md, docs/reference.md, docs/use-cases.md, docs/assets/overview.svg, examples/polix.yaml,
  plugins/polix/skills/workflow/reference.yaml, README.md, CHANGELOG.md, answer.md, log.md, load.md.
- **Review rev01**: approved with comments; 2 major findings (no warning for an event without a session that never
  reads the state; docs/reference.md still counting `finished` and the repeat of a `run` across sessions), both fixed
  in Fix 1.
- **Review rev02**: approved, no findings.
- **Acceptance**: AC1 met; AC2 met; AC3 met; AC4 met; AC5 met; AC6 met; AC7 met; AC8 met; AC9 met; AC10 met; AC11 met.
- **Status**: built.
