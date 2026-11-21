---
name: build
description: >
  Implements a plan task by task: for each task, polix:builder builds it, polix:verifier and polix:reviewer check it,
  and the build commits it once they close it. Then it updates the docs and polix:verifier checks the whole plan.
  It runs unattended, records each iteration in the plan, never pushes, and returns the stats of the build.
  It runs only when the user or the workflow of polix asks for it.
effort: low
model: sonnet
user-invocable: true
disable-model-invocation: true
argument-hint: "[plan and optional first task, e.g. add-retries.yaml T3; empty to take it from the conversation]"
---

# Building

Run a plan to its end, unattended: once it starts, it ends `done`, or `blocked` for what only a person can solve.
You coordinate and never build: each task runs its own cycle, so every agent sees one task and a small context.
In the cycle of a task, the builder builds it and reports, you write the report in the plan, and the verifier and
the reviewer check that task alone, in parallel.
When the checks leave something to fix, you add an iteration of that task to the plan and launch the builder on it.
When the checks close the task, you commit it, and the next task starts from that commit.
Once every task is closed, you update the docs, and the verifier checks the whole plan.
You are the only one that edits the plan and commits: the subagents read it and report.
The folder of plans is the one the start context of polix names, below the root.

## Requirements

- A plan file of the folder of plans, in YAML with a list `tasks` of ids `T1`, `T2`..., as the skill plan writes it.
  A plan in Markdown (`.md`) is no longer built: stop and offer `/polix:plan` to write it again as YAML.
  Without a plan, stop and offer `/polix:plan`: never build without a plan nor write one yourself.
- The subagents `polix:builder`, `polix:verifier` and `polix:reviewer`.
  `polix:verifier` checks that the change meets the plan, and `polix:reviewer` that its code is well written.
  Without them, as in Codex, read `builder.md`, `verifier.md` and `reviewer.md` in `../../agents/`, next to this skill.
  Follow each yourself when its turn comes, with the `subagent_start` injects of `polix.yaml` for it.
  The rules of this skill win over theirs, and the checks are not independent: say so in the report.

## Input

- **Plan**: the path from the arguments, or the plan named, written or discussed in the conversation.
  Without either, list the plans of the folder of plans and ask the user which one; never pick one yourself.
- **From**: the task from the arguments, as `T3` or `3`, to start there.
  Without it, the first task whose `status` is not `done`, so a task `departed` or `blocked` is tried again.
- **Clean tree**: run `git status --porcelain`.
  With changes other than the plan file, stop and ask the user to commit or stash them.
  Each commit of the build holds only its task, so the run never starts on changes of others.
  Never stash, commit nor discard them yourself.
  Only a plan `in-progress` starts with changes: they are the work of the run it resumes (see `Resume`).
- **Branch**: the `branch` of the plan, the branch it was planned on and is built on.
  When the current branch (`git branch --show-current`) is another one, switch to it with `git switch <branch>`.
  When the branch exists neither locally nor on a remote, stop and tell the user; never create it.
  A plan without `branch` builds on the current branch.
- **Approval**: a plan with `status: planned` is not approved.
  Stop and ask the user to approve it with `/polix:plan <plan>`; never approve it yourself.
- **Resume**: a plan `in-progress` was interrupted: resume it where it stopped (see `Resume`).
  When every task is `done` and the plan is `done`, tell the user and stop.
  A plan `approved` or `blocked` builds from `From`; its earlier iterations stay.

## Output

- **Plan**: the path of the plan file, its `status` and the branch it was built on.
- **Tasks**: each task with its `status`: `done` with its evidence and commit, `departed` or `blocked` and why.
- **Commits**: each commit of the run, its short hash and subject; none is pushed.
- **Changes**: the files changed, by task and by finding.
- **Docs**: each Markdown file updated and what changed in it, or why none needed it.
- **Acceptance**: each AC with `met`, `not met`, `not verified` or `pending` and its evidence.
- **Blockers**: when the plan ends `blocked`, each reason and what a person must do to clear it; or "none".
  Name the uncommitted changes the run left, and the task or the final verification they belong to.
- **Pending**: the minor findings, the manual checks and the failures that are not the change's.
  Also the changes `AGENTS.md`, `CLAUDE.md` or a protected file need; or "none".
- **Stats**: the build in figures (see `Stats`).

## Limits

- Ask only in `Input`, before Step 3: from then on, never ask the user nor wait for an answer.
  Decide what is open as the builder would (see its `Departures`), and record it.
- Never end your turn before Step 9: a subagent that returns, a failed check or a long run is never the end.
  The run ends only when the plan is done or a loop stops (see `Task cycle` and `Final verification`).
- Never edit the code yourself: only the builder changes it, except in the fallback of `Requirements`.
  You edit only the Markdown docs of the project (see `Docs`) and five parts of the plan.
  Those are its `status`, the `status` and `evidence` of each task and of each criterion.
  Also the iterations you add, and the fields of `stats` (see `Plan stats`).
  Of the iterations, you set their `status` and the `status` and `evidence` of their findings.
- Edit the plan with targeted edits that keep it valid YAML, never writing it again whole.
- Commit only as `Commits` says: never push, stash, reset, amend, rebase nor create branches.
  The only checkout is the switch to the plan's `branch` in Step 1 (see `Input`).
- Write the status of the tasks and findings as the builder reports them, never your own.
  Still, count nothing as closed on the builder's word: only the verifier's and the reviewer's reports close it.
- Never run a command that installs, generates or formats, even when the plan or `AGENTS.md` lists it.
- Add the findings the verifier, the reviewer and a failed commit report as they are: none of your own, none dropped.
- Talk to the user in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Build progress:
- [ ] Step 1: Resolve the plan, the clean tree, its branch, its approval, the first task and resume (see `Input`).
  Read the plan whole.
- [ ] Step 2: Show the user the tasks to build, in order, and go on without waiting.
- [ ] Step 3: Set the plan `in-progress` (see `Status`), its `stats.start` and `stats.iterations` (see `Plan stats`).
- [ ] Step 4: Run the cycle of each task from `From`, in order (see `Task cycle`).
  When a cycle stops, go to Step 7.
- [ ] Step 5: Update the docs and commit them (see `Docs` and `Commits`).
- [ ] Step 6: Verify the whole plan and fix until nothing is open or the loop stops (see `Final verification`).
- [ ] Step 7: Set the plan `done` or `blocked` (see `Status`).
  Set its `stats.end`, `stats.duration`, `stats.tokens` and `stats.code` (see `Plan stats`).
  When `done`, commit the plan and what the final verification fixed (see `Commits`).
- [ ] Step 8: Gather the stats of the build (see `Stats`).
- [ ] Step 9: Report (see `Output`), stop and return control to the user.
```

## References

### Task cycle

Run each task from `From` to the last, one at a time, in the order of the plan; skip a task `done`.

1. **Build**: launch the builder on the task (see `Models` and `Builder prompt`).
   Write its report in the plan (see `Builder report`).
   When it left the task `not started`, launch it again on the task once; a second time, the task is open.
2. **Checks**: launch the verifier and the reviewer on the task in one message, so they run in parallel.
   Wait for both (see `Verifier prompt` and `Reviewer prompt`).
   Set the iteration the builder worked on, if any, to `status: checked`.
3. **Criteria**: set the criteria the verification evaluated (see `Acceptance criteria`).
4. **Open**: list what is still open of the task.
   - The task, when it is not `done`.
   - The verifier's findings, its criteria `not met` among them.
   - The reviewer's `blocking` and `major` findings; its `minor` ones go to `Pending`.
   - Leave out a finding the builder skipped twice for the same reason: it goes to `Pending`.
5. **Close**: with nothing open, commit the task (see `Commits`), and go on with the next task.
6. **Stop**: the cycle stops in any of these cases, each to `Blockers` with what a person must do.
   - The task is `blocked`: it waits for a person.
   - The last two iterations of the task fixed nothing and did not turn it `done`: the work stalled.
   - The task reached five iterations: the guard against a fix that breaks something else every time.
   The task's changes stay uncommitted, no later task is built, and the commits of the earlier tasks stay.
7. **Iteration**: otherwise, add an iteration of the task with what is open (see `Iteration`).
   Launch the builder on it (see `Builder prompt`) and write its report in the plan (see `Builder report`).
   Set it `status: built`, and go back to 2.

A criterion covered by several tasks is evaluated at the last task that covers it; until then it stays `pending`.

### Final verification

Once every task is closed and the docs committed, check the whole plan, and repeat until nothing is open:

1. **Check**: launch the verifier with `Task: final` (see `Verifier prompt`); the reviewer does not run here.
   Set the iteration the builder worked on, if any, to `status: checked`.
2. **Criteria**: set every criterion from the verification (see `Acceptance criteria`).
3. **Open**: the verifier's findings, its criteria `not met` among them.
4. **Done**: with nothing open, the loop ends.
5. **Stop**: as in `Task cycle`, after two iterations that fixed nothing or after five iterations of `final`.
   Its fixes stay uncommitted.
6. **Iteration**: otherwise, add an iteration with `task: final` and what is open (see `Iteration`).
   Launch the builder on it and write its report in the plan, set it `status: built`, and go back to 1.

### Iteration

An iteration is one pass of the builder on what the checks left open, appended to `iterations` of the plan:

- `id`: `ITER-01`, `ITER-02`..., the number of items plus one; the prompts and later findings name it.
- `date`: today as YYYY-MM-DD.
- `status`: `open` while the builder works on it, `built` once it returns, `checked` once the checks ran.
- `task`: the id of the task whose cycle it belongs to, or `final` for the final verification.
- `findings`: one item per finding to fix, with its own id, `FND-NN`, numbered across the whole plan.
  - `source`: `verification`, `review` or `commit`.
  - `kind`: the area of a finding of the verification, the severity and category of one of the review, or the check
    that failed the commit.
  - `where`: the `file:line` or the id of the plan it is about.
  - `problem` and `fix`: what is wrong and what to change, from the report, as literal blocks.
  - `repeats`: the `FND-NN` of an earlier iteration the checks report again, or `none`.
    So is one whose report breaks the same guarantee by another path, such as a second way to read outside the root.
  - `status: open` and `evidence: none`: you set `fixed` or `skipped`, with why, from the builder's report.

Copy each finding from its report as it is: never merge, reword nor judge it.
The criteria go to `acceptance`, never to the iteration, and the rest of the reports goes to `Output`.
An iteration of a plan built before this format has `tasks` instead of `task`: keep it as it is, as history.

Two checks of a task, the first leaving a finding of each kind, and the iteration that fixed them:

```yaml
iterations:
  - id: ITER-01
    date: 2026-11-14
    status: checked
    task: T3
    findings:
      - id: FND-01
        source: verification
        kind: criterion
        where: internal/export/writer.go:31
        problem: |
          CUJ-03 asks the rows in the order of LIT-04, and they come unsorted.
        fix: |
          Sort the invoices by date before writing them.
        repeats: none
        status: fixed
        evidence: |
          `writer.go` sorts by date; `TestExportRowsSorted` failed before the fix and passes.
      - id: FND-02
        source: review
        kind: major, error handling
        where: internal/export/flags.go:40
        problem: |
          A date that does not parse panics instead of returning the error of LIT-03.
        fix: |
          Return the error of the parser wrapped with the flag name.
        repeats: none
        status: fixed
        evidence: |
          `TestExportDates` covers `2026-02-30`; it panicked before the fix and passes.
```

### Commits

You commit, never a subagent; each commit holds the plan file too, so the plan in `HEAD` shows the progress.

- **Task**: once its checks close it.
  Subject `T<N>: <title of the task>`, then a body with its `summary` and the line `Plan: <path of the plan>`.
- **Docs**: at Step 5, when a doc changed: subject `Docs: <title of the plan>`, and the line `Plan: <path>`.
- **Final**: at Step 7 when `done`: subject `Final: <title of the plan>`, a body with the `FND-NN` the final
  verification fixed, or "Nothing to fix.", and the line `Plan: <path>`.

How to commit:

1. List the changed and untracked paths with `git status --porcelain`: the tree was clean, so all are the run's.
   Stage them by path with `git add <paths>`, never `git add -A`, `-a` nor a path after `git commit`.
2. Commit with `git commit -m "<subject>" -m "<body>"`, run by its name and ending the line.
   The message holds only those lines: no trailer, no attribution.
   Never pass `--no-verify`: the hooks of the project are part of the checks.
3. When polix blocks the commit with a reminder that it gives only once, such as updating the docs, commit again once.
   The docs are Step 5's, so a task's commit never stops for them.
4. When a hook or a check fails the commit, the failure is a finding with `source: commit`.
   Add it to an iteration of the task, or of `final`, and go on with its loop: it counts toward its limit.
5. When a block asks for what only a person can do, stop the loop as `Task cycle` does.
6. Take the short hash with `git rev-parse --short HEAD` for `Output`; never push.

### Resume

A plan `in-progress` was interrupted; the commits of the branch say which tasks are closed.

- With a last iteration `open`, launch the builder on it; with one `built`, run its checks.
- Otherwise, with uncommitted changes, they belong to the first task without its commit `T<N>: ` in
  `git log --format=%s $(git merge-base HEAD <base>)..HEAD`, or to `final` when every task has one.
  Run the checks of that task, or of the final verification, on them; their findings start its iterations.
- Otherwise, run the cycle from the first task not `done`, or the docs and the final verification when all are.
- The base is the `base_branch` of `polix.yaml`, else `main`.

### Plan stats

The fields of `stats`, at the root of the plan, record the build as a whole across every run, a resumed one too.
A plan written before `stats` holds `start`, `end`, `duration` and `tokens` at its root.
Move them under `stats`, keeping their values, and add the fields it lacks.

- `start`: at Step 3, the current time from the shell, as `YYYY-MM-DDTHH:MM:SS+HH:MM` with the local offset.
  Take it from a command, such as `date -Iseconds` or `Get-Date -Format "yyyy-MM-ddTHH:mm:sszzz"`; never guess it.
  Set it only when it is `none`: a resumed plan keeps the `start` of its first run.
- `end`: at Step 7, the current time the same way; a later run of the plan writes it again.
- `duration`: `end` minus `start`, as `1h 05m 12s`; it includes the time the plan stood interrupted.
- `tokens`: the `totalTokens` of the usage each subagent reports when it returns, added to `stats.tokens`.
  Start from `0` when it is `none`, and add each subagent as it returns, never estimated.
  They leave out your own tokens, which no report gives: say so in `Stats`.
  When a subagent reports no usage, as in the fallback of `Requirements`, set it `unknown` and add no more.
- `iterations`: at Step 3, the runs of the build on the plan: `1` when it is `none`, else one more.
  Every run counts, a resumed one too; it is not the number of items of the key `iterations` of the plan.
- `code`: at Step 7, the `files` changed and the `lines` added plus removed on the branch, leaving out the plan.
  The commits of the build moved `HEAD`, so measure against the base of `Resume`.
  Take them from `git diff --shortstat $(git merge-base HEAD <base>) -- . ':(exclude)<plan>'`.
  Add the untracked files of `git ls-files --others --exclude-standard`, each line of one as added.
  Never stage them to measure them.

```yaml
stats:
  start: 2026-11-14T21:05:40+01:00
  end: 2026-11-14T21:48:02+01:00
  duration: 42m 22s
  tokens: 491603
  iterations: 2
  code:
    files: 7
    lines: 360
```

### Models

Launch each subagent with the `model` and `effort` of its item in `agents` of the plan.
Its item is the one whose `name` is its type, and both go as options of the launch.

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
Iteration: <the `ITER-NN` of the plan to work on, its task and findings, or "none">
```

### Builder report

The builder never edits the plan: it reports what it built, and you write it in the plan once it returns.

- **Task**: set the task's `status` and `evidence` as the builder reports them.
  `done`, `departed` or `blocked`; a task `not started` keeps its `status`.
- **Findings**: for each finding of its `Findings`, set its `status` (`fixed` or `skipped`) and `evidence`.
- Write each `evidence` as a literal block `|`, one sentence per line, with the builder's words.
- A task or finding of the prompt that the report leaves out keeps its `status`: say so in `Pending`.

### Verifier prompt

Send the verifier this block, in the language of the session:

```text
Plan: <path of the plan file>
Task: <the id of the task whose cycle runs, or "final" for the whole plan>
Previous: <the `ITER-NN` of the plan the builder just worked on, or "none">
```

### Reviewer prompt

Send the reviewer this block, in the language of the session:

```text
Plan: <path of the plan file>
Task: <the id of the task whose cycle runs>
Previous: <the `ITER-NN` of the plan the builder just worked on, or "none">
Skipped: <the `FND-NN` of that iteration the builder skipped, with why, or "none">
Repeats: <the `FND-NN` that a later finding of the plan `repeats`, with the guarantee they break, or "none">
```

### Status

Edit only the top-level `status` of the plan:

- `in-progress` right before the first builder.
- `done` at the end when three things hold.
  Every task is `done` and has its commit.
  The final verification is `met`: every AC `met`, or a manual `not verified` with its steps in `Pending`.
  The review of each task left no `blocking` nor `major` finding, but those a builder skipped twice for the same reason.
- `blocked` at the end otherwise.

### Docs

Once every task is closed, bring the docs in line with the whole change, in one pass:

1. Read the diff of the branch against the base of `Resume`, and list what a reader of the docs would see differently.
   That is behaviour, commands, flags, configuration, contracts, files and folders, install or setup steps.
2. Find the Markdown files that describe it with `rg`: `README.md`, the folder of docs and `CHANGELOG.md`.
   The folder of docs is `docs/` or what `AGENTS.md` names.
   Include any other `.md` of the project, except the folders of plans, reports and rules.
3. Update each passage that the change made stale or incomplete.
   Add an entry to the unreleased section of the changelog when the project keeps one.
   Follow the style, language and line format of each file; write nothing the change does not support.
4. Never edit `AGENTS.md`, `CLAUDE.md` nor a file protected by `polix.yaml`: the instructions belong to a person.
   Write the change they need in `Pending`.

Leave the docs alone when the change alters nothing a reader sees, and say so in `Docs` of `Output`.
The final verification checks them, and its fixes to the docs go in the final commit.

### Acceptance criteria

After each verification, take the criteria it evaluated as they are:

1. Each AC is `met`, `not met` or `not verified`, with its evidence; a manual step goes to `Pending` for the user.
   The verification of a task evaluates only the criteria whose last covering task it is; leave the others alone.
2. Set the `status` and `evidence` of each criterion evaluated in `acceptance`, again on every run.
   The evidence is the one of the verification, in one line or a literal block.
3. A criterion no verification evaluated keeps `status: pending`.

### Stats

Gather the figures of this run from the plan and from git, and give them as a short list:

- **Iterations**: those of this run, by task and `final`, and the plan's total, the items of `iterations`.
- **Tasks**: done, departed and blocked, of the total, and the commits of the run.
- **Criteria**: met, not met, not verified and pending, of the total.
- **Findings**: of the iterations, found, fixed and skipped, by `source`, and the reviewer's `minor` ones apart.
- **Agents**: how many times the builder, the verifier and the reviewer ran, and the model and effort of each.
- **Time**: the `start`, `end` and `duration` of `stats` of the plan, and its `iterations`, the runs of the build.
- **Tokens**: the tokens of this run by agent, and the `tokens` of `stats` of the plan.
  Say that they count the subagents only, not this skill's own.
- **Code**: the `files` and `lines` of `code` of `stats` of the plan, and the lines added and removed apart.
