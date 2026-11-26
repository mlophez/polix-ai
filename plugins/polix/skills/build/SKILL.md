---
name: build
description: >
  Implements a plan task by task, unattended: the subagent builder builds each task, reviewer notes its findings
  without iterating, and the build commits it. Then verifier checks the whole plan, and the build asks the user to
  triage the findings until they close the plan. It never pushes. It runs only when the user asks for it.
user-invocable: true
disable-model-invocation: true
argument-hint: "[plan and optional first task, e.g. add-retries.md T3; empty to take it from the conversation]"
---

# Building

Run a plan to its end, unattended: once it starts, it reaches `review`, or `blocked` for what only a person can solve.
You coordinate and never build: each task runs its own cycle, so every agent sees one task and a small context.
In the cycle of a task, the builder builds it and reports, you write the report in the file of its pass, and the
reviewer reviews that task once.
The review never sends the task back: you note its findings for the user, commit the task, and the next task starts
from that commit.
Once every task is committed, the verifier checks the whole plan: it is the only check against the plan, it runs in
no task, and only its findings are fixed in a loop.
The plan then waits in `review` and you triage it with the user: they pick the findings to fix, which run as an
iteration of `final`, and you ask again until they close the plan, which you set `done`.
You never update the docs of the project: they are not part of building.
You are the only one that edits the plan and the files of its passes, and commits: the subagents read them and
report.
The folders of plans, rules, reports and tasks are those the project names in `AGENTS.md`, `CLAUDE.md` or the session.
Without one, they are `ai/plans/`, `ai/rules/`, `ai/reports/` and `ai/tasks/` below the root.

## Requirements

- A plan file of the folder of plans, as the skill spec writes it: Markdown with a YAML frontmatter whose map
  `tasks` holds the ids `T1`, `T2`... and whose map `acceptance` holds the ids `AC-01`, `AC-02`....
  Read the body as prose, and find each task, criterion and literal by its id, never by the exact form of a heading.
  The title of the plan is the heading `#` of its body.
  Without a plan, stop and offer the skill `spec`: never build without a plan nor write one yourself.
- [pass.md](pass.md), the template of the file of a pass, next to this file.
  Without it, stop and tell the user.
- The subagents `builder`, `verifier` and `reviewer`, as `<plugin>:builder` when a plugin provides them.
  `verifier` checks that the change meets the plan, and `reviewer` that its code is well written.
  Without them, as in Codex, read `builder.md`, `verifier.md` and `reviewer.md` in `../../agents/`, next to this skill.
  Follow each yourself when its turn comes.
  The rules of this skill win over theirs, and the checks are not independent: say so in the report.

## Input

- **Plan**: the path from the arguments, or the plan named, written or discussed in the conversation.
  Without either, list the plans of the folder of plans and ask the user which one; never pick one yourself.
- **From**: the task from the arguments, as `T3` or `3`, to start there.
  Without it, the first task whose status in `tasks` is not `done`, so a task `departed` or `blocked` is tried again.
- **Clean tree**: run `git status --porcelain`.
  With changes other than the plan file, the files of its passes and the task file its `reference` names, stop and
  ask the user to commit or stash them.
  Each commit of the build holds only its task, so the run never starts on changes of others.
  Never stash, commit nor discard them yourself.
  Only a plan `in-progress` or `blocked` starts with changes: they are the work of the run it resumes (see `Resume`).
- **Branch**: the `branch` of the plan, the branch it was planned on and is built on.
  When the current branch (`git branch --show-current`) is another one, switch to it with `git switch <branch>`.
  When the branch exists neither locally nor on a remote, stop and tell the user; never create it.
  A plan without `branch` builds on the current branch.
- **Approval**: a plan with `status: planned` is not approved.
  Stop and ask the user to approve it with the skill `spec`; never approve it yourself.
- **Resume**: a plan `in-progress` was interrupted, and a plan `blocked` waited for a person.
  Resume either where it stopped (see `Resume`); the files of its earlier passes stay.
  A plan `review` waits for the user: skip Steps 2 to 8 and go to the triage of Step 9 (see `Triage`).
  A plan `done` is closed: tell the user and stop.
  A plan `approved` builds from `From`, the tasks a person added after a `review` included.

## Output

- **Plan**: the path of the plan file, its `status` and the branch it was built on.
- **Tasks**: each task with its `status`: `done` with its evidence and commit, `departed` or `blocked` and why.
- **Commits**: each commit of the run, its short hash and subject; none is pushed.
- **Changes**: the files changed, by task and by finding.
- **Acceptance**: each AC with `met`, `not met`, `not verified` or `pending` and its evidence.
- **Findings**: the findings the reviewer noted in this run, for the user to triage (see `Findings report`).
- **Stats**: the build in figures (see `Stats`).
- **Blockers**: when the plan ends `blocked`, each reason and what a person must do to clear it; or "none".
  Name the uncommitted changes the run left, and the task or the final verification they belong to.
  Once a person clears it, the skill `build` resumes the plan with those changes.
- **Pending**: the manual checks and the failures that are not the change's.
  Also the changes `AGENTS.md`, `CLAUDE.md` or a protected file need; or "none".
- **Next step**: with the plan `review`, the question of the triage, right under the report (see `Triage`).

## Limits

- Ask only in `Input`, before Step 3, and in the triage of Step 9: between them, never ask the user nor wait for an
  answer.
  Decide what is open from the plan, the code and the rules, and record it in `Pending`.
- Never end your turn before Step 8: a subagent that returns, a failed check or a long run is never the end.
  The run ends only when the user closes the plan, or a loop stops (see `Task cycle` and `Final verification`).
- Never edit the code yourself: only the builder changes it, except in the fallback of `Requirements`.
  You edit only the frontmatter of the plan, and never its body nor a doc of the project.
  Of the frontmatter, you edit its `status`, the status of each task in `tasks` and of each criterion in
  `acceptance`, and the fields of `stats` (see `Plan stats`).
  You write the files of the passes (see `Passes`): their `status`, the status of their findings, and their body.
  You set the plan `done` only when the user closes it in the triage (see `Triage`).
- Edit the frontmatter with targeted edits that keep it valid YAML, never writing the plan again whole.
- Commit only as `Commits` says, each task and each round of `final`: never push, stash, reset, amend, rebase nor create
  branches.
  The only checkout is the switch to the plan's `branch` in Step 1 (see `Input`).
- Write the status and the evidence of the tasks and findings as the builder reports them, never your own.
  Still, count nothing as verified on the builder's word: only the verifier's report closes the whole plan.
- Never send a task back to the builder for a finding of the reviewer, nor launch the builder on one: you only note
  it for the user.
- Never run a command that installs, generates or formats, even when the plan or `AGENTS.md` lists it.
- Add the findings the verifier, the reviewer and a failed commit report as they are: none of your own, none dropped.
- Talk to the user in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Build progress:
- [ ] Step 1: Resolve the plan, the clean tree, its branch, its approval, the first task and resume (see `Input`).
  Read the plan whole, and the files of its passes when it has any (see `Passes`).
- [ ] Step 2: Show the user the tasks to build, in order, and go on without waiting.
- [ ] Step 3: Set the plan `in-progress` (see `Status`), its `stats.start` and `stats.iterations` (see `Plan stats`).
  Take the start of this run and say it to the user (see `Timeline`).
- [ ] Step 4: Run the cycle of each task from `From`, in order (see `Task cycle`).
  When a cycle stops, go to Step 6.
- [ ] Step 5: Verify the whole plan and fix until nothing is open or the loop stops (see `Final verification`).
- [ ] Step 6: Set the plan `review` or `blocked` (see `Status`).
  Set its `stats.end`, `stats.duration`, the open entry of `stats.timeline` and `stats.code` (see `Plan stats`).
  When `review`, commit the plan and what the final verification fixed (see `Commits`).
- [ ] Step 7: Gather the stats of the build (see `Stats`).
- [ ] Step 8: Report (see `Output`); when the plan is `blocked`, stop and return control to the user.
- [ ] Step 9: With the plan `review`, triage its findings with the user until they close it (see `Triage`).
```

## References

### Task cycle

Run each task from `From` to the last, one at a time, in order; skip a task `done`.
Each pass of the builder on the task has its own file (see `Passes`).

1. **Build**: launch the builder on the task (see `Models` and `Builder prompt`).
   Write its report in the file of its pass (see `Builder report`).
2. **Retry**: when the builder left the task `not started` or `departed`, launch it once more on the task, in a new
   pass, and write its report.
   It reads the evidence of the first pass, so it tries another way.
3. **Stop**: the cycle stops when the task is `blocked`, or still not `done` after the retry.
   Each reason goes to `Blockers` with what a person must do.
   The work of the task stays uncommitted, no later task is built, and the commits of the earlier tasks stay.
4. **Review**: launch the reviewer on the task once, and wait for it (see `Reviewer prompt`).
   Never launch the verifier in a task, whatever its `risk`.
   Note each finding of its report in the file of the last pass of the task, as it is, with the status `noted`
   (see `Iteration`).
   A noted finding waits for the user: it reopens nothing, and no builder works on it.
   Set that file to `status: checked`.
5. **Close**: commit the task (see `Commits`), and go on with the next task.

The criteria stay `pending` through the cycles of the tasks: only the final verification evaluates them.

### Final verification

Once every task is committed, check the whole plan, and repeat until the verifier leaves nothing open:

1. **Check**: launch the verifier with `Task: final` (see `Verifier prompt`).
   After an iteration of `final`, launch the reviewer with `Task: final` in the same message, so they run in
   parallel (see `Reviewer prompt`); before the first iteration, nothing is uncommitted for it to review.
   Wait for both, and set the file of the pass the builder worked on, if any, to `status: checked`.
   The first check has no pass of a builder: write the file `final` of round 0 once the verifier returns, with
   `status: checked` (see `Passes`).
2. **Criteria**: set every criterion from the verification (see `Acceptance criteria`).
3. **Open**: the verifier's findings, its criteria `not met` among them.
   Note the reviewer's findings, as in `Task cycle`, in the file of the pass it reviewed: they never open anything.
   Leave out a finding the builder skipped twice for the same reason: it goes to `Pending`.
4. **Done**: with nothing open, the loop ends.
5. **Stop**: after two iterations that fixed nothing, or after three iterations of `final` in this run.
   Each reason goes to `Blockers`; its last round stays uncommitted, and its round commits stay.
6. **Iteration**: otherwise, add an iteration of `final` with what is open (see `Iteration`).
   Commit the round with it (see `Commits`), so the next review reads only what the next round changes.
   Launch the builder on it and write its report in its file, set it `status: built`, and go back to 1.

### Passes

Each pass of the build on a task or on the final verification has its own file next to the plan, from
[pass.md](pass.md).
Its name is the plan's path without `.md`, then the task in lowercase or `final`, then the round:

- `<plan>.t3.md`: the first pass of T3, round 0, which the builder builds from the plan.
- `<plan>.t3.rev1.md`, `<plan>.t3.rev2.md`...: each later pass of T3, round 1, 2..., numbered within its task.
  That is the retry of a task not done, or the fix of a commit the hooks rejected.
- `<plan>.final.md`: the first check of the final verification, round 0, with the evidence of the criteria.
- `<plan>.final.rev1.md`, `<plan>.final.rev2.md`...: each iteration of the final verification.

A task, or `final`, that already has files, from a run that resumes it or tries it again, takes the next round for
every new pass, its first one included.
Write a file once its pass starts, and fill its body as the reports come back.
Never edit the file of an earlier pass but its `status`, nor delete one: together they are the history of the plan.

The frontmatter of a file:

- `plan`: the path of the plan, in the one form of `Commits`.
- `task`: the id of the task, as `T3`, or `final`.
- `round`: `0` for the first pass, else its number in its task.
- `status`: `open` while the builder works on it, `built` once it returns, `checked` once the checks ran.
- `date`: today as YYYY-MM-DD.
- `findings`: a map of each `FND-NN` of the pass to its status; `{}` without findings.
  `noted` for a finding of the reviewer, which waits for the user.
  `open`, then `fixed` or `skipped`, for a finding the builder fixes: one of the verifier, or a rejected commit.

The body holds the text of the reports, in the order of pass.md: `Task`, `Findings`, `Acceptance`.

### Iteration

A finding goes in the file of a pass as an item of its `Findings`, with its status in `findings`:

- Its id, `FND-NN`, numbered across the whole plan: the highest of the files of the plan's passes plus one.
- `Source`: `review`, `verification` or `commit`.
- `Kind`: the area of a finding of the verification, the severity and category of one of the review, or the check
  that failed the commit.
- `Where`: the `file:line` or the id of the plan it is about.
- `Problem` and `Fix`: what is wrong, with its instances and evidence, and what to change, from the report.
- `Repeats`: the `FND-NN` of an earlier pass that the report says the finding repeats, or `none`.
- Its `Evidence`: "None." while it is `noted` or `open`; for a fixed or skipped one, the builder's report.

Copy each finding from its report as it is, its `Repeats` included: never merge, reword nor judge it.
The criteria go to `acceptance` of the plan and to the file of `final`, never to the findings, and the rest of the
reports goes to `Output`.

An iteration is one pass of the builder on findings to fix, in a new file with a round past the last one.
It opens `status: open`, with each finding to fix `open`: the verifier's in an iteration of `final`, or the failure
of a rejected commit (see `Commits`).
You set each one `fixed` or `skipped`, with why, from the builder's report.

The first pass of T3, whose review noted one finding, as `<plan>.t3.md`:

````markdown
---
plan: ai/plans/add-retries.md
task: T3
round: 0
status: checked
date: 2026-11-14
findings:
  FND-01: noted
---

# T3, first pass: Export the rows in order

## Task

done
`go test ./internal/export -run '^TestExportRows$'` failed before the change and passes.
The verification of the plan passes.

## Findings

### FND-01: major, test, internal/export/writer_test.go:31

- **Source**: review
- **Kind**: major, test
- **Where**: internal/export/writer_test.go:31
- **Repeats**: none

**Problem**

TestExportRows compares the rows as a set, so it passes with the rows unsorted, against LIT-04.

**Fix**

Compare the rows in order, with a case whose input comes unsorted.

**Evidence**

None.

## Acceptance

None.
````

### Commits

You commit, never a subagent; each commit holds the plan file and the files of its passes too, so `HEAD` shows the
progress.
When git ignores the plan file (`git check-ignore -q <plan>`), the commits leave it and the files of its passes out.
The `<path of the plan>` of every line `Plan:` has one form, whatever form the arguments gave it in.
It is relative to the root of the repository (`git rev-parse --show-toplevel`), with `/` as separator, such as
`ai/plans/add-retries.md`: never a bare filename, an absolute path nor `./` before it.

- **Task**: once its review ran.
  Subject `T<N>: <title of the task>`, then a body with its summary and the line `Plan: <path of the plan>`.
- **Round**: in the final verification, once its checks leave something open and the loop goes on, after its
  iteration is added.
  Subject `Final rev<R>: <title of the plan>`, with the round of the iteration it opens.
  A body with the `FND-NN` of that iteration, then the line `Plan: <path of the plan>`.
  So the review of the next round reads, as `git diff HEAD`, only what that round changes.
  A round that stops its loop is never committed: it stays uncommitted for `Blockers` and `Resume`.
- **Final**: at Step 6 when `review`: subject `Final: <title of the plan>`, a body with the `FND-NN` the final
  verification fixed, or "Nothing to fix.", and the line `Plan: <path>`.
- **Close**: when the user closes the plan in the triage: subject `Close: <title of the plan>`, and the line
  `Plan: <path>` as its body.
- When `git status --porcelain` shows nothing, as with an ignored plan and nothing changed, make no round nor final
  commit, and say so in `Commits` of `Output`.
  A round without its commit adds nothing to the next review, and the loop goes on as if it had committed.

How to commit:

1. Write the entry of the task, or of `final`, in `stats.timeline` (see `Timeline`), so the commit holds it.
   A round commit skips this step: it writes no entry.
2. List the changed and untracked paths with `git status --porcelain`.
   The tree held at most the plan file, the files of its passes and its task file when the run started, so all are
   the run's.
   Stage them by path with `git add <paths>`, never `git add -A`, `-a` nor a path after `git commit`.
3. Commit with `git commit -m "<subject>" -m "<body>"`, run by its name and ending the line.
   The message holds only those lines: no trailer, no attribution.
   Never pass `--no-verify`: the hooks of the project are part of the checks.
4. When a hook blocks the commit with a reminder that it gives only once, such as updating the docs, commit again once.
   You never update the docs, so a commit never stops for them.
5. When a hook or a check fails the commit, the failure is a finding with `source: commit`.
   - The commit of a task: add an iteration of the task with that finding, launch the builder on it once, and commit
     again; the reviewer does not review that pass, since the hook checks it.
     When the commit fails again, stop the cycle as its step `Stop` does.
   - A round or final commit: add the finding to the iteration the round opens, or to a new iteration of `final`, and
     go on with its loop: it counts toward its limit.
     A failed round commit leaves the round uncommitted, and the review after it reads both rounds.
   A finding with `source: commit` is fixed once the next commit of the task, or of `final`, passes.
   The entry of step 1 stays: the next commit of the task, or of `final`, writes its `end` and `duration` again.
6. When a block asks for what only a person can do, stop the loop as `Task cycle` does.
7. Take the short hash with `git rev-parse --short HEAD` for `Output`; never push.

### Resume

A plan `in-progress` was interrupted, and one `blocked` stopped; the commits of the branch say which tasks are closed.

- Read the frontmatter of the files of the passes of the plan.
  With a last file `open`, launch the builder on it; with one `built`, run its checks.
- Otherwise, with uncommitted changes, they belong to the first task without its commit `T<N>: ` in
  `git log --format=%s $(git merge-base HEAD <base>)..HEAD`, or to `final` when every task has one.
  Go on with the cycle of that task from its step `Review`, or with the final verification, on them.
- A round commit, `Final rev<R>: `, is work of the final verification, which is not closed.
- Otherwise, run the cycle from `From`, or the final verification when every task is `done`.
- A task `blocked` in an earlier run was cleared by a person: stop on it only when a builder of this run reports it
  `blocked` again.
- The base is the base branch `AGENTS.md` or `CLAUDE.md` names.
  Without one, the branch `git symbolic-ref --short refs/remotes/origin/HEAD` names without `origin/`, else `main`.

### Plan stats

The fields of `stats`, at the root of the plan, record the build as a whole across every run, a resumed one too.

- `start`: at Step 3, the current time from the shell, as `YYYY-MM-DDTHH:MM:SS+HH:MM` with the local offset.
  Take it from a command, such as `date -Iseconds` or `Get-Date -Format "yyyy-MM-ddTHH:mm:sszzz"`; never guess it.
  Set it only when it is `none`: a resumed plan keeps the `start` of its first run.
- `end`: at Step 6, the current time the same way; a later run of the plan writes it again.
- `duration`: `end` minus `start`, as `1h 05m 12s`; it includes the time the plan stood interrupted.
- `iterations`: at Step 3, the runs of the build on the plan: `1` when it is `none`, else one more.
  Every run counts, a resumed one too; it is not the number of files of iterations of the plan.
- `timeline`: the time each task and the final verification took in each run (see `Timeline`).
- `code`: at Step 6, the `files` changed and the `lines` added plus removed on the branch, leaving out the plan.
  The commits of the build moved `HEAD`, so measure against the base of `Resume`.
  Take them from `git diff --shortstat $(git merge-base HEAD <base>) -- . ':(exclude)<plan>'`.
  Add the untracked files of `git ls-files --others --exclude-standard`, each line of one as added.
  Never stage them to measure them.

```yaml
stats:
  start: 2026-11-14T21:05:40+01:00
  end: 2026-11-14T21:48:02+01:00
  duration: 42m 22s
  iterations: 2
  timeline:
    - id: T1
      start: 2026-11-14T21:05:40+01:00
      end: 2026-11-14T21:12:03+01:00
      duration: 6m 23s
      files: 2
      lines: 146
    - id: T2
      start: 2026-11-14T21:12:03+01:00
      end: 2026-11-14T21:20:15+01:00
      duration: 8m 12s
    - id: T2
      start: 2026-11-14T21:31:40+01:00
      end: 2026-11-14T21:44:10+01:00
      duration: 12m 30s
      files: 5
      lines: 214
    - id: final
      start: 2026-11-14T21:44:10+01:00
      end: 2026-11-14T21:48:02+01:00
      duration: 3m 52s
  code:
    files: 7
    lines: 360
```

The first run stopped `blocked` on T2, and the second one closed it and ran the final verification.

### Timeline

`stats.timeline` holds one entry per stretch of work of a run on a task or on the final verification, in order.
Never edit nor remove an entry of an earlier run: it is the history of the plan.

- `id`: the id of the task, as `T3`, or `final`.
  A task worked on in several runs has one entry per run, all with its id.
- `start`: when the run began the work, from a command as `stats.start`.
  It is the `end` of the previous entry of this run, or the start of this run for its first entry.
  The start of this run is the current time at Step 3; say it to the user then, so a summary of the context keeps it.
  The time a plan stood interrupted counts in no entry.
- `end`: the current time from a command, taken right before the commit that holds the entry.
  - A task: at step 1 of `How to commit` of its commit `T<N>: `.
  - The final verification: `stats.end`, written at Step 6, also when there is no final commit.
  - The task or the final verification the run stops on, when the plan ends `blocked`: `stats.end`, at Step 6.
- `duration`: `end` minus `start`, as `stats.duration`.
- `files` and `lines`: only in the entry written at the commit `T<N>: ` of a task, the change of the whole task.
  Measure them at step 1 of `How to commit`, as `stats.code` but against the task base, the plan excluded:
  `git diff --shortstat <task base> -- . ':(exclude)<plan>'`, plus the lines of its untracked files.
  Every other entry, of an earlier run, of a stop or of `final`, has neither.

A run interrupted before Step 6 leaves no entry for the task it was on: the next run measures it from its start.
A plan without `timeline` gets it as `[]`.

### Models

Launch each subagent with the `model` and `effort` of its item in `agents` of the plan.
Its item is the one whose `name` is its name, with or without the prefix of its plugin (`builder`, `<plugin>:builder`).
Both values go as options of the launch.

- Leave out an option whose value is `inherit`, and both for an agent without an item.
  So does a plan without `agents`: the frontmatter of the agent then decides.
- With two items of one name, use the first and say so in `Pending`.
- A value the launch does not take: leave that option out and say so in `Pending`.
- In the fallback of `Requirements` there is no launch: the values do not apply, and `Stats` says so.

### Builder prompt

Send the builder this block, in the language of the session:

```text
Plan: <path of the plan file>
Branch: <the branch of Step 1, or "current" for a plan without `branch`>
Task: <id and title of the one task to build, or "none" when it works on an iteration>
Base: <the short hash of the task base, or "none" with final>
Iteration: <the path of the file of the iteration to work on, its task and findings, or "none">
```

The task base is the commit the task started from: the build commits a task only once, so it is `HEAD` while the
task runs.
Take `git rev-parse --short HEAD` right before the first builder of the task in each run, and keep it for its cycle.
Every pass of the task, its retry and the fix of a rejected commit included, gets the same one.

### Builder report

The builder never edits the plan nor a file of a pass: it reports what it built, and you write it once it returns.
Write it in the file of the pass the builder worked on: a pass of the task, or an iteration (see `Iteration`).

- **Task**: set the task's status in `tasks` of the plan as the builder reports it, `done`, `departed` or `blocked`.
  Write its evidence under `Task` of the file; a task `not started` keeps its status, and `Task` says why.
- **Findings**: for each finding of its `Findings`, set its status in `findings` of the file, `fixed` or `skipped`,
  and write its evidence under the `Evidence` of the finding.
- **Departures**: add each item of its `Departures` to the evidence of the task or finding it names.
  Write it as a line `Departure: <what the builder did and why>`, so the checks and later builders read it.
- **Baseline**: only when the builder reports a baseline, as the method of its project may ask, add to the task's
  evidence a line `Baseline: <its failures>`.
  Take the failures its `Verification` reports under `Baseline`, or write `Baseline: none`; its iterations rely on it.
  The files of earlier passes keep their lines `Baseline:` and `Departure:`, since you never edit their body.
- Write each evidence one sentence per line, with the builder's words.
- A task or finding of the prompt that the report leaves out keeps its status: say so in `Pending`.

### Verifier prompt

Send the verifier this block at the final verification, its only launch, in the language of the session:

```text
Plan: <path of the plan file>
Task: final
Previous: <the path of the file of the iteration the builder just worked on, or "none">
Skipped: <the `FND-NN` of that iteration the builder skipped, with why, or "none">
```

### Reviewer prompt

Send the reviewer this block, in the language of the session:

```text
Plan: <path of the plan file>
Task: <the id of the task whose cycle runs, or "final" for the fixes of the final verification>
Previous: <in the final verification, the path of the file of the iteration the builder just worked on; else "none">
Skipped: <the `FND-NN` of that iteration the builder skipped, with why, or "none">
Repeats: <the `FND-NN` that a later finding of the passes `Repeats`, with the guarantee they break, or "none">
Rules: <the path of the folder of rules, or "none" when it does not exist>
```

### Status

Edit only the `status` of the frontmatter of the plan:

- `in-progress` right before the first builder.
- `review` at the end when two things hold, with or without findings noted.
  Every task is `done` in `tasks` and has its commit.
  The final verification left nothing open: every AC `met`, or a manual `not verified` with its steps in `Pending`.
  A finding of the verifier a builder skipped twice for the same reason does not hold it.
- `blocked` at the end otherwise.
- `done` only when the user closes the plan in the triage (see `Triage`).

### Triage

With the plan `review`, ask the user what to do with its findings, and repeat until they close it:

1. **Ask**: right under the report, ask one question as text with the template of `grill-me`
   ([question.md](../grill-me/question.md)), saying the findings are just above it.
   On a plan that was already `review` at Step 1, give first the findings as `Findings report` says.
   The options:
   - **Fix all**: every `blocking` and `major` finding still `noted`; recommended when there is one.
   - **Fix some**: the user names them, by `FND-NN` or in their words.
   - **Close**: recommended when no `blocking` nor `major` finding is still `noted`.
   Never offer to leave the plan in `review` for later: the triage ends only when the user closes it.
2. **Fix**: add an iteration of `final` whose findings are the ones the user chose, each copied from the file of the
   pass that noted it, `Repeats` naming that `FND-NN` (see `Iteration`).
   Mark each chosen finding `open` in the file that noted it, then `fixed` or `skipped` as the iteration closes it.
   Set the plan `in-progress` and run `Final verification` from its step 6, then Steps 6 to 8 again, and ask again.
   When the loop stops, the plan ends `blocked`, as at Step 6.
3. **Close**: set the plan `done` (see `Status`), commit it (see `Commits`), say its hash, and stop.

### Findings report

The findings of the reviewer wait for the user, who decides which become new tasks of the plan.
Give in `Findings` of `Output` the ones the reviewer noted in this run, from the files of its passes:

- The `blocking` and `major` ones in detail, grouped by root cause: one item per cause, with its severity, the
  `FND-NN` and `file:line` of each instance, the problem and the direction of the fix.
  Group two findings only when one fix closes both; otherwise give each its own item.
- The `minor` ones only counted, with the paths of the files of the passes that hold them.
- "none" when the reviewer noted nothing.

### Acceptance criteria

Only the final verification evaluates the criteria; after each of its checks, take them as they are:

1. Each AC is `met`, `not met` or `not verified`, with its evidence; a manual step goes to `Pending` for the user.
2. Set the status of each criterion evaluated in `acceptance` of the plan, again on every run.
   Write its evidence, the one of the verification, under `Acceptance` of the file of the pass of `final` it checks:
   round 0 for the first check, else the iteration the builder just worked on.
3. A criterion no verification evaluated keeps `pending`.

### Stats

Gather the figures of this run from the plan and from git, and give them as a short list:

- **Iterations**: those of this run, and the plan's total, the files of the plan's passes from round 1 on.
- **Tasks**: done, departed and blocked, of the total, and the commits of the run.
- **Criteria**: met, not met, not verified and pending, of the total.
- **Findings**: the reviewer's noted ones by severity, and the others found, fixed and skipped, by `source`.
- **Agents**: how many times the builder, the verifier and the reviewer ran, and the model and effort of each.
- **Time**: the `start`, `end` and `duration` of `stats` of the plan, and its `iterations`, the runs of the build.
  Below it, one line per task and `final` with an entry in `timeline`, in the order of the plan.
  Each line has its time, the sum of its entries, and that time as a share of the sum of every entry, rounded to a
  whole percent.
  It also has its iterations, the files of the plan's passes for its id from round 1 on, across every run.
  Say that the shares are rounded.

```text
- Time: 2026-11-21 16:57 to 17:44, 47m 31s, 2 runs
  - T1: 5m 01s (11%), 0 iterations
  - T2: 12m 20s (26%), 1 iteration
  - T3: 22m 20s (48%), 0 iterations
  - T4: 6m 15s (13%), 0 iterations
  - final: 1m 04s (2%), 2 iterations
```
- **Code**: the `files` and `lines` of `code` of `stats` of the plan, and the lines added and removed apart.
