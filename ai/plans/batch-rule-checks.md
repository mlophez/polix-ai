# Run the checks of several rules in one invocation of their runner

## Overview

Today polix runs the check of each rule of `ai/rules/` in a process of its own, one after another.
For the 33 architecture rules of this repository that means 33 runs of `go test`, about 19 seconds after any change,
while the same tests run together take about 1.5 seconds.

After this change a runner of `rules.run` can cover many rules at once.
Three new placeholders stand for all the rules of a group: their names, their files and their folders.
A runner that uses them runs once for all its rules, and a runner without placeholders, such as `go test ./...`,
runs once for every rule it covers.
Runners that name a single rule keep working as today.

When a shared run fails, polix runs each of its rules again on its own, so it still reports exactly which rules fail.
That extra cost is only paid when something fails.

Runners also match their globs like the rest of polix: with a `/` the glob matches the path of the check, so
different kinds of tests in different folders can have different runners.

The visible effect: `polix rules check` and the stop hook take seconds instead of tens of seconds, with the same
per-rule report.

## Context

Each deterministic rule is checked by its own process, sequentially, both in `polix rules check` and in the hook
action `run: { rules }`, so a suite of Go tests pays the start of `go test` and the parse of the module once per rule.
The outcome is a runner that can check every rule it matches in one invocation, still attributing failures to rules,
and runners that can tell apart checks by their folder.

## Decisions

- Batching is declared with plural placeholders, not with a new key: `{names}`, `{files}` and `{dirs}`.
  `{names}` is the names of the group (each id with only its letters and digits) joined with `|`, a regex alternation,
  and may be part of an argument.
  `{files}` and `{dirs}` expand into one argument per distinct value, in the order of the rules, so each must be a whole
  argument of the argv.
- The checks of one runner are grouped by their argv with the singular placeholders (`{file}`, `{dir}`, `{id}`,
  `{name}`) expanded: rules with the same such argv share one invocation, where the plural placeholders cover them all.
  A runner with `{dir}` and `{names}` runs once per folder; one without placeholders runs once; one with `{name}`
  runs once per rule, as today.
- A check that no runner matches still runs alone as a program.
- When a shared invocation fails, each of its rules runs again alone, with the plural placeholders reduced to that
  rule; the results of those runs decide the rules.
  When the argv of a rule alone is the same as the shared one (no placeholder tells the rules apart), the rerun would
  repeat it, so every rule of the group fails with the shared output instead.
- A shared invocation that passes makes every rule of it pass, with the shared output.
- The glob of a runner matches as the globs of `scope` and `protected` do (`pkg/glob`): without `/` against the base
  name of the check, with `/` against its path relative to the root, `**` crossing folders.
  Existing runners keep matching the same checks; character classes (`[abc]`), which only `path.Match` accepted, are
  no longer special.
- Runs stay sequential and in the order of their first rule (by id); there is no parallelism.
- The response and the messages stay per rule: `polix rules check` still counts checks as rules decided, and the stop
  hook blocks naming only the rules that fail.
- `CheckRule` takes the step shape (GOARCH-20): what runs next depends on the outcome of the shared invocation.
- The polix.yaml of this repository is not changed by this plan: it is protected, and the user decided to leave it.

## Approach

**Workflow domain** (`internal/workflow/workflow_runner.go`).
`RunnerPlaceholders` grows to the singular `file`, `dir`, `id`, `name` and the plural `files`, `dirs`, `names`.
Validation of a runner adds one problem: `{files}` or `{dirs}` that is not a whole argument.
The check of the glob syntax with `path.Match` goes away, since `pkg/glob` accepts any non-empty pattern.
Matching a check to a runner uses `glob.Path` on the cleaned path of the check, relative to the root with slashes.
`Workflow.CheckCommand` keeps its contract for one rule (argv of the first matching runner, nil when none), with the
plural placeholders reduced to that rule; `internal/setup` keeps using it to know whether a runner runs a check.
A new method, `Workflow.CheckCommands`, takes the checks of several rules (file and id, in order) and returns the
invocations that cover them: a new domain struct `workflow.Invocation` with the argv and the positions of the checks
it covers, and its `Valid`.
Checks without a matching runner get an invocation each with an empty argv.

**Rule domain** (`internal/rule/rule_run.go`, `internal/rule/rule_result.go`).
`rule.Run` changes from one rule to the rules it decides (`Rules`, in id order) plus `Argv`.
Its `Valid` requires at least one rule, every rule valid and with a check, and an argv when there is more than one
rule, since only a single check can run as a program.
A new domain function, `rule.Plan`, turns the rules with a check and the workflow into the runs to carry out, through
`Workflow.CheckCommands`.
A new domain struct, `rule.Outcome`, holds what carrying out a run gave: its output and its error, with its `Valid`.
A new method, `Run.Settle`, takes the outcome of the run and returns either the `Result` of each of its rules or the
runs of one rule each that must run to decide them (the reruns above).
`rule.Result` stays per rule; `Tail` and `Failed` keep their contracts.

**CheckRule** (`internal/rule/check_rule_*.go`).
The logic takes, besides what it takes today, the outcomes of the runs carried out so far, in order.
It replays them over the plan and its reruns, and its output is either the next run to carry out or, when none is
left, the results of every rule.
The shell loops: it calls the logic from one call site, carries out the next run through `RunCheck`, appends the
`Outcome` after `Valid`, and stops when the output has no next run; the response is built from the results.
The port is unchanged in name; `RunCheck` of the adapter takes the check of the first rule of the run for the case of a
program, which only happens with a single rule.

**Hook** (`internal/hook/answer_hook_logic.go`, `answerHookRun.runRules`).
It plans with `rule.Plan` the rules matching the target, acts each run as a `Step` with `Check`, settles it with
`Run.Settle`, and acts the reruns it returns, all through the journal, as the other steps do.
`AnswerHookAdapter.RunCheck` takes the check of the first rule for a program.

**Errors.**
A runner with `{files}` or `{dirs}` inside a longer argument, or with an unknown placeholder, makes `Rules.Valid` fail;
`polix rules check` stops before running anything, and `polix check` reports it, as with today's runner problems.
A run that exits with an error, times out or cannot start is a failed outcome; the 5 minutes of a check apply to each
invocation, shared or not.
An `Outcome` or a `Result` that is not valid stops the shell of `CheckRule` with its error, as a `Result` does today.

### Discarded alternatives

- A key `batch: true` on each runner: more verbose and changes the shape of `rules.run`; the placeholders already say
  which arguments cover the group.
- Only deduplicating identical argv: cannot select the rules of a group, so a runner could not run just the tests of
  the changed rules.
- Parsing the output of the runner (`go test -json`) to attribute failures: ties the engine to one tool.
- Marking every rule of a failed group as failed: one failing rule would show as 33.
- Running the checks in parallel: measured at about 9 seconds for this repository, far from 1.5, and it overloads the
  machine.

## Acceptance criteria

- AC1: With the runner `"*_test.go": [go, test, "{dir}", -run, "^Test({names})$"]`, `polix rules check` on a copy of
  this repository runs `go test` once for the 33 rules with a check in `tests/architecture/`, reports `33 checks, 0
  failed`, and takes less than 4 seconds after `go clean -testcache`.
- AC2: When a shared invocation passes, every rule of it passes and nothing else runs.
- AC3: When a shared invocation fails, each of its rules runs again alone, and only the rules whose own run fails are
  reported, each with the tail of its own output.
- AC4: A runner without placeholders, such as `[go, test, ./...]`, runs once for all the rules it matches; when it
  fails, all of them are reported as failed with the shared output and nothing is rerun.
- AC5: `{names}` expands to the names of the group joined with `|`; `{files}` and `{dirs}` expand to one argument per
  distinct value in the order of the rules; rules whose argv with the singular placeholders expanded differ run in
  different invocations.
- AC6: A runner glob without `/` matches the base name of the check and one with `/` matches its path, so
  `tests/architecture/*_test.go` and `tests/e2e/*_test.go` can have different runners.
- AC7: `polix check` and `polix rules check` reject a runner with `{files}` or `{dirs}` inside a longer argument and
  an unknown placeholder, naming the seven valid ones.
- AC8: The hook action `run: { rules: changed }` batches and attributes as `polix rules check` does, and its block
  message names only the rules that fail.
- AC9: Runners with only singular placeholders and checks without a runner behave as today: one invocation per rule.
- AC10: `docs/polix.md`, `examples/polix.yaml`, `docs/use-cases.md`, `docs/architecture.md`, the documents of the use
  cases and the plugin texts describe the new placeholders, the grouping, the reruns and the glob matching.

## Out of scope

The polix.yaml of this repository: it is protected and the user chose not to change it in this plan.
To get the gain here, its runner becomes `"*_test.go": [go, test, "{dir}", -run, "^Test({names})$"]`, a change the
user makes by hand; whether to keep both `go test ./...` and `rules: changed` on stop is also their call.
Parallel runs, plural placeholders for ids (`{ids}`) and parsing the output of runners are left out, as decided.

## Tasks

### Task 1: Plural placeholders, grouping and path globs in the runners of the workflow

- Covers: AC5, AC6, AC7, AC9.
- Files: `internal/workflow/workflow_runner.go` (`Runner`, `RunnerPlaceholders`, `Workflow.CheckCommand`,
  `validator.runner`, modify; `Workflow.CheckCommands` and `Invocation`, create in the same file),
  `internal/workflow/workflow_runner_test.go` (modify).
- Rules: GOARCH-4, GOARCH-9, GOARCH-13, GOARCH-18, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-29, GOARCH-30, GOARCH-35,
  GOARCH-38, GOARCH-40, GO-2, XPLAT-1.
- Reuse: `glob.Path` from `pkg/glob/glob.go`; the `placeholder` regexp and `contains` already used by
  `validator.runner`.
- Changes: The doc comment of `Runner` lists the seven placeholders and says which are plural.
  `RunnerPlaceholders` adds `files`, `dirs` and `names`.
  `validator.runner` drops the `path.Match` syntax check and adds the problem of `{files}` or `{dirs}` not being a whole
  argument, worded like the other runner problems with the glob.
  The match of a check to a runner goes through `glob.Path` with the check cleaned as today.
  `Workflow.CheckCommand` expands the plurals with the single rule it is given, so its result for one rule equals what
  `Workflow.CheckCommands` gives for a group of that rule alone.
  `Workflow.CheckCommands` finds the runner of each check, expands the singular placeholders, groups the checks of the
  same runner whose expanded argv are equal, and expands the plurals with the names, files and folders of the group,
  files and folders without repeats; invocations come in the order of their first check, and a check without runner
  is an invocation of its own with an empty argv.
  `Invocation` holds the argv and the positions of its checks; its `Valid` requires at least one position, positions
  without repeats, and an argv when there are several positions.
- Tests: in `workflow_runner_test.go`: grouping by `{dir}` with `{names}`; one invocation for an argv without
  placeholders; one invocation per rule with `{name}`; `{files}` and `{dirs}` deduplicated and in rule order;
  `{names}` joined with `|` inside `^Test(...)$`; two runners in one call keeping file order of the globs; checks
  without runner as single empty invocations; a glob with `/` matching only checks under its folder and one without
  `/` matching any folder; `CheckCommand` equal to the single-check `CheckCommands`; validation rejecting
  `--x={files}` and `{dirs}/y`, and an unknown placeholder listing the seven; `Invocation.Valid` cases.
- Verification: `just test -run 'Runner|CheckCommand|Invocation'`, expected: pass.

### Task 2: Runs of several rules, their outcome and how they settle

- Covers: AC2, AC3, AC4.
- Files: `internal/rule/rule_run.go` (`Run`, `Run.Valid`, modify; `Plan` and `Run.Settle`, create in the same file),
  `internal/rule/rule_result.go` (`Outcome`, create in the same file), `internal/rule/rule_run_test.go` and
  `internal/rule/rule_result_test.go` (modify).
- Rules: GOARCH-2, GOARCH-4, GOARCH-9, GOARCH-13, GOARCH-18, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-29,
  GOARCH-30, GOARCH-35, GOARCH-38, GOARCH-40, GO-2.
- Reuse: `Workflow.CheckCommands` and `Workflow.CheckCommand` from Task 1; `Rule.HasCheck`, `Rule.Valid`, `Result`.
- Changes: `Run` replaces its field `Rule` with `Rules`, the rules the run decides in id order, and keeps `Argv`; its
  doc comment says that more than one rule means a shared invocation.
  `Run.Valid` requires at least one rule, each valid and with a check, and a non-empty argv with more than one rule.
  `Plan` takes the rules and the workflow, keeps those with a check, and maps each invocation of
  `Workflow.CheckCommands` to a `Run` with its rules, each run followed by `Valid`.
  `Outcome` holds the output and the error of carrying out a run; its `Valid` returns nil, as it has no invariant
  beyond its types, and its doc comment says so.
  `Run.Settle` takes the outcome and decides: passed gives a passed `Result` per rule with the shared output; failed
  with one rule gives its failed `Result`; failed with several rules gives, for each rule, a run of that rule alone
  with the argv of `Workflow.CheckCommand`, unless that argv equals the shared one for every rule, in which case it
  gives a failed `Result` per rule with the shared output and error.
  `Run.Settle` needs the workflow to build the single runs, so it takes it as a parameter; it never touches the outside.
- Tests: in `rule_run_test.go`: `Plan` grouping the 33-like set into one run, keeping rule order, skipping rules
  without check, and giving single runs for checks without runner; `Run.Valid` with no rules, an invalid rule, a rule
  without check, and several rules with an empty argv; `Run.Settle` for a passed shared run, a failed single run, a
  failed shared run that splits, and a failed shared run without placeholders that does not.
  In `rule_result_test.go`: `Outcome.Valid`.
- Verification: `just test -run 'Run|Plan|Settle|Outcome|Result'`, expected: pass.

### Task 3: CheckRule runs the plan in the step shape and reruns failed groups

- Covers: AC1, AC2, AC3, AC4, AC9.
- Files: `internal/rule/check_rule_logic.go` (`CheckRule`, `CheckRuleOutput`, modify),
  `internal/rule/check_rule_in.go` (`checkRule`, `checkRuleResponse`, modify), `internal/rule/check_rule_out.go`
  (`CheckRuleAdapter.RunCheck`, modify), `internal/rule/check_rule.md` (modify),
  `internal/rule/check_rule_logic_test.go`, `internal/rule/check_rule_in_test.go` and
  `internal/rule/check_rule_out_test.go` (modify).
- Rules: GOARCH-3, GOARCH-4, GOARCH-10, GOARCH-11, GOARCH-13, GOARCH-14, GOARCH-15, GOARCH-20, GOARCH-21, GOARCH-22,
  GOARCH-23, GOARCH-24, GOARCH-31, GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-36, GOARCH-37, GOARCH-38, GO-2, XPLAT-1,
  DOC-1.
- Reuse: `rule.Plan`, `Run.Settle`, `rule.Outcome`, `rule.Matching`, `rule.Failed`, `Result.Tail`.
- Changes: `CheckRule` gains a last parameter, the outcomes carried out so far in order; it filters the rules as today,
  plans them with `Plan`, and walks the runs and the reruns their `Settle` returns, taking one outcome each, in order.
  `CheckRuleOutput` replaces `Run []Run` with the next run to carry out (none when the walk consumed every outcome
  and nothing is left) and the results of every rule, in rule order, set only when there is no next run.
  `checkRule` loops on the logic from one call site, carries out the next run through `RunCheck`, builds an `Outcome`
  followed by `Valid`, and builds the response from the results; every condition only reads the output and errors.
  `checkRuleResponse` keeps counting `Ran` as the rules decided.
  `CheckRuleAdapter.RunCheck` resolves the check file from the first rule of the run.
  `check_rule.md` updates Logic (grouping, settle, reruns, step shape), Port (`RunCheck` runs one invocation that may
  decide several rules), Flow (the loop) and keeps every named field and method current.
- Tests: in `check_rule_logic_test.go`: with no outcomes the output asks for the shared run; with a passing outcome it
  returns all results passed; with a failing outcome it asks for the first single rerun, and with all reruns it
  returns only the failing rules as failed; an argv without placeholders failing gives every rule failed with no
  rerun; `--changed` still narrows the rules.
  In `check_rule_in_test.go`, with the fake port counting calls: one `RunCheck` when the group passes; one plus one
  per rule when it fails; the response lists only the failing rules; an invalid runner stops before any run.
  In `check_rule_out_test.go`: `RunCheck` of a run of several rules runs its argv once in the temporary repository.
- Verification: `just test -run CheckRule`, expected: pass.

### Task 4: The hook action run rules batches and attributes the same way

- Covers: AC8.
- Files: `internal/hook/answer_hook_logic.go` (`answerHookRun.runRules`, modify), `internal/hook/answer_hook_out.go`
  (`AnswerHookAdapter.RunCheck`, modify), `internal/hook/answer_hook.md` (modify),
  `internal/hook/answer_hook_logic_test.go` and `internal/hook/answer_hook_in_test.go` (modify).
- Rules: GOARCH-4, GOARCH-10, GOARCH-13, GOARCH-14, GOARCH-20, GOARCH-21, GOARCH-24, GOARCH-34, GOARCH-36, GOARCH-37,
  GOARCH-38, GO-2, XPLAT-1, DOC-1.
- Reuse: `rule.Plan`, `Run.Settle`, `rule.Outcome`, `rule.Matching`, `rule.Failed`; `answerHookRun.act` and
  `answerHookRun.waiting` for the journal.
- Changes: `runRules` matches the rules to the paths as today, plans them with `rule.Plan`, acts each run as a `Step`
  with `Check`, turns its `Outcome` from the journal into the settle of the run, and acts the reruns it returns before
  the next run; it keeps waiting when a step is missing, and the block message lists the failing rules as today.
  `AnswerHookAdapter.RunCheck` resolves the check file from the first rule of the run.
  `answer_hook.md` says that a check step may decide several rules and that failed groups are rerun per rule, and
  names `rule.Plan` and `Run.Settle` where it names `workflow.Workflow.CheckCommand` today.
- Tests: in `answer_hook_logic_test.go`: on stop with `run: { rules: changed }` and a batch runner, the first step is
  one `Check` with every matching rule; a passing journal lets the stop through; a failing one asks for the single
  reruns, and the block names only the rules whose rerun fails; a runner with `{name}` asks one step per rule as today.
  In `answer_hook_in_test.go`: the fake port sees one `RunCheck` for a passing group.
- Verification: `just test -run AnswerHook`, expected: pass.

### Task 5: Documentation of the runners and of the flows

- Covers: AC10.
- Files: `docs/polix.md` (sections of `rules.run` and `run: { rules }`, modify), `examples/polix.yaml` (comment and
  runners of `rules`, modify), `docs/use-cases.md` (section "The same checks for the agent and for CI", modify),
  `docs/architecture.md` (section "Flow of a use case", modify), `plugins/polix/skills/add-rule/SKILL.md`,
  `plugins/polix/agents/rules-author.md` and `plugins/polix/skills/doctor/SKILL.md` (modify).
- Rules: DOC-1, GO-2.
- Reuse: none.
- Changes: `docs/polix.md` documents the seven placeholders, the grouping by the singular expansion, the reruns of a
  failed group, the case without placeholders, the glob matching of `pkg/glob`, and replaces "each rule runs on its
  own" with the new rule; its example of `GOARCH-4` shows the batch runner and the single command a rerun uses.
  `examples/polix.yaml` shows the batch runner for `*_test.go`, a runner with a `/` glob, and keeps the `*.py` one.
  `docs/use-cases.md` uses the batch runner in its example.
  `docs/architecture.md` describes the flow of `polix rules check` as the loop of the step shape.
  The plugin texts say that the glob of a runner matches the base name, or the path when it has a `/`, and the doctor
  skill drops "an invalid glob" in favour of the placement of `{files}` and `{dirs}`.
- Tests: none; the docs are checked by reading them against the code of Tasks 1 to 4.
- Verification: `rg -n "matches its base name|each rule runs on its own" docs plugins examples`, expected: no output.

## Risks

- A batch of many slow checks may hit the 5 minutes of one invocation that used to apply per rule: the docs say the
  timeout applies per invocation, and a runner with `{name}` keeps the old behaviour.
- `{files}` with very many rules could reach the argv limit of Windows: rule sets are small, and the docs suggest
  `{dirs}` or `{names}` for large suites.
- Globs with character classes change meaning: no runner in the repository or the examples uses them, and the docs
  name the syntax of `pkg/glob`.
- `docs/architecture.md` has uncommitted changes of the user: edit only the flow section and keep the rest as it is.
- Rules whose checks share state across runs could behave differently together: batching is opt-in through the
  plural placeholders.

## Verification

- `just test`: pass.
- `golangci-lint run ./...`: no findings.
- `just check`: install, `polix check`, `polix rules check` and `polix doctor --offline` pass with the current
  polix.yaml (runner with `{name}`, unchanged behaviour).
- `claude plugin validate plugins/polix`: valid.
- End to end, for AC1 and AC3, on a copy of the repository so the protected polix.yaml stays untouched:
  1. Copy the working tree to a temporary folder and initialise it as a git repository with one commit.
  2. In the copy, set the runner of `*_test.go` to `[go, test, "{dir}", -run, "^Test({names})$"]`.
  3. Run `go clean -testcache` and time `polix rules check -C <copy>`: one `go test`, `33 checks, 0 failed`, under 4
     seconds.
  4. Break one rule in the copy (for example, add a file of a domain package with an unknown name for GOARCH-1),
     run `polix rules check -C <copy>` again: only GOARCH-1 is reported, with its own output.
