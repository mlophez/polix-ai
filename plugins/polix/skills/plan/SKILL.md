---
name: plan
description: >
  Writes an outcome-driven plan for a feature, refactor or bugfix in the folder of plans the start context names.
  It interviews the user on what the request leaves open, replaces plan mode and never writes code nor commits.
  It runs only when the user or the workflow of polix asks for it.
effort: high
model: inherit
user-invocable: true
disable-model-invocation: true
argument-hint: "[what to plan, e.g. add retries to the webhook sender, and where to save it; empty: the conversation]"
---

# Planning

Turn a request into a plan file that one executor runs alone, task by task in order, knowing only the plan and the code.
The plan goes from the outcome expected to the criteria that prove it and the tasks that build it, and holds no code.
The plan is a YAML file: each fact lives in one place, and the others name it by its id.
The folders of plans, of rules and of tasks are those the start context of polix names, below the root.

## Requirements

- [template.yaml](template.yaml) next to this file.
  Without it, stop and tell the user.
- [template.md](template.md) next to this file, only to adjust a plan in Markdown (see `Markdown plans`).

## Input

- **Request**: what to plan, from the arguments or the conversation.
  Without either, ask the user.
  It may be a file of the folder of tasks, as `/polix:task` writes it.
  Then plan from its sections, and give its path as the `reference` of the plan.
  Once the plan file exists, set the task's `status: planned` and `plan: <path of the plan>`, and nothing else of it.
- **Plan**: the plan file, from the arguments or the conversation.
  A file that already exists is the plan to adjust.
  So is the plan the task's `plan` names, or one written earlier in this session, even with a new scope or title.
  A path that does not exist is where the new plan goes, used as is, outside the folder of plans too.
  A path ending in `/` is a folder: name the file inside it as `Plan file` says.
  Without a path, write the new plan where `Plan file` says.
  When two of these name different files, ask the user which one is the plan.

## Output

- **Plan**: the path of the plan file and its `status`; Step 8 already showed its outcome, so never repeat it.
- **Decisions**: every decision of the plan with `source: delegated`, or "none".
- **Next steps**: adjust the plan, or implement, review and check it with `/polix:build <plan>`.
- **Pending**: open blockers and stale or missing docs, with `/polix:add-rule` when rules are missing; or "none".

## Limits

- Never call `EnterPlanMode`; in plan mode, follow `Plan mode`.
- Approval never starts the build: Step 9 still applies.
- Explore read-only: you write only the plan file and the frontmatter of its task, and run only the project's tests.
- Define tasks, never code: no snippets, implementations or signatures (see `Plan file`).
- One plan is one change, delivered whole: never phases or follow-up plans.
- Reference the rules of the folder of rules by id in the `rules` of each task, never restate them.
- Name no executor, agent, skill or tool of the agent in the plan file; the commands of the project are fine.
- Interview in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Plan progress:
- [ ] Step 1: Resolve the request and the plan to adjust, if any (see `Input`).
- [ ] Step 2: Read the project context and explore the docs and the code yourself (see Step 2).
- [ ] Step 3: Brainstorm the approaches and the cases the request does not consider (see Step 3).
- [ ] Step 4: Settle with the user the problem and the split (see Step 4).
- [ ] Step 5: Write `overview` and `specification`, asking what they lack in rounds (see Step 5).
- [ ] Step 6: Write `approach` and `tasks`, asking the approach first and what they lack in rounds (see Step 6).
- [ ] Step 7: Self-review the plan file and fix every gap (see Step 7); in plan mode, then call `ExitPlanMode`.
- [ ] Step 8: Show the user the outcome of the plan and ask for approval (see Step 8).
- [ ] Step 9: Report (see `Output`), stop and return control to the user.
```

Never reach Step 8 before Step 7 is done: the self-review is what the approval relies on.

### Step 2: Read the project context and explore the docs and the code yourself

1. Read the project context (see `Project context`).
2. Run the test command of the project once in the background, editing nothing.
   Its failures, when any, go to `context` as failures that already exist; when it cannot run, say so in `Pending`.
   When adjusting a plan, skip it unless the adjustment adds tasks.
3. Find with `rg` the areas the request touches: the modules, packages or commands.
4. Explore each area yourself with `rg` and targeted reads, never whole directories.
   Find what `Explore prompt` lists: behaviour and docs, code paths, what to reuse, tests and callers.
5. Read each file a task will change and each symbol the plan will name: never name one from a search hit alone.
6. The code wins over the docs when they disagree, and the gap goes to `Pending`.

Delegate a search to an `Explore` subagent of Claude Code only when reading it yourself would flood the context.
That is a sweep of a large or unknown codebase whose conclusion alone matters, such as every caller of a symbol.
Or an area unrelated to the files the plan changes, such as another repository.
Launch at most two, in one message, with the prompt of `Explore prompt`, breadth "medium" and the model `haiku`.
A report is a lead, never a fact: read yourself what the plan names from it (see item 5).

### Step 3: Brainstorm the approaches and the cases the request does not consider

1. **Approaches**: when more than one reasonable approach exists, draft two to four distinct ones.
   For each, its core idea, how it fits the architecture and its trade-offs: complexity, risk, effort, blast radius.
   Discard, with a one-line reason, those that break a rule, a constraint or the architecture.
2. **Cases**: walk the change against each of these and note the cases the request does not settle:
   - Inputs: empty, missing, duplicated, too large, malformed, with special characters, at each boundary.
   - Failures: each dependency, file, network call or command that can fail, time out or answer late.
   - State: the data, configuration and files already there, and how they reach the new behaviour.
   - Interactions: the features, commands and callers that share the code or the data the change touches.
   - Compatibility: older versions, platforms, settings and users of the contracts that change.
   - Operation: concurrent runs, retries, partial runs and how to undo the change.
   - Security: who may trigger the change, what it exposes and what it trusts.
3. **Pre-mortem**: imagine the change shipped and failed a month later; write the likeliest reasons.
   Each goes to `risks` with its mitigation, or naming the cross-cutting concern that covers it without restating it.
   What the design always does goes to `cross_cutting`, never to `risks`.
4. **Size**: estimate the tasks and changed lines, to know whether the change fits one review (see Step 4).
5. Tag each case with the key it belongs to (see `Sections`).
   Settle a case only when the request, the docs, the code or a rule settle it.
   Every other case is asked in its key at Step 5 or 6.

### Step 4: Settle with the user the problem and the split

- Ask first what changes the whole plan, before any key (see `Questions`).
- When the problem is unclear enough to change the outcome, ask it here (see `Sections`).
- When the change is too big for one review, propose splitting it into plans, each with its outcome and dependencies.
  Split by function, never by layer (data, API, UI) or into preparatory steps.
  Once the user confirms, plan only the part they pick and list the others in the `out` of `scope`.
- Never ask the approach here: it waits for Step 6, once the outcome settles what it must build.
- When nothing is open at this level, skip the step and say so.

### Step 5: Write `overview` and `specification`, asking what they lack in rounds

1. Create the plan file from [template.yaml](template.yaml) (see `Plan file`), or open the plan to adjust.
2. Draft each key of `overview` and `specification` in the order of template.yaml, as `Sections` says.
   Draft it from the request, the docs, the code, the answers so far and the cases of Step 3 tagged to it.
3. List what the drafts lack: each fact, value or choice that the request, the docs, the code and a rule do not settle.
4. Ask those gaps together, in rounds, the ones that change the most first (see `Questions`).
   Each question names its key; without gaps, go on without asking.
5. Write the keys with the answers into the plan file, and the decisions on the outcome into `decisions`.
   An answer that changes a key already written: edit that key too before going on.

### Step 6: Write `approach` and `tasks`, asking the approach first and what they lack in rounds

Do as Step 5 with `approach` and `tasks`, on the plan file Step 5 left.

1. With more than one approach from Step 3, ask it in the first round.
   It goes to `decisions` with `on: approach`, and the ones not chosen go to its `discarded`.
2. An answer that changes the outcome: edit `specification` too, with its journeys and criteria, before going on.

### Step 7: Self-review the plan file and fix every gap

Fix each gap with a targeted edit of the plan file at its path, never by writing it again whole or elsewhere.

1. **Coverage**: `outcome` solves the `problem`, and every requirement of the request is in `outcome` and a criterion.
   Every system of `affected` whose contract changes is in `interface` and in a task's `risk`.
   Every non-functional requirement of the request has a criterion with its threshold.
   Every behaviour of `outcome` has a journey, and every journey is in the `cujs` of a criterion.
   Every criterion has one EARS `requirement`, a `verify` and a task whose `covers` names it.
   Every answer of the interview and every case of Step 3 has its place.
   No criterion asks for what the user did not.
   Every decision comes from the request, the docs, the code, a rule or an answer or delegation of the user.
   A choice of yours found here is a question: go back to Step 5 or 6 for the key it belongs to.
2. **Placeholders**: none of the patterns of `Plan file`, no `<...>` and no comment of template.yaml left.
   `iterations` is `[]`, never the item of the template.
3. **References**: every `LIT-NN`, `CUJ-NN`, `AC-NN`, `CON-NN`, `DEC-NN` and `TN` the plan names exists.
   So does every rule id.
4. **One owner**: each fact lives in one place (see `Readability`).
   Each literal is in one step once and nowhere else, and no criterion restates the values of its journeys.
   Every message or exact string `interface` names is the literal of a step; a missing one is a missing journey.
   The decisions are only in `decisions`, and `overview.example` is the id of a journey.
5. **Order**: run the tasks in your head one by one: each holds what `Tasks` says and stays within `constraints`.
6. **Tests and risk**: each task follows `Tests` and `High risk`.
7. **Rules**: every id in a task's `rules` exists and is not retired, and no rule that applies to the task is missing.
8. **One change**: nothing is deferred to another phase or plan, except the parts of a confirmed split.
9. **No code**: no snippets or implementations; the `text` of a literal holds only data of the outcome.
10. **YAML**: the file parses, with the keys and the order of template.yaml, and every plain value follows its rule.
    Parse it with the first of `yq`, `python3` with `yaml`, or `ruby -ryaml` that runs.
    When none runs, check each plain value by eye and say in `Pending` that the plan was not parsed.
11. **Readability**: read the plan as a reviewer who knows the project but not the request (see `Readability`).
    `overview` holds no path, identifier or code jargon.
    Each task's `summary` reads alone, without its data.
    Every `requirement` reads alone, and every `verify` names the test or case that proves its criterion.

### Step 8: Show the user the outcome of the plan and ask for approval

- Show, in the language of the session, the final result the plan builds, with the path of the plan.
  It is the `summary` of `overview`, the `interface` and the journeys, `scope` and the titles of the criteria.
- Ask the user to approve it, with `AskUserQuestion` or in free text: approve, or adjust with comments.
- Approved: set `status: approved`, and nothing else.
- Adjust: go back to Step 5 or 6 with the comments, keeping `status: planned`, for the keys they touch.
  Then show the result again.
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
- The commands of `AGENTS.md` feed `verification`, and so does `polix rules check` when a rule of a task has a `check`.
  A `check` is the file that decides the rule, never a command to run.
- Without facts in `AGENTS.md` or without the folder of rules, follow the conventions of the surrounding code.
  Say what is missing in `Pending`, never in the plan.

### Request

- Every requirement of the request, and of each free-text answer that changes it, goes to `outcome`.
  Never drop nor weaken one: the review checks the work against `outcome`.
- An option picked in the interview goes to `decisions`, with `on: outcome` when it shapes the outcome.
  It takes `on: approach` when it shapes how the outcome is built.
- The literals quote verbatim the strings, messages, commands and data of the request.
- Never add a requirement the user did not ask for.
- Never make a choice yourself: what the request, the docs, the code or a rule do not settle, ask (see Steps 5 and 6).
  Never leave a choice out of the interview because it looks minor, obvious or not decisive.
- A limit on how the solution is built is a constraint, and a measurable property of the result is a criterion.
  A choice among options is a decision.

### Sections

Walk the keys in the order of template.yaml; each one says where it comes from and what to ask when it lacks something.

- `problem`: from the request; ask who suffers what, its cost and why now, and never invent them.
  Give the cost as a figure only when the request or an answer gives one.
- `affected`: from the request and the code; ask the users, teams and systems the code does not show.
- `context`: from the code; never asked.
- `terms`: from the docs and the code; ask the meaning of a term only the user knows.
- `outcome`, `interface` and `journeys`: ask what would let two executors build different results.
  That is each name, field, default and error of `interface` the request leaves open.
  Ask the exact result of each journey, and the boundaries and errors of the cases tagged to it.
- `scope`: ask what the user leaves out and why, and the areas the code does not show.
- `constraints`: ask the compatibility, platforms, versions and dependencies the docs and the code do not settle.
  Number them `CON-01`, `CON-02`...; never renumber them, since the decisions, risks and tasks name them.
- `acceptance`: ask the threshold of each non-functional requirement and the steps of each manual check.
- `decisions` with `on: outcome`: ask each choice on the outcome with more than one reasonable option.
  Number all the decisions `DEC-01`, `DEC-02`...; never renumber them, since the design, risks and tasks name them.
- `design`: ask the components, data model and error handling the code does not settle.
  It names what other modules use, never a private helper: the builder chooses those.
  Ask the cross-cutting concerns of the cases of Step 3 tagged to it: state, operation and security.
- `decisions` with `on: approach`: ask the approach among those of Step 3, first (see Step 6).
  Then code location, names and task order when they have more than one option.
- `risks`: from the pre-mortem of Step 3; ask a mitigation only when it is a choice of the user.
- `verification` and `tasks`: from the code and the earlier keys; never asked.
  A question they raise belongs to an earlier key: go back to it.

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
- `requirement` reads alone: it never points to another key, such as "as `outcome` says".
- `verify` lists commands with their expected result, or `manual` steps that show the criterion on the finished change.
  It names the tests or cases that prove this criterion, so two criteria never share a vague `verify`.
  The build runs it and sets `status` and `evidence`.
- Number them `AC-01`, `AC-02`...; never renumber them: the `covers` of the tasks name them.

### Tasks

The executor runs every task in one go, in order, each on the tree the earlier ones left, without asking.

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
- Size a task to one reviewable step, about 400 changed lines as a guide, never as a limit.
  Split a task that mixes unrelated behaviours, merge tasks not verifiable apart; never split one behaviour to fit.
- The checks of the whole project go to the `verification` of `approach`: the full test suite, the linter, the build.
  What proves a criterion goes to its `verify`, never to both.
  A task has no checks of its own: the build runs its `tests`, the `verify` of its `covers` and the plan's checks.
- List the tasks in the order they run, with the ids `T1`, `T2`...
- Each task has a `summary` for the reviewer, then its data for the executor (see template.yaml).
  - `summary` says in two or three sentences what the task delivers and how, naming the part of `design` it builds.
    It never restates its tests nor its criteria: `tests` and `covers` hold them.
  - `files` lists the paths the behaviour of the task lives in, with no description: the `design` says what changes.
    Never list the callers, comments, fixtures or docs that only follow the change: the build finds them.
  - `status: pending` and `evidence: none`: the builder sets them.

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
- A refactor kept by existing tests, or a change no test can observe such as docs, takes `tests: []`.
  Then its `summary` says why, and its `tests` list only the `kept` tests that stay green, or `[]` when none applies.
- A high-risk task adds a differential check to its `differential`.
  It runs the affected commands, or the tests of the affected functions, with the existing and the new inputs.
  It compares the base version with the working tree.
  The base version is `HEAD` exported with `git archive HEAD | tar -x -C <temporary directory>`.
  Any difference in output, errors or exit code that no criterion asks for is a regression.
  A difference that comes from changes in the working tree before the run, earlier iterations included, is not.
- Tests and the programs they run write only to temporary directories, never inside the working tree.

### Explore prompt

What to find in each area, exploring yourself or through an `Explore` subagent (see Step 2).
Send a subagent this block, with breadth "medium" and the model `haiku`, keeping only the items you need from it:

```text
Area: <the module, package or command to explore, with its paths>
Request: <the request of the user, in one or two sentences>
Known: <the paths and facts you already found, so it does not search them again>
Question: <the one thing to find, such as every caller of a symbol>
Return, with paths and symbols, never whole files nor what Known already says:
- The current behaviour of the area and its why, from the pages of docs/ on it and from the code.
- The code paths the request goes through, and where the docs and the code disagree.
- The utilities, types and patterns to reuse, and the minimal set of files to change.
- The tests of those files and the commands that run them.
- The commands, options and functions that call the code to change.
```

### Questions

- With more open points than one call takes, ask in rounds, the ones that change the most first.
- Each question stands on its own: it names the feature, file or case it is about, never "this" or "the above".
  It says in a clause what you found that leaves it open, since the dialog may hide the text written before it.
- Each option says in one or two sentences how the change behaves with it and its effort, risk and tests.
- The recommended option goes first, with why.
  On single-select questions, the previews show the behaviour side by side.
- Write a longer comparison as text before the call, and keep the questions answerable without it.
- Never ask what the request, the docs or the code already answer.
- Use `AskUserQuestion`, or grouped free text without it.
- Ask an unanswered question again; never fill it with a default.
  Only when the user leaves it to you explicitly, take the recommended option with `source: delegated` in its decision.

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
Otherwise `<plans>/<name>.yaml`, where `<plans>` is the folder of plans the start context names; report the path.
Name it as the project names its plans: the convention of `AGENTS.md`, of `polix.yaml` or of the plans already there.
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
Write `status: planned` and `created` first, and fill each key as Step 5 or 6 writes it.
At Step 7, every `<...>` is replaced and every comment of the template is deleted.

The top-level keys, beyond what their placeholders say:

- `version`: `1`, the version of the format of the plan.
- `status`: `planned` when you write the plan, and `approved` once the user approves it (see Step 8).
  The build moves it to `in-progress`, then to `done` or `blocked`.
- `risk`: `high` when a task touches an item of `High risk`.
  `medium` when one changes an internal contract or code that several callers share; `low` otherwise.
- `reference`: the ticket, issue or link of the request or the conversation, or `none`; never invent one.
- `branch`: the branch the plan is written on, from `git branch --show-current`; the build switches to it first.
  On a detached `HEAD`, ask the user the branch to build on, and never create it.
- `artifacts`: the external resources of the change the user gives or the conversation names, or `[]`.
  Each is the link or path of an artifact of Claude Design, a prototype, a diagram, a mock-up or a document.
  Never invent one.
  The plan stays self-contained: what an artifact settles goes to `outcome`, `constraints` or a decision in words.
- `iterations`: `[]`; the item of the template shows the format the build appends, one per iteration.

The plan holds no derived data: no count of tasks or iterations, no list of progress, no checklist of criteria.
The `status` of each task and criterion is their progress.
Reference code by path and symbol (function, type, method), never by line number.

Never write placeholders:

- "TBD", "TODO", "implement later", "fill in details".
- "Add appropriate error handling" or "handle edge cases" without saying which and how.
- "Similar to T1" or "as above": the executor reads each task with only the code and the plan.
- Types, functions or files defined neither in a task nor in the code.

### Plan mode

In plan mode, Steps 5 to 7 work on a draft that you pass whole to `ExitPlanMode`, since the file is blocked.
The approval of `ExitPlanMode` is the approval of Step 8: save the plan with `status: approved`.
When rejected, go back to Step 5 or 6 with the comments, for the keys they touch.

### Adjusting a plan

Revise the file in place with targeted edits, never writing it again whole nor elsewhere.
At Steps 5 and 6, walk only the keys the adjustment touches and the ones that depend on them.
Read first `iterations`, the last one first.
Their departures and open findings are what the adjustment usually answers.
Bring the new requirements of the user into `outcome`.
Keep the ids of the literals, journeys, criteria and tasks, and add new ones after the last.
Set back to `status: pending` and `evidence: none` a criterion that changes, or that a task to do covers.
Never rewrite a task with `status: done`: it is built, so what changes it goes in a new task.
Rewrite in place a task `pending`, `departed` or `blocked`, and set it `pending` with `evidence: none`.
Add a new task after the last one; never renumber the tasks, since `iterations` names them.
With a task not `done`, set `status` back to `planned` until the user approves the adjustment (see Step 8).
Recompute `risk` and `risk_reason`, never `created`, `branch` nor `iterations`; add the new `artifacts`.
Keep `iterations` always as it is.

### Markdown plans

A plan file in Markdown (`.md`) was written before this format: adjust it in Markdown, never convert it.
Follow [template.md](template.md) for its sections, and keep its own format where it differs.
Its keys are its sections: `#### Task N` for `TN`, `##### CUJ-NN`, `#### AC-NN`, and `Progress` for the tasks' status.
Its tasks hold their data in a `yaml` block under their prose, and its criteria a checklist `- [ ] AC-NN: <title>`.
In it, a task is done when its line of `Progress` is `[x]`; rewrite its line as `- [ ] Task N: <title>` to redo it.
Keep its `tasks` and `iterations` of the frontmatter in step with its tasks and its `## Iterations`.
