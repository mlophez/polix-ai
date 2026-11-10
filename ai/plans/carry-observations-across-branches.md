# Let a review, a passed run and a reminder survive a new branch made from the same commit

## Overview

Today polix forgets what it saw as soon as the branch changes.
A review of the Go code done on `main` does not count when the session then creates a branch with `git checkout -b`
and commits there, even though not a single Go file changed: polix asks for the same review again.
The checks that already passed run again, and the reminder to update the docs blocks again.

With this change polix also remembers the commit the work sat on (HEAD) when it records a review, a passed check or
a reminder.
Such a record keeps counting on another branch when that branch sits on the same commit, which is what
`git checkout -b` gives, so the change about to be committed is exactly the one that was reviewed.
On the same branch nothing changes: a record still counts after a commit, so committing a reviewed change in two
parts does not ask for a second review.

A branch on another commit, an older record written before this change and a repository with no commit yet behave
as today: the record counts only on its own branch.
The log grows only on the lines polix reads back, and `polix log --json` shows the commit.

## Context

The condition `finished`, the skip of a `run` that already passed and the memory of a `once` block select their
observed lines of the log by exact branch.
A branch created after the review, with the same HEAD and the same content, finds none of them, so the review, the
checks and the reminder repeat (seen in the log on 2026-11-10: a review on `main`, then a block on
`feat/requirements` with only a report in `ai/reports/` changed in between).
The outcome is that those lines also match on another branch when it has the same HEAD commit.

## Decisions

- A line matches when it was written on the current branch, or when it carries a HEAD commit equal to the current
  one: an extension of today's rule, never a replacement, so a partial commit on the same branch keeps its review.
- The rule applies to the three readers of observed lines by branch: `finished`, the repeat of `run` and `once`
  (answer of the interview).
- The HEAD commit is written only on the observed lines that are about a content and are read by branch: `finished`
  with a hash, `passed` and `blocked`; decisions and the other outcomes do not carry it (answer of the interview).
- The branch and the HEAD commit are learned together in one fact, from one git call, so a hook that already asks
  for the branch pays no extra process.
- The port of AnswerHook keeps its 19 methods (the limit of interfacebloat): its method `Branch` becomes `Head`,
  which returns the branch and the commit.
- A line without HEAD commit (written by an older polix) or a current HEAD that is unknown (no commit yet, git
  failing) matches only by branch, as today.

## Approach

The domain `state` owns the rule.
`state.Entry` gains the field `Head`, the commit HEAD pointed to when the line was written, empty when unknown.
`state.Query` gains the field `Head`; with `ByBranch`, it selects an entry whose branch equals `Branch`, or, when
both the query and the entry have a non empty `Head`, whose `Head` equals it.
The doc comments of `Query`, `Entry` and `OutcomeFinished` say so.

The facts of the hook learn the commit with the branch.
`internal/platform/git` replaces `Repo.Branch` with `Repo.Head`, which returns the branch ("" on a detached HEAD)
and the commit ("" when HEAD resolves to no commit), from one `git rev-parse` that prints both, errors swallowed as
`Branch` does today.
`hook.Facts` gains `Head`, set by `learnBranch` with the branch, and `hook.Evaluation.KnowBranch` takes both.
The step `hook.Step.Branch` keeps its name and meaning: it asks for the branch and the commit.
The port of `internal/app/hook/answer` replaces `Branch` with `Head`, and the adapter wraps `Repo.Head`.

The logic of AnswerHook uses it in two places.
`pass.write` sets `Head` on an observed entry of outcome finished, passed or blocked that carries a hash.
`pass.finished`, `pass.passed` and `pass.reminded` put the current commit in the `Head` of their queries.
`pass.finished` keeps taking the latest matching `finished` line with a hash, now from any branch with the same
HEAD as well, and compares its tree on the scope as today.

The log gains the key `head`, left out when empty.
The encoder of AnswerHook (`logLine` and `Adapter.Append`), the decoder of the loader (`entryJSON` of
`internal/app/state/load`) and the `Entry` of `polix log` (`internal/app/workflow/log`) carry it.
An older log reads as before: its lines have no `head`.

No new error: a commit that cannot be read is empty and only narrows the match to the branch.

### Discarded alternatives

- Drop the branch from the queries and trust the tree alone: a branch on an older commit with the same tree would
  commit changes the reviewer never saw as a change.
- Require the same HEAD instead of the same branch: a reviewed change committed in two parts would ask for a review
  after the first commit.
- Write the commit on every line of the log: nothing reads it on the other lines, and every line grows.
- Add a separate port method for the commit: the port would pass the limit of 19 methods, and a second git call
  would run on every hook that asks for the branch.

## Acceptance criteria

- AC1: A `finished: polix:reviewer` recorded on branch A holds on branch B when B has the same HEAD commit and no
  file in the scope of the entry changed; the `pre_commit` gate does not block.
- AC2: The same `finished` does not hold on branch B when B has another HEAD commit; the gate blocks.
- AC3: On the same branch, a `finished` keeps holding after a commit moved HEAD, while the files in scope did not
  change, as today.
- AC4: A `run` that passed on branch A on a content is skipped on branch B with the same HEAD and the same content
  hash, and runs on a branch with another HEAD.
- AC5: A `once` block that reminded the actor of a session on branch A does not block again on branch B with the
  same HEAD while the files in its scope did not change.
- AC6: The observed lines finished with a hash, passed and blocked carry the key `head` with the commit when it is
  known; decisions and the other outcomes do not; `polix log --json` shows it.
- AC7: A line without `head` and a repository without commits match only by branch, as today.
- AC8: `docs/polix.md` and the documents of the use cases touched describe the new rule and the new field.

## Out of scope

The loop guard of `stop` and `subagent_stop` counts blocks by session and hash, not by branch: it does not change.
The injects that act once are keyed by session, not by branch: they do not change.

## Tasks

### Task 1: Select observed lines by branch or by HEAD commit in the domain state, and read the field from the log

- Covers: AC6, AC7.
- Depends on: none.
- Files: `internal/app/state/state_entry.go` (`Entry`, `OutcomeFinished`, modify), `internal/app/state/state.go`
  (`Query`, `Query.selects`, modify), `internal/app/state/load/out.go` (`entryJSON`, modify),
  `internal/app/state/load/load.md` (modify), `internal/app/workflow/log/in.go` (`Entry`, modify),
  `internal/app/workflow/log/log.md` (modify), `internal/app/hook/answer/out_log.go` (`logLine`, modify).
- Rules: GOARCH-1, GOARCH-2, GOARCH-4, GOARCH-13, GOARCH-14, GOARCH-16, GOARCH-18, GOARCH-21, GOARCH-24,
  GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-30, GOARCH-35, GOARCH-36, GOARCH-38, GOARCH-41, GOARCH-44, GO-2, GO-4,
  DOC-1.
- Reuse: the conversions `state.Entry(j)` of `ReadLog`, `Entry(e)` of the shell of `polix log` and `logLine(e)` of
  `Adapter.encodeLine`, which keep working when the new field sits at the same position in the four structs.
- Changes: `Entry` gains `Head`, placed right after `Hash`, documented as the commit HEAD pointed to, written only on
  the observed lines about a content that are read by branch.
  `Query` gains `Head`; `selects` with `ByBranch` accepts an entry of the same branch, or one whose non empty `Head`
  equals the non empty `Head` of the query; without `ByBranch` the head is ignored.
  The doc comment of `Query` replaces the sentence on the exact branch with this rule, detached HEAD included.
  `entryJSON`, the `Entry` of `polix log` and `logLine` of AnswerHook gain `Head` with the key `head`, omitted when
  empty, at the same position as in `state.Entry`; copying `Head` in `Adapter.Append` is left to Task 3.
  `load.md` and `log.md` name the new field where they list the fields of a line.
- Tests: in `internal/app/state/state_test.go`, a query with branch and head selects an entry of the same branch with
  another head, an entry of another branch with the same head, and not one of another branch with another head;
  an entry of another branch without head is not selected; a query without head selects only by branch; on a
  detached HEAD (empty branch) an entry of a branch with the same head is selected.
  In `internal/app/state/load/out_test.go`, a line with `head` decodes into `Entry.Head`, and a line without it
  leaves it empty.
  In `internal/app/workflow/log/in_test.go`, `--json` prints `head` for a line that has it and omits it otherwise.
- Verification: `just test`, expected: all packages pass.

### Task 2: Learn the HEAD commit with the branch, in one git call

- Covers: none.
- Depends on: none.
- Files: `internal/platform/git/git.go` (`Repo.Branch`, modify), `internal/platform/git/git_test.go` (modify),
  `internal/app/hook/hook_facts.go` (`Facts`, `Facts.learnBranch`, modify), `internal/app/hook/hook_evaluation.go`
  (`Evaluation.KnowBranch`, modify), `internal/app/hook/answer/in.go` (`Port`, `carry`, modify),
  `internal/app/hook/answer/out.go` (`Adapter.Branch`, modify), `internal/app/hook/answer/out_test.go` (modify),
  `internal/app/hook/answer/logic_test.go` (`walk`, modify), `internal/app/hook/hook_step.go` (`Step`, modify).
- Rules: GOARCH-1, GOARCH-4, GOARCH-5, GOARCH-12, GOARCH-13, GOARCH-18, GOARCH-19, GOARCH-20, GOARCH-24,
  GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-30, GOARCH-33, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-37, GOARCH-38,
  GOARCH-41, GOARCH-42, GOARCH-44, GO-2, GO-3, GO-4, XPLAT-1, DOC-1.
- Reuse: `Repo.git` of `internal/platform/git`, with the context detached from cancellation as `Repo.Branch` does.
- Changes: `Repo.Branch` becomes `Repo.Head`, which returns the branch and the commit from one `git rev-parse` that
  prints the full commit of HEAD and its abbreviated name; "HEAD" as the name means a detached HEAD, so the branch
  is empty; any error leaves both empty, as the branch is today on a repository without commits.
  `Facts` gains `Head` next to `Branch`, set together by `learnBranch`; `KnowBranch` takes the branch and the commit.
  The method `Branch` of `Port` becomes `Head`, returning the branch and the commit, documented in its comment;
  `carry` feeds both to `KnowBranch` on the step `Branch`; `Adapter.Head` wraps `Repo.Head`.
  The port keeps 19 methods.
  The comment of `hook.Step.Branch` says it asks for the branch and the HEAD commit.
  `walk` of `logic_test.go` feeds the commit to `KnowBranch` from a new field of its facts, empty by default, so the
  tests of the logic compile and Task 3 can set it.
- Tests: in `internal/platform/git/git_test.go`, `Head` returns the branch and the commit of HEAD on a branch, an
  empty branch and the commit on a detached HEAD, and both empty on a repository without commits.
  In `internal/app/hook/hook_facts_test.go` and `internal/app/hook/hook_evaluation_test.go`, `KnowBranch` keeps
  both values and marks the branch known.
  In `internal/app/hook/answer/in_test.go`, the fake port answers `Head` and the shell feeds both values back.
  In `internal/app/hook/answer/out_test.go`, the test of the adapter calls `Adapter.Head` and checks the branch and
  a non empty commit.
- Verification: `just test`, expected: all packages pass; `golangci-lint run ./...`, expected: no findings
  (interfacebloat included).

### Task 3: Write the HEAD commit on the observed lines and match by it in finished, run and once

- Covers: AC1, AC2, AC3, AC4, AC5, AC6, AC7.
- Depends on: Task 1, Task 2.
- Files: `internal/app/hook/answer/logic_facts.go` (`pass.branch`, modify), `internal/app/hook/answer/logic_log.go`
  (`pass.write`, modify), `internal/app/hook/answer/logic_condition.go` (`pass.finished`, modify),
  `internal/app/hook/answer/logic_run.go` (`pass.passed`, modify), `internal/app/hook/answer/logic_entries.go`
  (`pass.reminded`, modify), `internal/app/hook/answer/out_log.go` (`Adapter.Append`, modify),
  `internal/app/hook/answer/answer.md` (modify).
- Rules: GOARCH-3, GOARCH-4, GOARCH-13, GOARCH-14, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-24, GOARCH-29,
  GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-38, GOARCH-41, GO-2, GO-3, GO-4, DOC-1.
- Reuse: `pass.branch` and the step `hook.Step.Branch`, `pass.sameScope`, `state.State.Observed` and
  `state.State.Latest`.
- Changes: next to `pass.branch`, an unexported accessor gives the commit learned with it, asking the same step when
  it is not known yet.
  `pass.write` sets the `Head` of an observed entry of outcome finished, passed or blocked that carries a hash; every
  other entry keeps an empty `Head`.
  `pass.finished`, `pass.passed` and `pass.reminded` set the current commit as the `Head` of their queries, keeping
  `ByBranch`; the rest of their logic (latest line with a hash, comparison of the tree on the scope, unknown hash)
  stays as it is, and their doc comments say "on the current branch or on the same HEAD commit".
  `Adapter.Append` copies `Head` into the stamped entry; `logLine` already has the field (Task 1).
  `answer.md` describes in its Logic that these three readers match by branch or by HEAD commit, and names `Head`
  in its Port.
- Tests: in `internal/app/hook/answer/logic_condition_test.go`, a `finished` line of branch A with head H holds on
  branch B with head H when only a file outside the scope changed (AC1); it does not hold on branch B with head H2
  (AC2); on branch A with head H2 it holds (AC3); a line without head on branch A does not hold on branch B (AC7);
  with an unknown current head only the branch matches (AC7).
  In `internal/app/hook/answer/logic_run_test.go`, a `passed` line of branch A with head H skips the run on branch B
  with head H and the same hash, and not with head H2 (AC4).
  In `internal/app/hook/answer/logic_test.go`, a `once` block that reminded on branch A with head H does not block
  on branch B with head H, and blocks with head H2 (AC5); the written observed lines finished, passed and blocked
  carry the head, and the decisions do not (AC6).
  In `internal/app/hook/answer/out_log_test.go`, `Append` writes `head` when set and omits it when empty (AC6).
- Verification: `just test`, expected: all packages pass.

### Task 4: Document the new rule in the docs of the project

- Covers: AC8.
- Depends on: Task 3.
- Files: `docs/polix.md` (modify), `README.md` (modify, only if a sentence states the exact branch).
- Rules: GO-2, DOC-1.
- Reuse: none.
- Changes: in `docs/polix.md`, the condition `finished` of the list of conditions says "on this branch, or on another
  one with the same HEAD commit"; the paragraph "The branch is compared exactly" of "What polix observes, and
  finished" is rewritten with the rule (same branch, or same non empty HEAD commit; older lines and repositories
  without commits by branch only; detached HEAD included) and keeps that it holds for the repeat of `run` and for a
  `once` block; the bullet on the repeat of `run` says the same; the list of the fields of a line of the log names
  `head` and on which lines it is written.
  `README.md` changes only where it states that a review or a passed run is tied to the exact branch.
- Tests: none, documentation only.
- Verification: `rg -n "compared exactly" docs/polix.md`, expected: no line left with the old rule.

## Risks

- A branch on the same commit but meant for another change reuses a review: the tree is still compared on the scope,
  so any edit of a Go file asks again; only an identical change carries over.
- The port method renamed from `Branch` to `Head` breaks the fakes of the tests: Task 2 updates them in the same
  task.
- The conversions between `state.Entry`, `entryJSON`, the `Entry` of `polix log` and `logLine` of AnswerHook need the
  same fields in the same order: Task 1 adds `Head` at the same position in the four, and the compiler checks it.

## Verification

- `just test`: go vet and every test pass.
- `golangci-lint run ./...`: no findings.
- `just check`: install, `polix config check`, `polix rules check` and `polix doctor --offline` pass.
- End to end, in a scratch repository governed by this polix.yaml, on `main` with a Go change over the thresholds:
  run the reviewer until it finishes, edit only a markdown file, run `git checkout -b feat/x`, commit: the review
  gate does not block, and `polix log --json` shows `head` on the `finished` line.
- Same scratch repository: after the review, run `git checkout` of a branch made from an older commit with the same
  files, commit: the review gate blocks.
