---
name: plan
description: >
  Writes an outcome-driven plan for a feature, refactor or bugfix in the folder of plans, ai/plans/ by default.
  It interviews the user on what the request leaves open, replaces plan mode and never writes code nor commits.
  It runs only when the user or the workflow of polix asks for it.
effort: high
model: inherit
user-invocable: true
disable-model-invocation: true
argument-hint: "[what to plan, e.g. add retries to the webhook sender; empty to take it from the conversation]"
---

# Planning

Turn a request into a plan file that one executor runs alone, task by task in order, knowing only the plan and the code.
The plan goes from the outcome expected to the criteria that prove it and the tasks that build it, and holds no code.
The folders of plans and of rules are `plans.path` and `rules.path` of `polix.yaml`, else `ai/plans/` and `ai/rules/`.

## Requirements

- [template.md](template.md) next to this file.
  Without it, stop and tell the user.

## Input

- **Request**: what to plan, from the arguments or the conversation.
  Without either, ask the user.
- **Plan**: the file of the folder of plans to adjust, when the request names or refers to one.
  A plan written earlier in this session is the plan to adjust, also when the request changes its scope or title.

## Output

- **Plan**: the path of the plan file and its `status`.
- **Overview**: the summary of `Overview` of the plan, before its subsections, verbatim.
- **Decisions**: every `Delegated:` decision of the plan, of `Specification` and of `Approach`, or "none".
- **Next steps**: adjust the plan, or implement, review and check it with `/polix:build <plan>`.
- **Pending**: open blockers and stale or missing docs, with `/polix:add-rule` when rules are missing; or "none".

## Rules

- Never call `EnterPlanMode`.
  In plan mode, Steps 6 to 8 work on a draft that you pass whole to `ExitPlanMode`, since the file is blocked.
  The approval of `ExitPlanMode` is the approval of Step 9: save the plan with `status: approved`.
  When rejected, go back to Step 7 with the comments.
- Approval never starts the build: Step 10 still applies.
- Explore read-only: the plan file is the only file you write, and the tests of the project the only thing you run.
- Define tasks, never code: no code blocks, snippets, implementations or signatures.
  The only blocks allowed hold literal data of the outcome, in `Interface` and in the journeys (see `Outcome`).
  Describe in prose what changes and why, naming the paths, symbols and contracts involved.
- One plan is one change, delivered whole: never phases or follow-up plans.
- When the scope is too big, propose splitting it into plans, each with its outcome and dependencies.
  Split by function, never by layer (data, API, UI) or into preparatory steps.
  Once the user confirms, plan only the part they pick and list the others in the `Out of scope` of `Scope`.
- Reference the rules of the folder of rules by id in the `Rules:` of each task, never restate them.
- Name no executor, agent, skill or tool of the agent in the plan file; the commands of the project are fine.
- Interview in the language of the session.
  Write the whole plan file in English, whatever the language of the session or of the docs.
  Only the literals keep their language: the strings, messages, commands and data the code must reproduce as is.
- Write the plan file one sentence per line, never wrapped; split a sentence longer than 120 characters.
  Use no tables.
  A block of literal data keeps its lines as the code must reproduce them.

## How to operate

Copy this checklist and track your progress:

```text
Plan progress:
- [ ] Step 1: Resolve the request and the plan to adjust, if any (see `Input`).
- [ ] Step 2: Read the project context (see `Project context`).
- [ ] Step 3: Explore the docs and the code with `Explore` subagents in parallel (see Step 3).
- [ ] Step 4: Brainstorm the approaches and the cases the request does not consider (see Step 4).
- [ ] Step 5: Settle with the user the split and the approach (see Step 5).
- [ ] Step 6: Create the plan file from template.md (see `Plan file`), or open the plan to adjust.
- [ ] Step 7: Walk the sections in order: draft each, ask what it lacks and write it (see Step 7).
- [ ] Step 8: Self-review the plan file and fix every gap (see Step 8); in plan mode, then call `ExitPlanMode`.
- [ ] Step 9: Show the user the outcome of the plan and ask for approval (see Step 9).
- [ ] Step 10: Report (see `Output`), stop and return control to the user.
```

### Step 3: Explore the docs and the code with `Explore` subagents in parallel

1. Find with `rg` the areas the request touches: the modules, packages or commands, at most four.
2. Launch one `Explore` subagent of Claude Code per area, all in one message, with the prompt of `Explore prompt`.
3. Meanwhile, run the test command of the project once in the background, editing nothing.
   Record its result and failures in `Starting point`; when it cannot run, write `not run` and why.
   When adjusting a plan, run it only when `Starting point` changes, on the tree the earlier runs left.
4. Merge the reports: the code wins over the docs when they disagree, and the gap goes to `Pending`.
5. Read yourself each file a task will change and each symbol the plan will name: never name one only from a report.

Without the `Explore` subagent, as in Codex, explore each area yourself with `rg` and targeted reads.

### Step 4: Brainstorm the approaches and the cases the request does not consider

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
   Each goes to `Risks` with its mitigation, or naming the cross-cutting concern that covers it without restating it.
   What the design always does goes to `Cross-cutting concerns`, never to `Risks`.
4. **Size**: estimate the tasks and changed lines, to know whether the change fits one review (see `Rules`).
5. Tag each case with the section it belongs to (see `Sections`).
   Settle a case only when the request, the docs, the code or a rule settle it.
   Every other case is asked in its section at Step 7.

### Step 5: Settle with the user the split and the approach

- Ask first what changes the whole plan, before any section (see `Questions`).
- When the problem is unclear enough to change the approach, ask it here (see `Sections`).
- When the change is too big for one review, propose the split (see `Rules`).
- With several approaches, ask the approach.
  The ones not chosen go to the `Discarded:` of that decision in `Approach`.
- When nothing is open at this level, skip the step and say so.

### Step 7: Walk the sections in order: draft each, ask what it lacks and write it

Take each `###` section of template.md in order, with what `Sections` says of it.

1. Draft the section from the request, the docs, the code, the answers so far and the cases of Step 4 tagged to it.
2. List what the draft lacks: each fact, value or choice that the request, the docs, the code and a rule do not settle.
3. Without gaps, write the section into the plan file and go on without asking.
4. With gaps, ask only about them, naming the section (see `Questions`).
   Then write the section with the answers into the plan file.
5. An answer that changes a section already written: edit that section too before going on.

### Step 8: Self-review the plan file and fix every gap

Fix each gap with a targeted edit of the plan file at its path, never by writing it again whole or elsewhere.

1. **Coverage**: `Outcome` solves the `Problem`, and every requirement of the request is in `Outcome` and a criterion.
   Every system of `Affected` whose contract changes is in the `Interface` of `Outcome` and in `High risk:`.
   Every non-functional requirement of the request has a criterion with its threshold.
   Every behaviour of `Outcome` has a journey, and every journey a criterion.
   Every criterion has one EARS sentence, a `Verify:` and a task whose `Covers:` names it.
   Every answer of the interview and every case of Step 4 has its place.
   No criterion asks for what the user did not.
   Every decision comes from the request, the docs, the code, a rule or an answer or delegation of the user.
   A choice of yours found here is a question: go back to the section of Step 7 it belongs to.
2. **Placeholders**: none of the patterns of `Plan file` and no placeholder of template.md left.
3. **Consistency**: names, contracts and paths match across `Outcome`, the tasks and the code.
   Each term of `Terms` keeps one meaning, and `Progress`, the checklist of criteria and the frontmatter match too.
4. **Order**: run the tasks in your head one by one: each holds what `Tasks` says and stays within `Constraints`.
5. **Tests and risk**: each task follows `Tests` and `High risk`.
6. **Rules**: every id in a `Rules:` exists and is not retired, and no rule that applies to the task is missing.
7. **One change**: nothing is deferred to another phase or plan, except the parts of a confirmed split.
8. **No code**: no code blocks, snippets or implementations.
   A block holds only literal data of `Interface` or of a journey, and says whether it is exact or partial.

### Step 9: Show the user the outcome of the plan and ask for approval

- Show, in the language of the session, the final result the plan builds, with the path of the plan.
  It is the summary of `Overview`, the `Interface` and `Journeys` of `Outcome`, `Scope` and the checklist of criteria.
- Ask the user to approve it, with `AskUserQuestion` or in free text: approve, or adjust with comments.
- Approved: set `status: approved` in the frontmatter, and nothing else.
- Adjust: go back to Step 7 with the comments, keeping `status: planned`, for the sections they touch.
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
- The commands of `AGENTS.md` feed `Verification`, and so does `polix rules check` when a rule of a task has a `check`.
  A `check` is the file that decides the rule, never a command to run.
- Without facts in `AGENTS.md` or without the folder of rules, follow the conventions of the surrounding code.
  Say what is missing in `Pending`, never in the plan.

### Request

- Every requirement of the request, and of each free-text answer that changes it, goes to `Outcome`.
  Never drop nor weaken one: the review checks the work against `Outcome`.
- An option picked in the interview goes to the `Decisions` of `Specification` when it shapes the outcome.
  It goes to the `Decisions` of `Approach` when it shapes how the outcome is built.
- The acceptance criteria quote verbatim the strings, messages, commands and data of the request.
- Never add a requirement the user did not ask for.
- Never make a choice yourself: what the request, the docs, the code or a rule do not settle, ask (see Step 7).
  Never leave a choice out of the interview because it looks minor, obvious or not decisive.
- A limit on how the solution is built is a constraint, and a measurable property of the result is a criterion.
  A choice among options is a decision.

### Sections

Walk them in the order of template.md; each one says where it comes from and what to ask when it lacks something.

- `Problem`: from the request; ask who suffers what, its cost and why now, and never invent them.
  Give the cost as a figure only when the request or an answer gives one.
- `Affected`: from the request and the code; ask the users, teams and systems the code does not show.
- `Context`: from the code; never asked.
- `Terms`: from the docs and the code; ask the meaning of a term only the user knows.
- `Outcome`: ask what would let two executors build different results.
  That is each name, field, default and error of `Interface` the request leaves open.
  Ask the exact result of each journey, and the boundaries and errors of the cases tagged to it.
- `Scope`: ask what the user leaves out and why, and the areas the code does not show.
- `Constraints`: ask the compatibility, platforms, versions and dependencies the docs and the code do not settle.
- `Acceptance criteria`: ask the threshold of each non-functional requirement and the steps of each manual check.
- The `Decisions` of `Specification`: ask each choice on the outcome with more than one reasonable option.
- `Design`: ask the components, data model and error handling the code does not settle.
  Ask the cross-cutting concerns of the cases of Step 4 tagged to it: state, operation and security.
- The `Decisions` of `Approach`: ask code location, names and task order when they have more than one option.
- `Risks`: from the pre-mortem of Step 4; ask a mitigation only when it is a choice of the user.
- `Starting point`, `Verification`, `Progress` and `Tasks`: from the code and the earlier sections; never asked.
  A question they raise belongs to an earlier section: go back to it.

### Outcome

- Write `Outcome` before the criteria and the tasks: the criteria prove it and the tasks build it.
- It describes the result as its users and callers see it, never how it is built.
  What a caller notices goes to the `Interface` of `Outcome`.
  What only a reader of the code notices goes to the `Design` of `Approach`.
  The `Design` names the `Interface` without restating it.
  A caller may be code: the actor of a journey may be the module that calls an internal contract.
- Its `Interface` names every command, option, message, file, format, API and event the change adds or changes.
  It gives their names, fields, defaults and errors.
- Its `Journeys` are critical user journeys, `##### CUJ-01: <title>`, `##### CUJ-02: <title>`...
  There is one per behaviour, plus one per boundary and per error.
- Each journey has `Given:` the starting state, `When:` the input or action and `Then:` the exact result.
  The result is the output, the exit code and the state left, all with literal values.
  `And:` adds another step of the same kind as the one before it.
  `But:` after `Then:` names what must not happen or must stay unchanged, such as the state of a rejected input.
  A negation goes in the text of a step: Gherkin has no keyword for it.
  Many combinations of the same journey go to the tests of its task, never to a table.
- When the content of a request, command, body or output is part of the contract, put it under its step.
  It is a block of literal data, a doc string of Gherkin.
  The request or command goes under `When:`, and the response or output under `Then:`.
  - The step says whether the block is `exactly` the result or what it `contains`.
  - Put only what the criteria check.
    Leave out, or name as `<variable>`, the values that change on every run, such as generated ids and timestamps.
  - A block holds data, never code of the implementation.
    Its general shape, the fields and their types, goes to `Interface`.
- Number the journeys `CUJ-01`, `CUJ-02`...; never renumber them: the criteria point to them.
- [journeys.md](journeys.md) shows each kind of journey and a criterion that checks them; never copy its systems.
- A change with no visible effect, such as a refactor, has journeys `(kept)` of the behaviour it puts at risk.
  Its criteria compare it with the base version (see `Tests`), and its `Interface` is the structure after it.
- A change to an internal contract has journeys whose actor is its caller, with the contract in `Interface`.
- `In scope` lists the areas of the code and of the product the plan touches, each with the journeys that use it.
  `Out of scope` lists what the plan leaves out and why.

### Acceptance criteria

- Each criterion checks one part of the outcome, and every journey is checked by a criterion.
- The checklist has one line `- [ ] AC-NN: <title>` per criterion, followed by one block `#### AC-NN: <title>` each.
- The block has its `Requirement:`, the `CUJs:` it checks and its `Verify:`.
- `Requirement:` is one sentence in EARS (Easy Approach to Requirements Syntax), with the system as the subject:
  - Ubiquitous: "The <system> shall <response>".
  - Event: "When <trigger>, the <system> shall <response>".
  - State: "While <state>, the <system> shall <response>".
  - Unwanted behaviour: "If <condition>, then the <system> shall <response>", one for each error the outcome names.
  - Optional: "Where <feature is included>, the <system> shall <response>".
  - Combined, in this order: "While..., when..., the <system> shall <response>".
- A non-functional requirement, such as performance, a limit, security or availability, is a criterion of its own.
  Its sentence names a measurable threshold and the conditions it holds under.
  For example: "The export shall finish in under 5 s for 10,000 invoices".
  Its `CUJs:` is `none`, and its `Verify:` measures it.
- `Verify:` is a command with its expected result, or the steps that show the criterion on the finished change.
  The build runs it and marks the line: `[x]` when met, `[-]` with why otherwise.
- Number them `AC-01`, `AC-02`...; never renumber them: the `Covers:` of the tasks point to them.

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
  A journey `(kept)` shows the current consumers still work.
  Removing the old form goes to `Out of scope`, with the condition to do it.
- Size a task to one reviewable step, about 400 changed lines as a guide, never as a limit.
  Split a task that mixes unrelated behaviours, merge tasks not verifiable apart; never split one behaviour to fit.
- The checks of the whole project go to the `Verification` of `Approach`: the full test suite, the linter, the build.
  What proves a criterion goes to its `Verify:`, never to both, and neither goes to the `Verification:` of a task.

### High risk

A task is high risk when it touches one of these:

1. Data, or effects that cannot be undone.
2. Security: authentication, authorization, secrets, input from outside.
3. A contract that others consume: an API, an event, a schema, a file format, a command line.
4. Concurrency.
5. Delivery or environment: build, release, install, deployment, configuration of the machine.
6. Existing behaviour that the task changes and no test covers.

Keep each high-risk change in a task with nothing unrelated to it, unless that splits one behaviour.
Name its items in `High risk:` and add its differential check (see `Tests`).

### Tests

Test first: `Tests:` says how each test fails before the change, and `Changes:` is what makes it pass.
Label each test `new`, which fails before the change, or `kept`, which passes before and after it.

- Test in two loops: the outer one checks the journeys and the criteria, the inner one the parts that build them.
- Outer loop: one `new` test per journey and per criterion the task covers, end to end through the public interface.
  It checks every observable effect of its `Then:`, `And:` and `But:`.
  The effects are the output, the exit code or error returned and, when an input is rejected, the state unchanged.
- A journey `(kept)` has a `kept` test, or the differential check when no test can run both versions.
- Inner loop: unit tests of the parts, `new` too, only where a part has logic the outer test cannot reach.
- `Tests:` names the journey `CUJ-NN` or the criterion `AC-NN` each test checks.
- Add a `kept` test for each existing command, option or function that calls the code the task changes.
  Skip it when an existing test already covers that behaviour.
- A refactor kept by existing tests, or a change no test can observe such as docs, takes `Tests: none` with why.
- A high-risk task adds a differential check to its `Verification:`.
  It runs the affected commands, or the tests of the affected functions, with the existing and the new inputs.
  It compares the base version with the working tree.
  The base version is `HEAD` exported with `git archive HEAD | tar -x -C <temporary directory>`.
  Any difference in output, errors or exit code that no criterion asks for is a regression.
  A difference that comes from changes in the working tree before the run, earlier iterations included, is not.
- Tests and the programs they run write only to temporary directories, never inside the working tree.

### Explore prompt

Send each `Explore` subagent this block, with breadth "very thorough":

```text
Area: <the module, package or command to explore, with its paths>
Request: <the request of the user, in one or two sentences>
Return, with paths and symbols, never whole files:
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
  Only when the user leaves it to you explicitly, take the recommended option and label it `Delegated:` in its decision.

### Plan file

Path: `<plans>/<name>.md`, where `<plans>` is the folder of plans.
Name it as the project names its plans: the convention of `AGENTS.md`, of `polix.yaml` or of the plans already there.
When the plans already there mix conventions, follow the one last added in the git history.
Without one, `<name>` is short kebab-case English from the request.
When another plan already has the name, choose a different one; this applies only to a new plan.
The path is fixed once the file is first written, even when its title or scope changes.
Never rename, copy or move it, nor write it again under another name.
The files `<name>.iterNN.md` and `<name>.revNN.md` are run files of the build, never plans.

Create the file at Step 6 from [template.md](template.md), with its sections in its order and at its levels.
Write the frontmatter with `status: planned` and `created` first, and fill each section as Step 7 closes it.
At Step 8, every `<...>` is replaced by its content.

The frontmatter, beyond what its placeholders say:

- `status`: `planned` when you write the plan, and `approved` once the user approves it (see Step 9).
  The build moves it to `in-progress`, then to `done` or `blocked`.
- `risk`: `high` when a task touches an item of `High risk`.
  `medium` when one changes an internal contract or code that several callers share; `low` otherwise.
- `reference`: the ticket, issue or link of the request or the conversation, or `none`; never invent one.
- `artifacts`: the external resources of the change the user gives or the conversation names, or `[]`.
  Each is the link or path of an artifact of Claude Design, a prototype, a diagram, a mock-up or a document.
  Never invent one.
  The plan stays self-contained: what an artifact settles goes to `Outcome`, `Constraints` or a decision in words.

The body:

- Use each term as `Terms` defines it, as the docs or the code do, and never two names for one thing.
- `Progress` has one line `- [ ] Task N: <title>` per task, with the number and title of its heading.
  The executor marks it `[x]` when done or `[-]` when it departed, with its evidence; any line not `[x]` is to do.
- `Iterations` is `none`: the build adds one entry per run.
- Reference code by path and symbol (function, type, method), never by line number.

Never write placeholders:

- "TBD", "TODO", "implement later", "fill in details".
- "Add appropriate error handling" or "handle edge cases" without saying which and how.
- "Similar to Task N" or "as above": the executor reads each task with only the code and the plan.
- Types, functions or files defined neither in a task nor in the code.

### Adjusting a plan

Revise the file in place with targeted edits, never writing it again whole nor elsewhere.
At Step 7, walk only the sections the adjustment touches and the ones that depend on them.
Read first `Iterations` and the run files of a run that did not end.
Their departures and open findings are what the adjustment usually answers.
Bring the new requirements of the user into `Outcome`.
Keep the ids of the journeys and of the criteria, and add new ones after the last.
Rewrite the line of a criterion that changes, or that a task to do covers, as `- [ ] AC-NN: <title>`.
Never rewrite a task marked `[x]`: it is built, so what changes it goes in a new task.
Rewrite a task marked `[ ]` or `[-]` in place, and its line of `Progress` as `- [ ] Task N: <title>`.
Insert a new task where its order needs, never before the last line `[x]`.
Renumber the tasks after an inserted one, with their lines of `Progress` and every `Task N` the plan mentions.
`Iterations` keeps the task numbers of its runs.
With a line not `[x]`, set `status` back to `planned` until the user approves the adjustment (see Step 9).
Recompute `risk`, `risk_reason` and `tasks`, never `created`; add the new `artifacts`.
Keep `Starting point` unless the adjustment changes what it says, and `Iterations` always as it is.
