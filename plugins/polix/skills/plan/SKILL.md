---
name: plan
description: >
  Writes an outcome-driven plan, on a new branch, in the folder of plans from the spec the skill grill-me confirms.
  It writes the tasks that build it, has the subagent critic review it, and never runs in plan mode, codes nor commits.
  It runs only when the user asks for it.
effort: high
model: inherit
user-invocable: true
disable-model-invocation: true
argument-hint: "[what to plan, e.g. add retries to the webhook sender, and where to save it; empty: the conversation]"
---

# Planning

Turn a request into a plan file that an executor builds task by task in order, knowing only the plan and the code.
The plan goes from the outcome expected to the criteria that prove it and the tasks that build it, and holds no code.
The plan is a YAML file: each fact lives in one place, and the others name it by its id.
The skill `grill-me` interviews the user; this skill turns its spec into the plan and never interviews.
The folders of plans, of rules and of tasks are those the project names in `AGENTS.md`, `CLAUDE.md` or the session.
Without one, they are `ai/plans/`, `ai/rules/` and `ai/tasks/` below the root.

## Requirements

- [template.yaml](template.yaml) next to this file.
  Without it, stop and tell the user.
- The skill `grill-me`, which interviews the user each time the spec opens or reopens.
  From a plugin, it and the subagent below carry its prefix, as `<plugin>:grill-me` and `<plugin>:critic`.
  Without the Skill tool, as in Codex, read `../grill-me/SKILL.md` next to this skill and follow it in this session.
- The subagent `critic`, which reviews the plan at Step 6.
  Without it, as in Codex, read `../../agents/critic.md` next to this skill and follow it yourself at Step 6.
  Follow it in a pass apart from the writing, reading only what the critic may read.
  The critique is then not independent: say so in `Pending`.

## Input

- **Request**: what to plan, from the arguments or the conversation.
  Without either, ask the user.
  It may be a file of the folder of tasks, as the skill `task` writes it.
  Then plan from its sections, and give its path as the `reference` of the plan.
  Once the plan file exists, set the task's `status: planned` and `plan: <path of the plan>`, and nothing else of it.
- **Spec**: a spec that `grill-me` returned and the user confirmed in this session for this request, if any.
- **Plan**: the plan file, from the arguments or the conversation.
  A file that already exists is the plan to adjust.
  So is the plan the task's `plan` names, or one written earlier in this session, even with a new scope or title.
  A plan in Markdown (`.md`) is no longer supported: stop, and offer to write a new YAML plan from its content.
  A path that does not exist is where the new plan goes, used as is, outside the folder of plans too.
  A path ending in `/` is a folder: name the file inside it as `Plan file` says.
  Without a path, write the new plan where `Plan file` says.
  When two of these name different files, ask the user which one is the plan.

## Output

- **Plan**: the path of the plan file, its `status` and its `branch`; Step 7 already showed its outcome.
- **Decisions**: every decision of the plan with `source: delegated`, or "none".
- **Next steps**: adjust the plan, or build, verify and review it with the skill `build`.
- **Pending**: open blockers and stale or missing docs, with the skill `add-rule` when rules are missing.
  Also the decisions the user left deferred and the findings of `critic` its last round left open.
  Also the uncommitted changes other than the plan and its task: the skill `build` starts only once they are
  committed; or "none".

## Limits

- Never plan in plan mode, nor call `EnterPlanMode`, since the skill writes the plan file and creates its branch.
  Started in plan mode, stop and tell the user to leave it and run the skill again.
- Approval never starts the build: Step 8 still applies.
- Never ask the user a decision of the plan yourself: `grill-me` asks them (see Steps 2 and 6).
  Ask the user only what `Input` leaves open, the existing branch of Step 4 and the approval of Step 7.
- Write only the plan file and its task's frontmatter, create only the branch of Step 4, run only the project's tests.
- Define tasks, never code: no snippets, implementations or signatures (see `Plan file`).
- One plan is one change, delivered whole: never phases or follow-up plans.
- Reference the rules of the folder of rules by id in the `rules` of each task, never restate them.
- Name no executor, agent, skill or tool of the agent in the plan file but the subagents of `agents`.
  The commands of the project are fine.

## How to operate

Copy this checklist and track your progress:

```text
Plan progress:
- [ ] Step 1: Resolve the request, the spec and the plan to adjust, if any (see `Input`).
- [ ] Step 2: Get the confirmed spec from `grill-me` (see Step 2).
- [ ] Step 3: Read the project context and the code the tasks will change (see Step 3).
- [ ] Step 4: Create the branch of a new plan (see Step 4).
- [ ] Step 5: Write the plan file from the spec and the code (see Step 5).
- [ ] Step 6: Have `critic` review the plan, and fix what it finds, up to three rounds (see Step 6).
- [ ] Step 7: Show the user the outcome of the plan and ask for approval (see Step 7).
- [ ] Step 8: Report (see `Output`), stop and return control to the user.
```

Never reach Step 7 before Step 6 is done: the critique is what the approval relies on.
Every time `grill-me` reopens the spec, update the keys it changes, then run Step 6 again whole.

### Step 2: Get the confirmed spec from `grill-me`

1. With a `Spec` of `Input`, take it as it is and skip the rest of this step.
2. Otherwise invoke the skill `grill-me` with the request as its subject.
   Give as its `Known` the facts and decisions the conversation or the task file already settle.
   When adjusting a plan, give the plan's current keys as its `Adjust`, with what the adjustment changes.
3. Wait for the spec the user confirms, then go on with Step 3 of this checklist.
   Its `Pending` decisions go to `Pending`, and Step 7 never sets `status: approved` while one is open.

### Step 3: Read the project context and the code the tasks will change

1. Read the project context (see `Project context`).
2. Run the test command of the project once in the background, editing nothing.
   Its failures, when any, go to `context` as failures that already exist; when it cannot run, say so in `Pending`.
   When adjusting a plan, skip it unless the adjustment adds tasks.
3. Take the `Facts` of the spec as leads, never as facts of the plan.
4. Read each file a task will change and each symbol the plan will name: never name one from a search hit alone.
5. The code wins over the docs and the spec when they disagree.
   A gap that changes a decision of the spec reopens it with `grill-me`; any other gap goes to `Pending`.

### Step 4: Create the branch of a new plan

1. When adjusting a plan, skip this step: the plan keeps its `branch`.
2. Choose the plan file's path (see `Plan file`) before writing it.
3. Name the branch as the plan file without its extension, unless `AGENTS.md` or `CLAUDE.md` names a convention.
4. When the branch exists locally (`git branch --list <name>`) or on a remote (`git branch -r --list '*/<name>'`),
   stop and ask the user whether to switch to it or take another name, which the plan file takes too.
5. Create it from the current branch with `git switch -c <name>`, so the uncommitted changes go with it.
   Switch to an existing branch the user chose with `git switch <name>`.

### Step 5: Write the plan file from the spec and the code

1. Create the plan file from [template.yaml](template.yaml) (see `Plan file`), or open the plan to adjust.
2. Write each key in the order of template.yaml, from the section of the spec `Spec to keys` names for it.
   Follow `Request`, `Outcome`, `Acceptance criteria`, `Tasks`, `High risk` and `Tests` for its form.
3. A key that needs a decision the spec does not settle is a reopened spec: invoke `grill-me` with `Adjust`.

### Step 6: Have `critic` review the plan, and fix what it finds, up to three rounds

The critic reads the plan in a context of its own, with the spec, the template, the rules and three sections of this
skill (`High risk`, `Tests` and `Readability`), never the code.
It judges whether an executor could build it without deciding, and whether it follows the spec, the rules and the form.

1. Launch the subagent `critic` with the prompt of `Critic prompt`, and wait for its report.
2. Sort each finding of the report:
   - Mechanical: the plan, the request, the code, a rule or an answer of the user already settles its fix.
     Fix it with a targeted edit of the plan file at its path, `minor` ones too.
     Never write the plan again whole nor elsewhere.
   - Decision: its fix is a choice no source settles, as a fix `decision` says.
     Invoke `grill-me` once for all of them, with the findings and the plan's current keys as its `Adjust`.
     Write each answer of its spec into its key.
3. Launch the critic again, with the next round and the findings so far in `Previous`.
   It checks the whole plan again, so a fix that breaks another key is a finding of the next round.
4. Stop when the verdict has no `blocking` nor `major` finding, or after the third round.
   What the last report leaves open goes to `Pending`, each finding with its severity and its first words.

Without the subagent `critic`, follow `critic.md` yourself as `Requirements` says, at each round.

### Step 7: Show the user the outcome of the plan and ask for approval

- Write as text, in the language of the session, the final result the plan builds, with the path of the plan.
  It is the `summary` of `overview`, the `interface` and the journeys, `scope` and the titles of the criteria.
  Then the model and effort of each agent of `agents`, which the user may change.
- Then the critique in a few lines: the rounds, the findings fixed, those asked to the user and those left open.
- Then ask with `AskUserQuestion` whether to approve the plan or adjust it, approve first.
  The question says the result is just above it, since the dialog may hide it.
  Without `AskUserQuestion`, ask for the approval in the text after the result.
- With a deferred decision open, reopen it with `grill-me` first; while it stays open, never offer to approve.
- Approved: set `status: approved`, and nothing else.
- Adjust: invoke `grill-me` with the comments as its `Adjust`, keeping `status: planned`.
  Then update the keys, run Step 6 and show the result again.
- Without an answer, leave `status: planned` and say in `Pending` that the plan waits for approval.

## References

### Project context

- Read `AGENTS.md` and `CLAUDE.md` when they are not in context: purpose, stack, commands, layout and testing.
  Run a command as they write it, narrowing it only with the options they document.
- Read every `yaml` file of the folder of rules except `retired.yaml`, whose ids never apply.
- A rule applies when the trigger of its `description` holds and the files touched match its `scope`.
  Without `scope`, it applies to the whole project.
- A task never breaks a rule that applies: when the design needs it, stop and tell the user.
- Two rules that contradict each other on a file a task touches: stop and report both ids, never choose one.
- The commands of `AGENTS.md` feed `verification`, with the one that runs the checks of the rules when a task's rule
  has a `check`.
  A `check` is the file that decides the rule, never a command to run.
- Without facts in `AGENTS.md` or without the folder of rules, follow the conventions of the surrounding code.
  Say what is missing in `Pending`, never in the plan.

### Request

- Every requirement of the request, and of each free-text answer that changes it, goes to `outcome`.
  Never drop nor weaken one: the review checks the work against `outcome`.
- A decision of the spec goes to `decisions`, with `on: outcome` when it is `what` and `on: approach` when `how`.
- The literals quote verbatim the strings, messages, commands and data of the request.
- Never add a requirement the user did not ask for.
- Never make a choice yourself: what the request, the spec, the docs, the code or a rule do not settle reopens the
  spec with `grill-me`, even when it looks minor, obvious or not decisive.
- A limit on how the solution is built is a constraint, and a measurable property of the result is a criterion.
  A choice among options is a decision.

### Spec to keys

Each key of template.yaml comes from a section of the spec of `grill-me`, or from the code when never asked.

- `overview`: `summary`, `changes`, `not_changes` and `risk_and_size` from `Outcome`, `Scope` and `Risks`.
  `problem` from `Problem`, `affected` from `Affected`, `terms` from `Terms` and `context` from the code.
- `specification`: `outcome` from `Outcome`, `interface` from `Contract` and `journeys` from `Journeys`.
  The exact data of a journey becomes the `literal` of its step.
  `scope` from `Scope`, `constraints` from `Constraints` and `acceptance` from `Criteria`, each in EARS by this skill.
  Number the constraints `CON-01`, `CON-02`...; never renumber them, since the decisions, risks and tasks name them.
- `decisions` from `Decisions`: `what` is `on: outcome` and `how` is `on: approach`.
  An answer of the user is `source: interview`, and the approaches discarded go to the `discarded` of the chosen one.
  Number them `DEC-01`, `DEC-02`...; never renumber them, since the design, risks and tasks name them.
- `approach`: `design` from `Approach`, naming what other modules use, never a private helper.
  `risks` from `Risks`; what the design always does goes to `cross_cutting`, never to `risks`.
  `runtime`, `cross_cutting` and `verification` from the code and the earlier keys.
- `tasks`: from the code and the earlier keys; a question they raise belongs to an earlier key and reopens the spec.

### Outcome

- Write `outcome` before the criteria and the tasks: the criteria prove it and the tasks build it.
- It describes the result as its users and callers see it, never how it is built.
  What a caller notices goes to `interface`.
  What only a reader of the code notices goes to `design`, which names `interface` without restating it.
  A caller may be code: the actor of a journey may be the module that calls an internal contract.
- `interface` names every command, option, message, file, format, API and event the change adds or changes.
  It gives their names, fields, defaults and errors.
- `journeys` are critical user journeys, `CUJ-01`, `CUJ-02`..., one per behaviour, boundary and family of errors.
  Never renumber them: the criteria name them.
- Each journey has `given` the starting state, `when` the input or action and `then` the exact result.
  Each is a list of steps, and each step has its `step`: the text, with literal values.
  The result is the output, the exit code and the state left.
  `but` names what must not happen or must stay unchanged, such as the state of a rejected input.
- When a step shows exact data the code must reproduce, the step carries it as its `literal`.
  That is a string, message, request, command, body or output that is part of the contract.
  - Its `id` is `LIT-01`, `LIT-02`..., unique in the plan; never renumber them, since other places name them.
  - The literal lives in the step of the first journey that shows it.
    Another journey, a criterion, `interface` or a task names its id, never copies its `text`.
  - `match` says whether the data is `exactly` the result or what it `contains`.
  - Put only what the criteria check.
    Leave out, or name as `<variable>`, the values that change on every run, such as generated ids and timestamps.
  - A literal holds data, never code of the implementation; its general shape, fields and types go to `interface`.
- A negation goes in the text of a step: Gherkin has no keyword for it.
- Every exact string the code must reproduce is the literal of a step: `interface` names it, never holds its text.
- A family of errors of one input, such as each way a value can be wrong, is one journey, never one per error.
  Its `then` has one step per case, each with the input it rejects and its message as the literal.
  So every message of the family lives once, and the plan does not grow by a journey per message.
- Other combinations of the same journey, without a message of their own, go to the tests of its task.
- [journeys.md](journeys.md) shows each kind of journey and a criterion that checks them; never copy its systems.
- A change with no visible effect, such as a refactor, has journeys `kept: true` of the behaviour it puts at risk.
  Its criteria compare it with the base version (see `Tests`), and its `interface` is the structure after it.
- A change to an internal contract has journeys whose actor is its caller, with the contract in `interface`.
- `scope.in` lists the areas of the code and of the product the plan touches.
  `scope.out` lists what the plan leaves out and why.

### Acceptance criteria

- Each criterion checks one part of the outcome, and every journey is in the `cujs` of a criterion.
- Each has its `title`, `requirement`, `cujs`, `verify`, and `status: pending` and `evidence: none` for the build.
- `requirement` is one EARS sentence (Easy Approach to Requirements Syntax), with the system as the subject:
  - Ubiquitous: "The <system> shall <response>".
  - Event: "When <trigger>, the <system> shall <response>".
  - State: "While <state>, the <system> shall <response>".
  - Unwanted behaviour: "If <condition>, then the <system> shall <response>", one for each error the outcome names.
  - Optional: "Where <feature is included>, the <system> shall <response>".
  - Combined, in this order: "While..., when..., the <system> shall <response>".
- `requirement` states the general rule, and never restates the values of its journeys: `cujs` names them.
- A non-functional requirement, such as performance, a limit, security or availability, is a criterion of its own.
  Its sentence names a measurable threshold and the conditions it holds under.
  For example: "The export shall finish in under 5 s for 10,000 invoices".
  Its `cujs` is `[]`, and its `verify` measures it.
- A guarantee the change gives is a criterion of its own, never only a sentence of `cross_cutting` or a comment.
  That covers security or platforms, such as "never reads outside the root" or "a link is never followed".
  It has one journey per way it could break, from the cases of the spec: each input, platform and path that reaches it.
  For example, a walk of folders has a journey for `..`, a link in any segment, a Windows junction and name case.
- `requirement` reads alone: it never points to another key, such as "as `outcome` says".
- `verify` lists commands with their expected result, or `manual` steps that show the criterion on the finished change.
  It names the tests or cases that prove this criterion, so two criteria never share a vague `verify`.
  Each `run` runs only those tests or cases, such as `go test ./internal/export -run '^TestExport'`.
  Never the whole suite, a linter nor a build: they go to the `verification` of `approach`.
  The verifier runs it as written, and the build sets `status` and `evidence` from its report.
- Number them `AC-01`, `AC-02`...; never renumber them: the `covers` of the tasks name them.

### Tasks

The build runs each task alone, in order, without asking: a builder builds it, a verifier and a reviewer check it.
Then the build commits it, and the next task starts from that commit.

- Each task leaves the project building and no test failing that passed at the baseline.
  Never a task that breaks what a later one repairs.
- Slice by behaviour, never by layer: each task delivers one behaviour that a test shows, across the layers it needs.
  A refactor and a change no test can observe are the only tasks without one.
- Start with the thinnest path that runs end to end, then widen it task by task.
- When the change is hard to make in the current code, put first a refactor that keeps the behaviour, as its own task.
- Change a contract and every caller of it in the same task.
- A contract consumed outside the repository changes by expand-contract.
  The task widens it without breaking the current form.
  For example, it adds a new optional field, or a new version beside the old one.
  A journey `kept: true` shows the current consumers still work.
  Removing the old form goes to `scope.out`, with the condition to do it.
- Size a task to one behaviour that its own tests show, verified and reviewed apart from the other tasks.
  Split a task that mixes behaviours, and merge tasks not verifiable apart.
- A criterion covered by several tasks is checked at the last task that covers it: list it in the `covers` of each.
- The checks of the whole project go to the `verification` of `approach`: the full test suite, the linter, the build.
  What proves a criterion goes to its `verify`, never to both.
  A task has no checks of its own but its `differential`.
  The build runs its `tests`, the `verify` of the criteria whose last covering task it is, and the plan's checks.
- List the tasks in the order they run, with the ids `T1`, `T2`...
- Each task has a `summary` for the reviewer, then its data for the executor (see template.yaml).
  - `summary` says in two or three sentences what the task delivers and how, naming the part of `design` it builds.
    It never restates its tests nor its criteria: `tests` and `covers` hold them.
  - `files` lists the paths the behaviour of the task lives in, with no description: the `design` says what changes.
    Never list the callers, comments, fixtures or docs that only follow the change: the build finds them.
  - `status: pending` and `evidence: none`: the build sets them from the builder's report.

### High risk

A task is high risk when it touches one of these:

1. Data, or effects that cannot be undone.
2. Security: authentication, authorization, secrets, input from outside.
3. A contract that others consume: an API, an event, a schema, a file format, a command line.
4. Concurrency.
5. Delivery or environment: build, release, install, deployment, configuration of the machine.
6. Existing behaviour that the task changes and no test covers.

Keep each high-risk change in a task with nothing unrelated to it, unless that splits one behaviour.
Name its items in the task's `risk` and add its differential check to its `differential` (see `Tests`).

### Tests

Test first: a task's `tests` say how each new test fails before the change, and its `files` what makes it pass.
Label each test `new`, which fails before the change, or `kept`, which passes before and after it.

- Test in two loops: the outer one checks the journeys and the criteria, the inner one the parts that build them.
- Outer loop: one `new` test per journey and per criterion the task covers, end to end through the public interface.
  It checks every observable effect of its `then` and `but`.
  The effects are the output, the exit code or error returned and, when an input is rejected, the state unchanged.
- A journey `kept: true` has a `kept` test, or the differential check when no test can run both versions.
- Inner loop: unit tests of the parts, `new` too, only where a part has logic the outer test cannot reach.
- Each `new` test names in its `covers` the journeys `CUJ-NN` or the criteria `AC-NN` it checks.
- Add a `kept` test for each existing command, option or function that calls the code the task changes.
  Skip it when an existing test already covers that behaviour.
- A refactor kept by existing tests, or a change no test can observe such as docs, has no `new` tests.
  Its `tests` list only the `kept` tests that stay green, or `[]` when none applies, and its `summary` says why.
- A high-risk task adds a differential check to its `differential`.
  It runs the affected commands, or the tests of the affected functions, with the existing and the new inputs.
  It compares the base version with the working tree.
  The base version is `HEAD` exported with `git archive HEAD | tar -x -C <temporary directory>`.
  Any difference in output, errors or exit code that no criterion asks for is a regression.
  A difference that comes from changes in the working tree before the run, earlier iterations included, is not.
- Tests and the programs they run write only to temporary directories, never inside the working tree.

### Critic prompt

What to send `critic` at each round of Step 6, in the language of the session:

```text
Plan: <the path of the plan file>
Spec: <the spec the user confirmed, whole, with the answers of every reopening>
Template: <the path of the template.yaml next to this skill>
Skill: <the path of this SKILL.md, whose sections High risk, Tests and Readability the plan follows>
Rules: <the path of the folder of rules, or "none" when it does not exist>
Round: <the round, as 1 of 3>
Previous: <each finding of the earlier rounds, with fixed, answered by the user or left open, or "none">
```

### Readability

A reviewer reads `overview` and `specification` to approve the plan; the executor reads the parts it needs.

- `overview` is an executive summary for a reader who reads nothing else.
  Its `summary` is one sentence with the outcome, then `changes`, `not_changes` and `risk_and_size`.
  Its `example` is the id of the simplest journey, never its data again.
  It names no path, identifier or code jargon; the keys that follow may.
- Each fact has one owner, and the other places name it by its id instead of restating it:
  - The behaviour belongs to `outcome`, and the contract, its names, fields and errors, to `interface`.
  - Each exact string, message or output belongs to the step that first shows it, and other places name its id.
  - A journey shows a case, and the criterion that checks it names it in `cujs`, never restating its values.
  - Each decision lives once in `decisions`: the choice and its source, never the behaviour again.
  - `design` says how the parts are built and names `interface` without restating it.
    It stops at the modules, types, functions and contracts other modules use, never a private helper.
  - A task's `files` list paths, and its `summary` never repeats its data.
- Write plain English in short, whole sentences, with the connectors that tie them: so, but, then, because.
  Prefer the possessive and compound nouns to chains of "of".
  For example, "the line's exit code" or "the interview answer", never "the exit code of the line".
- An enumeration is a list, one item per line, never a chain of sentences that start with "Also".
- Use a term as `terms` defines it, and define in `terms` every term a reviewer new to the request would not know.

### Plan file

Path: the `Plan` of `Input` when the request gives one.
Otherwise `<plans>/<name>.yaml`, where `<plans>` is the folder of plans; report the path.
Name it as the project names its plans: the convention of `AGENTS.md`, of `CLAUDE.md` or of the plans already there.
When the plans already there mix conventions, follow the one last added in the git history.
Without one, `<name>` is short kebab-case English from the request.
When another plan already has the name, choose a different one; this applies only to a new plan.
The path is fixed once the file is first written, even when its title or scope changes.
Never rename, copy or move it, nor write it again under another name.

Write the whole file in English, whatever the language of the session or of the docs.
Only the literals keep their language: the strings, messages, commands and data the code must reproduce as is.
Write each text one sentence per line, never wrapped, with no line over 120 characters.
Rewrite a sentence longer than 120 characters as two whole sentences, never cut it into fragments.
The `text` of a literal keeps its lines as the code must reproduce them, at any length.
Describe in prose what changes and why, naming the paths, symbols and contracts involved.

YAML, so that every program and agent reads the plan the same:

- The keys go in the order of template.yaml, with a blank line between the keys of the first two levels.
- A value is plain only when it holds no `: ` nor ` #` and does not start with an indicator of YAML.
  The indicators are `` ` ``, `"`, `'`, `[`, `{`, `*`, `&`, `!`, `|`, `>`, `%`, `@`, `-`, `?` and `,`.
- Any other text goes in a literal block `|`, even a single line; never quote it nor escape it.
- Every collection is a list of objects with named fields, never a map whose keys are free text or ids.
  Each item of `journeys`, `acceptance`, `constraints`, `decisions` and `tasks` opens with its `id`, as each `literal`.
  Ids are unique in the plan.
- A list of ids or paths is a flow list: `[CUJ-01, CUJ-02]`.
- An empty list is `[]`, and a missing value is `none`: never delete a key of the template.
  The only optional key is the `literal` of a step, present when the step shows exact data.

Create the file at Step 5 from [template.yaml](template.yaml), with its keys in its order.
Write `status: planned` and `created` first, and fill each key as Step 5 writes it.
By the end of Step 5, every `<...>` is replaced and every comment of the template is deleted.

The top-level keys, beyond what their placeholders say:

- `version`: `1`, the version of the format of the plan.
- `status`: `planned` when you write the plan, and `approved` once the user approves it (see Step 7).
  The build moves it to `in-progress`, then to `done` or `blocked`.
- `risk`: its `level` and the `reason` for it, in one line.
  `level` is `high` when a task touches an item of `High risk`.
  `medium` when one changes an internal contract or code that several callers share; `low` otherwise.
- `agents`: one item per subagent of the build, `builder`, `verifier` and `reviewer`, in that order.
  Each has its `name`, without the prefix of a plugin, and the `model` and `effort` the build launches it with.
  Each name appears once, and only these three.
  A value the user gives wins, and `inherit` leaves it to the frontmatter of the agent.
  Propose them from `risk.level`, and show them at Step 7 so the user may change them:
  - `low`: builder `sonnet` and `high`, verifier `sonnet` and `medium`, reviewer `sonnet` and `high`.
  - `medium`: builder `opus` and `medium`, verifier `sonnet` and `high`, reviewer `opus` and `high`.
  - `high`: builder `opus` and `medium`, verifier `sonnet` and `xhigh`, reviewer `fable` and `medium`.
- `reference`: the ticket, issue or link of the request or the conversation, or `none`; never invent one.
- `branch`: the branch Step 4 created or switched to for the plan; the build switches to it first.
- `artifacts`: the external resources of the change the user gives or the conversation names, or `[]`.
  Each is the link or path of an artifact of Claude Design, a prototype, a diagram, a mock-up or a document.
  Never invent one.
  The plan stays self-contained: what an artifact settles goes to `outcome`, `constraints` or a decision in words.
- `iterations`: `[]`; the item of the template shows the format the build appends, one per iteration.
- `stats`: each of its fields `none`; the build sets them.

The plan holds no derived data: no count of tasks or iterations, no list of progress, no checklist of criteria.
The `status` of each task and criterion is their progress.
Reference code by path and symbol (function, type, method), never by line number.

Never write placeholders:

- "TBD", "TODO", "implement later", "fill in details".
- "Add appropriate error handling" or "handle edge cases" without saying which and how.
- "Similar to T1" or "as above": the executor reads each task with only the code and the plan.
- Types, functions or files defined neither in a task nor in the code.

### Adjusting a plan

Revise the file in place with targeted edits, never writing it again whole nor elsewhere.
At Step 2, `grill-me` reopens only the decisions the adjustment touches and the ones that depend on them.
Read first `iterations`, the last one first.
Their departures and open findings are what the adjustment usually answers.
Bring the new requirements of the user into `outcome`.
Keep the ids of the literals, journeys, criteria and tasks, and add new ones after the last.
Set back to `status: pending` and `evidence: none` a criterion that changes, or that a task to do covers.
Never rewrite a task with `status: done`: it is built, so what changes it goes in a new task.
Rewrite in place a task `pending`, `departed` or `blocked`, and set it `pending` with `evidence: none`.
Add a new task after the last one; never renumber the tasks, since `iterations` names them.
With a task not `done`, set `status` back to `planned` until the user approves the adjustment (see Step 7).
Recompute `risk`, never `created`, `branch`, `iterations` nor `stats`.
When `risk.level` changes, propose `agents` again from it; otherwise keep it, since the user may have chosen it.
A plan without `agents` gets it proposed from `risk.level`.
A plan with `risk` and `risk_reason` as two keys gets them as `risk.level` and `risk.reason`.
Add the new `artifacts`.
Keep `iterations` always as it is.
