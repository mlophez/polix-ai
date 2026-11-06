# Conditions on the size and the risk of the uncommitted diff, to ask for a review only when it is worth it

## Overview

Today the gate before a commit asks for a review of polix:reviewer whenever any Go file changed, even a one-line fix.
This change lets a workflow measure the change not committed yet, and ask for the review only past a threshold.

- Three new conditions measure the uncommitted change: how many lines changed, how many files, and a risk score.
  They are named `changed_lines`, `changed_files` and `risk_score`, and each one says its minimum as `at_least`.
- They measure the files that the `changed` condition next to them selects, so the globs are written once.
- The risk score is a fixed, deterministic formula: each file adds its weight times the logarithm of its lines.
  Weights per path are set in polix.yaml, so the core of the engine counts more than the docs.
  Any other file weighs 1.
- A new generic condition, `any`, holds when any of the conditions inside it holds, so the gate can say
  "at least 150 lines, or at least 5 files, or a risk of at least 25".
- The texts of the workflow can show the measures, so the message to the agent says why the review is asked.
- A new command, `polix risk`, prints the measures of the current change file by file, to calibrate them.
  With `--commit` it measures a commit already made instead, so thresholds and weights can be tuned on the history.
- The workflow of this repository adopts it: small Go changes commit without a review, large or risky ones do not.
- When the change cannot be measured, the conditions hold and polix warns, so a failure never skips a review.

## Context

The `pre_commit` gate of `polix.yaml` denies a commit of Go code until polix:reviewer reviewed that exact content.
Trivial changes pay the cost of a full review, and polix has no condition on the size or the risk of a change.
The outcome is a set of measure conditions, an `any` condition, measure placeholders and a command to calibrate them.

## Decisions

- The measure conditions are `changed_lines`, `changed_files` and `risk_score`, named after the family of `changed`.
- Their value is always a map with one key, `at_least`, a non-negative integer; they hold at or above it.
  There is no short form: a bare number would not say whether it is a minimum, a maximum or an exact value.
- They have no globs of their own: they measure the files matching the nearest `changed` condition.
  The nearest is the one in the same map, else the one of the enclosing map (`any` and `not` nest maps).
  Without any `changed` around them, they measure the whole uncommitted diff.
- The conditions combine through a new generic condition `any`, the OR of the conditions of its map, on any event.
- The diff is what `changed` sees: the diff against HEAD plus the untracked files not ignored.
- An untracked file counts all its lines as added; a binary file counts as a changed file with zero lines.
- Without a commit in the repository, every tracked and untracked file counts whole, as `changed` lists them all.
- The risk score is the sum over the measured files of weight times log2(1 + added + deleted lines), rounded.
- The weights live in a new top-level section `risk`, under its key `weights`: an ordered map of glob to
  non-negative number, written `risk.weights` in the docs. The section leaves room for other options of the score.
  The first glob that matches a file, in the order of the file, gives its weight.
  A file that no glob matches weighs 1.
- A diff that cannot be measured makes the three measure conditions hold, with one warning per call.
  It fails closed, as `done` does with an unknown content hash, so a broken git never skips a review.
- The measures are read once per call, like `changed`, not once per epoch.
- `{changed_lines}`, `{changed_files}` and `{risk_score}` become built-in placeholders.
  They measure the files of the `changed` condition of the `if` of the entry, else the whole uncommitted diff,
  so a message shows the same numbers its thresholds compare.
  The diff is read only when a string uses one of them.
- `polix risk` is a new command of the group of the repository, backed by a new use case, `MeasureDiff`.
  Its repeatable flag `--changed` takes globs with the meaning of the condition `changed`.
- `polix risk --commit <rev>` measures the changes that commit introduced: its diff against its first parent,
  or against the empty tree for a root commit, so a merge measures what it brought into the branch.
  It takes any revision git resolves to a commit (a hash, `HEAD~3`, a tag), one per call.
  The weights come from the current `polix.yaml`, not from the one of the commit, so a new weighting is tried on
  old commits.
  Without `--commit` it measures the uncommitted diff, as the conditions do.
- Assumption: the measures live in a new domain package, `internal/diff`, whose entity is `Diff`.
- Assumption: the thresholds of this repository start at 150 lines, 5 files or a risk score of 25 over the Go files.
  They use the weights of Task 7 and are tuned with `polix risk`.
- Assumption: `polix risk` prints text only, with no `--json`.

## Approach

`pkg/git` gains a method that returns, for each uncommitted file, its added and deleted lines and whether it is binary.
It reads the tracked files with `git diff --numstat` against HEAD and counts the lines of the untracked files itself.

`internal/diff` holds the pure measures: the entity `Diff` (its `Changes`, one `Change` per file) and the `Weight` of a
glob, with the methods that narrow a diff to some globs, sum its lines, count its files and compute its risk score.

`internal/workflow` parses `risk.weights` into `Workflow.Weights`.
It accepts the conditions `changed_lines`, `changed_files`, `risk_score` and `any` and validates their values.
It adds `changed_lines`, `changed_files` and `risk_score` to the built-in placeholders.

`internal/hook` learns the diff as a new fact of the evaluation, asked by a new step and read by a new port method.
While it evaluates a map of conditions it carries the globs of the nearest `changed` into the nested maps.
It evaluates the new conditions on the diff narrowed to those globs and expands the new placeholders.
It warns once when the diff cannot be measured.

`internal/diff` also holds the use case `MeasureDiff`, behind `polix risk`.
It reads the diff and the weights and prints the totals and the contribution of each file.

Contracts that change:

- The schema of `polix.yaml`: section `risk` with `weights`,
  conditions `changed_lines`, `changed_files`, `risk_score` and `any`, and three placeholders.
- The command line: `polix risk`, with `--changed` and `--commit`.
- The port `AnswerHookPort` (method `Diff`), and the `Step` and `Facts` of the hook.

Errors: a failure of git while measuring reaches the hook as the error of `Diff`, kept in the facts as a warning.
`polix risk` prints that error and exits with 1.
Invalid values are problems of `polix check`, never runtime errors.

### Discarded alternatives

- Short names with a bare number (`lines: 150`): the YAML does not say what is measured nor that it is a minimum.
- Globs on each measure (`in:`): they repeat the globs of the `changed` next to them.
- One condition `diff` grouping the measures with an inner OR: an OR inside a condition, unlike the rest of polix.
- The comparator in the name (`min_changed_lines`): a maximum one day would multiply the keys.
- AND of the three conditions: the gate wants "large or risky", and an OR through nested `not` is unreadable.
- Only a risk score: line and file thresholds can be read without knowing the formula.
- Counting only the staged diff: it differs from `changed`, and falls short with `git commit -a`.
- A fixed formula without weights: every repository has a different critical core.
- An external command that prints the risk: one more process per evaluation, and the formula leaves polix.
- Highest matching weight instead of the first: a test of a critical folder could not weigh less than that folder.

## Acceptance criteria

- AC1: `changed_lines: { at_least: N }` holds when the added plus deleted lines of the measured files are at least N.
  The measured files are the uncommitted ones, untracked included, matching the nearest `changed`, or all of them.
- AC2: `changed_files: { at_least: N }` holds when at least N measured files changed, deleted and binary included.
- AC3: `risk_score: { at_least: N }` holds when the rounded sum of weight times log2(1 + changed lines) over the
  measured files is at least N; the weight of a file is that of the first glob of `risk.weights` matching it, else 1.
- AC4: `any` holds when at least one of its conditions holds, it may nest inside `not` and hold `not`.
  Its nested conditions are validated as the conditions of an `if`, and a measure inside it takes the scope of the
  `changed` of the enclosing map unless its own map has a `changed`.
- AC5: `polix check` reports a measure value that is not a map, a map without `at_least`, an `at_least` that is not
  a non-negative integer, an unknown key in that map, and an empty `any`.
  It also reports a `risk` or a `risk.weights` that is not a map, an unknown key under `risk`,
  and a weight with an empty glob, a value that is not a number, or a negative value.
- AC6: when the diff cannot be measured, the three measure conditions hold and the call warns once, naming the cause.
- AC7: `{changed_lines}`, `{changed_files}` and `{risk_score}` expand in every string of an entry to the measures
  over the `changed` of its `if`, else the whole diff.
  The diff is not read when no condition nor string needs it, and a variable cannot take those names.
- AC8: `polix risk` prints the lines, files and risk score of the uncommitted diff, then one line per file.
  Each line has the added and deleted lines, the weight and the contribution, sorted by contribution, then path.
  `--changed` narrows the measure as the condition `changed` does.
- AC9: in this repository a Go change below the thresholds commits without a review.
  One above them is denied with a message that shows its measures.
- AC10: `docs/polix.md`, `examples/polix.yaml`, the reference of the skill workflow and the documents of the use
  cases describe the new key, conditions, placeholders, step, port method and command.
- AC11: `polix risk --commit <rev>` prints the same report for the changes of that commit against its first parent,
  a root commit against the empty tree, with the weights of the current `polix.yaml` and narrowed by `--changed`.
  A revision that is not a commit prints the cause and exits with 1; the uncommitted changes do not count.

## Out of scope

Signals beyond lines and paths, such as deleted files, changed dependencies or code without tests.
The weights cover paths, and more signals would make the formula harder to calibrate.
A maximum (`at_most`): the map leaves room for it, but no gate needs it yet.
Ranges of commits in `polix risk` (`--commit A..B`) and a summary over many commits: one commit per call is enough
to calibrate, and a loop of the shell over `git log` covers a range.
Measures per epoch: `changed` is read once per call too, and a run that rewrites files on `pre_commit` is rare.
The update of the layout of `AGENTS.md`, which is protected: only a person changes it (see `Pending`).

## Tasks

### Task 1: Per file line counts of the uncommitted diff and of a commit in pkg/git

- Covers: AC1, AC2, AC11.
- Files: `pkg/git/git.go` (`Repo`, new methods `Stats` and `CommitStats`, new type `Stat`, modify),
  `pkg/git/git_test.go` (modify).
- Rules: GOARCH-7, GOARCH-13, GOARCH-19, GOARCH-35, GOARCH-38, GOARCH-39, GO-2, GO-3, XPLAT-1.
- Reuse: `Repo.paths`, `Repo.git`, `addPaths` and the HEAD check of `Repo.Changed` from `pkg/git/git.go`.
- Changes: add a type `Stat` with the path relative to the root with slashes, added lines, deleted lines and binary.
  Add `Repo.Stats(ctx, ref)`, with the same set of files as `Repo.Changed` and an empty ref meaning HEAD.
  With a HEAD, it reads `git diff --numstat -z --no-renames` against the ref, where a `-` count marks a binary file.
  It adds the untracked files not ignored, read from the working tree, every line counted as added.
  Without a HEAD, every file of `ls-files` and every untracked file is read whole the same way.
  A file read whole is binary when its first 8000 bytes hold a NUL byte, as git decides.
  Its lines are its newlines, plus one when it is not empty and does not end in a newline.
  A file that vanished between listing and reading is left out.
  The result is sorted by path; any other failure of git or of a read is returned as an error.
  Add `Repo.CommitStats(ctx, rev)`, which first resolves the revision to a commit and fails naming it otherwise.
  It reads the numstat of that commit against its first parent, or against the empty tree for a root commit,
  with renames off and binary files marked as in `Stats`; the working tree and the untracked files do not count.
  Its result is sorted by path, as that of `Stats`.
- Tests: in `pkg/git/git_test.go`, on a temporary repository: a modified file with added and deleted lines,
  a deleted file, an untracked file without a final newline, an untracked binary file, an ignored file left out,
  a repository without commits, and a path with spaces and non-ASCII characters.
  For `CommitStats`: a normal commit, a root commit, a merge commit measured against its first parent,
  a binary file in a commit, uncommitted changes left out, and a revision that does not exist.
- Verification: `just test -run 'TestStats|TestCommitStats'`, expected: the new tests pass.

### Task 2: The domain of the measures, internal/diff

- Covers: AC1, AC2, AC3.
- Files: `internal/diff/diff.go` (create), `internal/diff/diff_test.go` (create).
- Rules: GOARCH-1, GOARCH-2, GOARCH-4, GOARCH-7, GOARCH-8, GOARCH-9, GOARCH-13, GOARCH-17, GOARCH-18, GOARCH-25,
  GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-29, GOARCH-30, GOARCH-35, GOARCH-38, GOARCH-39, GOARCH-40, GO-2, GO-3,
  XPLAT-1, DOC-1.
- Reuse: `glob.AnyPath` from `pkg/glob`.
- Changes: create the package with its doc comment and the entity `Diff`, whose `Changes` hold one `Change` per file.
  `Change` has `Path`, `Added`, `Deleted` and `Binary`; `Weight` has `Glob` and `Factor`.
  `Contribution` has `Path`, `Lines`, `Factor` and `Score`, the share of one file in the risk score.
  Each struct has `Valid`: a change needs a path and non-negative counts, and a binary change has zero counts.
  A diff has no repeated path, and a weight needs a glob and a finite, non-negative factor.
  `Diff.Narrow(patterns)` keeps the changes matching any of the globs, and the whole diff for no globs.
  `Diff.Lines` sums added and deleted lines; `Diff.Count` returns the number of changed files.
  `Diff.Contributions(weights)` returns one contribution per change, sorted by score, highest first, then by path.
  The factor of a change is that of the first matching weight, else 1, and its score is factor times log2(1 + lines).
  `Diff.Risk(weights)` is the sum of the scores, rounded half away from zero to an integer.
- Tests: in `internal/diff/diff_test.go`: `Narrow` with no globs, a base name glob and an anchored glob;
  `Lines` and `Count` with binary and deleted files; `Risk` with the first matching weight winning over a later one,
  a zero weight, an unmatched file weighing 1, an empty diff scoring 0 and a rounding at .5;
  the order of `Contributions`; and each `Valid`.
- Verification: `just test -run 'TestDiff|TestChange|TestWeight'`, expected: the new tests pass,
  and `just test -run 'TestGOARCH'` stays green.

### Task 3: The schema: risk weights, measure conditions, any and the measure placeholders

- Covers: AC1, AC2, AC3, AC4, AC5, AC7.
- Files: `internal/workflow/workflow.go` (`Workflow`, `Conditions`, `Builtins`, modify),
  `internal/workflow/workflow_parse.go` (`workflowYAML`, `topKeys`, new decoding of `risk`, modify),
  `internal/workflow/workflow_entry.go` (`validator.cond`, `Nested`, new `Measure` and `Measured`, modify),
  `internal/workflow/workflow_test.go`, `internal/workflow/workflow_parse_test.go`,
  `internal/workflow/workflow_entry_test.go` (modify).
- Rules: GOARCH-1, GOARCH-4, GOARCH-9, GOARCH-13, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-29,
  GOARCH-30, GOARCH-35, GOARCH-38, GOARCH-40, GO-2, GO-3, DOC-1.
- Reuse: the ordered decoding of `runnersYAML` and `mapKeys` from `internal/workflow/workflow_parse.go`,
  `Nested` and `Strings` from `internal/workflow/workflow_entry.go`, and `diff.Weight` from Task 2.
- Changes: `Workflow` gains `Weights`, a list of `diff.Weight` in the order of the file.
  It is decoded from the top-level section `risk`, whose only key is `weights`, an ordered map of glob to number;
  `topKeys` gains `risk`.
  `Problems` reports a `risk` or a `weights` that is not a map, an unknown key under `risk`,
  and the problems of each weight.
  `Conditions` gains `changed_lines`, `changed_files`, `risk_score` and `any`; none is limited by `conditionEvents`.
  A new domain type `Measure`, with `AtLeast`, and a function `Measured` read the map of a measure condition.
  `validator.cond` reports a value that is not a map, a missing `at_least`, an `at_least` that is not a non-negative
  integer and an unknown key of the map.
  It validates `any` as `not`, recursively, and rejects an empty `any`.
  `Builtins` gains `changed_lines`, `changed_files` and `risk_score`, so a variable cannot take those names and the
  strings may use them.
- Tests: parse and validate a workflow with `risk.weights` and keep their order; reject each invalid case of AC5;
  accept each measure with `at_least`, `any` holding `not` and `not` holding `any`; reject a bare number as a measure;
  reject a var named `risk_score`; accept `{risk_score}` in a `deny`.
- Verification: `just test -run 'TestParse|TestProblems|TestCond'`, expected: the new and existing tests pass.

### Task 4: The hook measures the diff, evaluates the new conditions and expands the measures

- Covers: AC1, AC2, AC3, AC4, AC6, AC7.
- Files: `internal/hook/hook_step.go` (`Step`, `Step.Valid`, `Step.None`, modify),
  `internal/hook/hook_evaluation.go` (`Facts`, new `Evaluation.KnowDiff` and `Facts.learnDiff`, modify),
  `internal/hook/hook_condition.go` (`Expand`, modify),
  `internal/hook/answer_hook_logic.go` (`answerHookRun.matches`, `answerHookRun.condition`, `answerHookCost`,
  `answerHookRun.expand`, new `answerHookRun.diff`, modify),
  `internal/hook/answer_hook_in.go` (`AnswerHookPort`, the shell `answerHook`, modify),
  `internal/hook/answer_hook_out.go` (`AnswerHookAdapter`, new method `Diff`, modify),
  `internal/hook/answer_hook.md` (modify), and the tests next to each of them.
- Rules: GOARCH-4, GOARCH-9, GOARCH-10, GOARCH-12, GOARCH-13, GOARCH-18, GOARCH-20, GOARCH-21, GOARCH-23,
  GOARCH-24, GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-30, GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35, GOARCH-36,
  GOARCH-37, GOARCH-38, GOARCH-40, GO-2, GO-3, XPLAT-1, DOC-1.
- Reuse: the path of `Changed` across `Step`, `Facts.learnChanged`, `answerHookRun.changed`, the port and the adapter;
  `Facts.Warnings` and `appendNew` for the warning; `diff.Diff` and its methods from Task 2;
  `workflow.Measured` and `workflow.Nested` from Task 3; `Repo.Stats` from Task 1.
- Changes: `Step` gains `Diff`, asking for the measures of the uncommitted diff, counted in `Valid` and `None`.
  `Facts` gains `Diff`, `DiffErr` and `HasDiff`, learnt by `Evaluation.KnowDiff`.
  An error adds the warning "polix no puede medir el diff", followed by its cause in parentheses
  and ": changed_lines, changed_files y risk_score se cumplen".
  `AnswerHookPort` gains `Diff(ctx)`, returning a `diff.Diff` and an error.
  The adapter builds it from `Repo.Stats` with an empty ref and validates it, so an invalid diff is an error too.
  The shell answers the new step through the port, as it answers `Changed`.
  `answerHookRun.matches` takes the scope of the enclosing map, the globs of its nearest `changed` or none.
  A map with its own `changed` replaces that scope for its conditions and for the maps nested in its `any` and `not`.
  `answerHookRun.condition` evaluates the measures on the diff narrowed to the scope, comparing `Lines`, `Count` or
  `Risk` (with the weights of the workflow) against `at_least`; with `DiffErr` the three hold.
  It evaluates `any` as true when any condition of its nested map holds, each evaluated as `matches` does.
  `answerHookCost` ranks the three measures after `changed`, and `any` with `not`.
  `expand` asks for the diff only when the string holds one of the three placeholders, and measures it over the
  globs of the `changed` of the `if` of the entry, else the whole diff.
  `Expand` gains the measures to replace them, as integers, or `?` when the diff could not be measured.
  `answer_hook.md` names the new step, fact and port method, the scope of the measures and their failure policy.
- Tests: in `internal/hook/answer_hook_logic_test.go`: each measure below and at its `at_least`, with and without a
  `changed` next to it; a measure inside `any` taking the `changed` of the enclosing `if`; a `changed` inside the
  `any` overriding it; `risk_score` with weights; `any` with one condition true and with none; the fail-closed case
  with its single warning; `{risk_score}` in a `deny` measured over the `changed` of the `if`;
  and no `Diff` step on an event whose entries use no measure.
  In `internal/hook/answer_hook_in_test.go`: the fake port answers the `Diff` step and its error.
  In `internal/hook/answer_hook_out_test.go`: the adapter on a temporary repository,
  with a tracked and an untracked file.
  In `internal/hook/hook_condition_test.go`: `Expand` with known and unknown measures.
- Verification: `just test -run 'TestAnswerHook|TestExpand'`, expected: the new and existing tests pass,
  and `just test -run 'TestGOARCH'` stays green.

### Task 5: The command polix risk, use case MeasureDiff

- Covers: AC8, AC11.
- Files: `internal/diff/measure_diff_in.go`, `internal/diff/measure_diff_logic.go`,
  `internal/diff/measure_diff_out.go`, `internal/diff/measure_diff.md`, `internal/diff/measure_diff_in_test.go`,
  `internal/diff/measure_diff_logic_test.go`, `internal/diff/measure_diff_out_test.go` (create),
  `cmd/polix/main.go` (`main`, modify).
- Rules: GOARCH-1, GOARCH-3, GOARCH-4, GOARCH-5, GOARCH-6, GOARCH-10, GOARCH-11, GOARCH-12, GOARCH-13, GOARCH-14,
  GOARCH-15, GOARCH-16, GOARCH-18, GOARCH-20, GOARCH-21, GOARCH-22, GOARCH-23, GOARCH-24, GOARCH-25, GOARCH-31,
  GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-37, GOARCH-38, GOARCH-39, GO-2, GO-3, XPLAT-1,
  DOC-1.
- Reuse: the shape and the styled output of `GraphWorkflowCommand` and `LogWorkflowCommand` in `internal/workflow`,
  `workflow.Parse` for the weights, `Repo.Stats` and `Repo.CommitStats` from Task 1, and the `--dir`/`-C` flag of
  the repository commands.
- Changes: `MeasureDiffCommand` is `polix risk`, in the group `repo`, with `--dir`, a repeatable `--changed` glob
  and `--commit`, a revision; `MeasureDiffRequest` carries the globs and the revision, empty for the uncommitted diff.
  The entry rejects an empty `--commit` given explicitly before the shell runs.
  The port opens the repository, loads `polix.yaml` (absent: no weights) and returns the diff: `Diff` for the
  uncommitted changes and `CommitDiff` for a revision; the shell picks one by whether the request has a revision.
  The adapter wraps `pkg/git`, and builds and validates a `diff.Diff` from either result.
  `MeasureDiff` takes the diff, the weights and the globs, and narrows the diff to the globs.
  It returns the totals (changed lines, changed files, risk score) and the contributions with the added and deleted
  lines of each file.
  The response prints what was measured (the uncommitted changes, or the short hash and subject of the commit),
  then the totals, with the names of the conditions, and then one line per file.
  With nothing to measure it prints zeros.
  Outside a repository, with a `polix.yaml` that does not parse or is not valid, with a revision that is not a
  commit, or when git fails, it prints the cause and exits with 1.
  `measure_diff.md` documents the use case with the sections of GOARCH-14 and the names of GOARCH-36.
  `cmd/polix/main.go` adds the command to the group `repo`.
- Tests: the logic with weights and with globs; the shell with a fake port for each error and for an empty diff,
  and choosing `CommitDiff` only with a revision; the entry rejecting an empty `--commit`;
  the adapter on a temporary repository, for the uncommitted diff and for a commit; and the order of the output lines.
- Verification: `just test -run 'TestMeasureDiff'` passes, and `just build` then `build/polix risk` prints the measures.

### Task 6: Documentation of the schema, the command and the skill

- Covers: AC10.
- Files: `docs/polix.md` (sections Commands, polix.yaml and How a hook call is answered, modify),
  `examples/polix.yaml` (modify), `plugins/polix/skills/workflow/reference.yaml` (modify).
- Rules: DOC-1, GO-2, XPLAT-2.
- Reuse: the wording of `changed` and `not` in `docs/polix.md`.
- Changes: `docs/polix.md` lists `polix risk` among the commands, with `--changed` and `--commit`, and shows how
  to calibrate the thresholds by running it over the commits of `git log`.
  It documents the section `risk` and its `weights`,
  the conditions `changed_lines`, `changed_files`, `risk_score` and `any`,
  how a measure takes its files from the nearest `changed`, the formula, the fail-closed policy and the placeholders.
  `examples/polix.yaml` annotates `risk.weights` and a `pre_commit` entry with `changed` and an `any` of the three.
  The reference of the skill workflow lists the new key, conditions and placeholders, so the skill can write them.
- Tests: the test of `internal/workflow/workflow_test.go` that validates `examples/polix.yaml` keeps passing
  with the new entries.
- Verification: `just test`, expected: green; `claude plugin validate plugins/polix`, expected: passes.

### Task 7: The gate of this repository asks for a review only past the thresholds

- Covers: AC9.
- Files: `polix.yaml` (`risk.weights`, `on.pre_commit` and the comments of the header, modify).
- Rules: none.
- Reuse: the current `pre_commit` entry, its `changed` on the Go files and its `unless: { done: reviewed }`.
- Changes: add the section `risk` with its `weights`, in this order of priority, so a test of the core weighs 0.5:
  `*_test.go` and `tests/**` at 0.5, then `internal/hook/**` and `internal/state/**` at 3,
  then `pkg/git/**` at 2.
  The `pre_commit` entry keeps its `changed` on `*.go`, `go.mod` and `go.sum`, which scopes the measures,
  and adds an `any` of `changed_lines` at least 150, `changed_files` at least 5 and `risk_score` at least 25.
  Its message shows `{changed_lines}`, `{changed_files}` and `{risk_score}`.
  The comments of the header and of the entry explain the thresholds and point to `polix risk` to tune them.
- Tests: none beyond the end to end check of `Verification`.
- Verification: `just check`, expected: install, `polix check`, `polix rules check` and `polix doctor --offline` pass.

## Risks

- A failing git skips no review but asks for one on every Go commit: the warning names the cause,
  as with an unknown content hash.
- A measure written without a `changed` around it measures the whole diff, docs and plans included:
  `docs/polix.md` and the example show it next to `changed`.
- Huge untracked files are read whole on each measure: the measure runs only when a condition or a string needs it,
  and ignored files, where build outputs live, are left out.
- Thresholds too high let risky changes through without a review: `polix risk` on recent commits calibrates them,
  and the Go lint and tests of `stop` still run on every change.
- A run on `pre_commit` that rewrites files leaves the measures of the start of the call:
  documented as a limit, as for `changed`.
- New built-in placeholders break a workflow with a variable of the same name: `polix check` names it.

## Verification

- `just test`: every test passes, the architecture tests of `tests/architecture/` included.
- `golangci-lint run ./...`: no finding.
- `just check`: install, `polix check`, `polix rules check` and `polix doctor --offline` pass.
- `claude plugin validate plugins/polix`: passes.
- End to end, in this repository after `just install`:
  1. Change one line of a Go file and run `polix risk --changed '*.go'`: the totals are below the thresholds.
  2. Commit it without a review: polix lets the commit through.
  3. Change Go files past 150 lines, or touch `internal/hook/` enough to pass a risk score of 25.
  4. Commit: polix denies it with a message that shows the measures.
  5. Run polix:reviewer, wait for it to end, and commit again: polix lets the commit through.
  6. Run `polix graph`: the `pre_commit` entry shows the `any` with its three measures.
  7. Run `polix risk --commit HEAD --changed '*.go'` after the commit: it prints the measures of step 4.
  8. Run `polix risk --commit` with a revision that does not exist: it prints the cause and exits with 1.
