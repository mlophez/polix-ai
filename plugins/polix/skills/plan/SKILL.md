---
name: plan
description: >
  Writes a self-contained implementation plan in the folder of plans (ai/plans/ by default) for a feature, refactor
  or bugfix, before any code, interviewing the user on what the request leaves open; it replaces plan mode.
  One executor runs the plan alone, task by task in order. It never writes code, branches or commits.
  It runs only when the user or the workflow of polix asks for it.
effort: high
model: inherit
user-invocable: true
disable-model-invocation: true
argument-hint: "[what to plan, e.g. add retries to the webhook sender; empty to take it from the conversation]"
---

# Planning

Turn a request into a plan file that one executor runs alone, task by task in order, knowing only the plan and the code.
The plan says what to change, where, why and how to verify it; it never holds code.
The folder of plans is `plans.path` of `polix.yaml` and the folder of rules is `rules.path`.
Without them, `ai/plans/` and `ai/rules/`; polix names both at session start.

## Requirements

- [template.md](template.md) next to this file.
  Without it, stop and tell the user.

## Input

- **Request**: what to plan, from the arguments or the conversation.
  Without either, ask the user.
- **Plan**: the file of the folder of plans to adjust, when the request names or refers to one.
  A plan written earlier in this session is the plan to adjust, also when the request changes its scope or title.

## Rules

- Never call `EnterPlanMode`.
  In plan mode, Steps 6 and 7 work on a draft that you pass whole to `ExitPlanMode`, since the file is blocked.
  Once approved, only save it; when rejected, go back to Step 6 with the comments.
  Approval never starts the build: Step 8 still applies.
- Explore read-only: the plan file is the only file you write, and the tests of the project the only thing you run.
- Define tasks, never code: no code blocks, snippets, implementations or signatures.
  Describe in prose what changes and why, naming the paths, symbols and contracts involved.
- One plan is one change, delivered whole: never phases or follow-up plans.
- When the scope is too big, propose splitting it into plans, each with its outcome and dependencies.
  Split by function, never by layer (data, API, UI) or into preparatory steps.
  Once the user confirms, plan only the part they pick and list the others in `Out of scope`.
- Reference the rules of the folder of rules by id in the `Rules:` of each task, never restate them.
- Name no executor, agent, skill or tool of the agent in the plan file; the commands of the project are fine.
- Interview in the language of the session.
  Write the whole plan file in English, whatever the language of the session or of the docs, `Request` included.
  Only the literals keep their language: the strings, messages, commands and examples the code must reproduce as is.
- Write the plan file one sentence per line, each line at most 120 characters, with no tables.
  `Request` is the exception to this rule: it keeps the lines of the user as they are.

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
  Say what is missing in `Missing docs`.

### Request

- `Request` holds the words of the user translated faithfully into English, never summarized nor reworded: the review
  checks the work against them.
  An option picked in the interview goes to `Decisions`.
  A free-text answer that changes the request goes to `Request`, translated the same way.
- The acceptance criteria quote verbatim the strings, messages, commands and examples of the request.
- Never add a requirement the user did not ask for.
- Never make a choice yourself: what the request, the docs, the code or a rule do not settle, ask (see Step 5).
  Never leave a choice out of the interview because it looks minor, obvious or not decisive.
- A limit the solution respects is a constraint; a choice among options is a decision.

### Tasks

The executor runs every task in one go, in order, each on the tree the earlier ones left, without asking.

- Each task leaves the project building and no test failing that passed at the baseline.
  Never a task that breaks what a later one repairs.
- Slice by behaviour, never by layer: each task delivers one behaviour that a test shows, across the layers it needs.
  A refactor and a change no test can observe are the only tasks without one.
- Start with the thinnest path that runs end to end, then widen it task by task.
- When the change is hard to make in the current code, put first a refactor that keeps the behaviour, as its own task.
- Change a contract and every caller of it in the same task.
- Size a task to one reviewable step, about 400 changed lines as a guide, never as a limit.
  Split a task that mixes unrelated behaviours, merge tasks not verifiable apart; never split one behaviour to fit.
- The checks of the whole project go to `Verification`, never to the `Verification:` of a task.

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

- One `new` test per requirement of the request, checking every observable effect through the public interface.
  The effects are the output, the exit code or error returned and, when an input is rejected, the state unchanged.
- Cover the cases the requirement names: its examples, its boundaries and its errors.
- For each existing command, option or function that calls the code the task changes, a `kept` test that its behaviour
  holds, unless an existing test already covers it.
- A refactor kept by existing tests, or a change no test can observe such as docs, takes `Tests: none` with why.
- A high-risk task adds a differential check to its `Verification:`.
  It runs the affected commands, or the tests of the affected functions, with the existing and the new inputs.
  It compares the base version with the working tree.
  The base version is `HEAD` exported with `git archive HEAD | tar -x -C <temporary directory>`.
  Any difference in output, errors or exit code that no criterion asks for is a regression.
  A difference that comes from changes in the working tree before the run, earlier iterations included, is not.
- Tests and the programs they run write only to temporary directories, never inside the working tree.

### Questions

- `AskUserQuestion` takes at most 4 questions per call with 2 to 4 options each.
  With more open points, ask in rounds, the ones that change the most first.
- Each question stands on its own: it names the feature, file or case it is about, never "this" or "the above".
  It says in a clause what you found that leaves it open, since the dialog may hide the text written before it.
- Each option says in one or two sentences how the change behaves with it and its effort, risk and tests.
- The recommended option goes first, with why; on single-select questions, the previews show the behaviour side by side.
- Write a longer comparison as text before the call, and keep the questions answerable without it.

### Plan file

Path: `<plans>/<name>.md`, where `<plans>` is the folder of plans.
Name it as the project names its plans: the convention of `AGENTS.md`, of `polix.yaml` or of the plans already there.
When the plans already there mix conventions, follow the one last added in the git history.
Without one, `<name>` is short kebab-case English from the request.
When another plan already has the name, choose a different one; this applies only to a new plan.
The path is fixed once the file is first written: never rename, copy or move it, nor write it again under another
name, even when its title or scope changes.
The files `<name>.iterNN.md` and `<name>.revNN.md` are run files of the build, never plans.

Fill [template.md](template.md): its sections in its order, every `<...>` replaced by what it describes.
Tasks are `### Task 1: ...`, `### Task 2: ...`, in the order they run (see `Tasks`, `High risk` and `Tests`).

The header under the title:

- `Status`: `planned` when you write the plan; the build moves it to `building`, then `built` or `partial`.
- `Risk`: `high` when a task touches an item of `High risk`; `medium` when one changes an internal contract or code that
  several callers share; `low` otherwise.
- `Reference`: the ticket, issue or link of the request or the conversation, or `none`; never invent one.
- `Created`: the date you first write the plan.
- `Size`: the number of tasks and the sum of the lines each one changes, estimated (see Step 4).

The body:

- `Acceptance criteria` are numbered `AC1`, `AC2`...
  Never renumber them: the `Covers:` of the tasks point to them.
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

Revise the file in place with targeted edits of the sections the adjustment touches, never a new file.
Never write the whole file again: only the edits of what changes, at its own path.
Add the new words of the user to `Request`, translated faithfully into English.
Keep the ids of the criteria and add new ones after the last.
Never rewrite a task marked `[x]`: it is built, so what changes it goes in a new task.
Rewrite a task marked `[ ]` or `[-]` in place, and its line of `Progress` as `- [ ] Task N: <title>`.
Insert a new task where its order needs, never before the last line `[x]`.
Renumber the tasks after an inserted one, with their lines of `Progress` and every `Task N` the plan mentions
outside `Iterations`, which keeps the numbers of its run.
With a line not `[x]`, set `Status` back to `planned`; recompute `Risk` and `Size`, never `Created`.
Keep `Starting point` unless the adjustment changes what it says, and `Iterations` always as it is.
Read `Iterations` and the run files of a run that did not end first: their departures and open findings are what
the adjustment usually answers.
A plan without the header or `Progress` keeps its format: never add them.

## How to operate

Copy this checklist and track your progress:

```text
Plan progress:
- [ ] Step 1: Resolve the request and the plan to adjust, if any (see `Input`).
- [ ] Step 2: Read the project context (see `Project context`).
- [ ] Step 3: Explore the docs and the code with `rg` and targeted reads (see Step 3).
- [ ] Step 4: Brainstorm the approaches and the cases the request does not consider (see Step 4).
- [ ] Step 5: Interview the user on what the request, the docs and the code leave open (see Step 5).
- [ ] Step 6: Write the plan file (see `Plan file`), or revise it (see `Adjusting a plan`).
- [ ] Step 7: Self-review the plan file and fix every gap (see Step 7); in plan mode, then call `ExitPlanMode`.
- [ ] Step 8: Report (see `Output format`), stop and return control to the user.
```

### Step 3: Explore the docs and the code with `rg` and targeted reads

- Start from the pages of `docs/` on the area of the request, for the current behaviour and its why.
- Trace the real code paths: the code wins when they disagree, and the gap goes to `Pending` of the output.
- Find the utilities to reuse, the minimal set of files to change, and their tests and test commands.
- Find the commands, options and functions that share the code to change: their behaviour must hold (see `Tests`).
- Run the test command of the project once, editing nothing, and record its result and failures in `Starting point`.
  When adjusting a plan, run it only when `Starting point` changes, on the tree the earlier runs left.
  When it cannot run, write `not run` and why as the expected result.

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
4. **Size**: estimate the tasks and changed lines; when too big for one review, the interview proposes a split first.
5. Settle a case only when the request, the docs, the code or a rule settle it; every other case goes to the interview.
   A settled case goes to the `Tests:` of its task, to `Risks`, to `Constraints` or to `Out of scope`.

### Step 5: Interview the user on what the request, the docs and the code leave open

- A split proposed in Step 4 is the first question, then the approach.
- With several approaches, ask the approach before the details; the ones not chosen go to `Discarded alternatives`.
- Ask about the outcome, the acceptance criteria and the limits of the scope when the request leaves them open.
- Ask about the constraints the request, the docs and the code do not settle: compatibility, platforms, dependencies.
- Ask about the technical design: contracts, data model, error handling, code location, names and task order.
  Also about every case of Step 4 left open, and every choice with more than one reasonable option, however small.
- Never ask what the request, the docs or the code already answer; when nothing is open, skip the interview and say so.
- Use `AskUserQuestion`, or grouped free text without it (see `Questions`).
- Ask an unanswered question again; never fill it with a default.
  Only when the user leaves it to you explicitly, take the recommended option and label it `Delegated:` in `Decisions`.

### Step 7: Self-review the plan file and fix every gap

Fix each gap with a targeted edit of the plan file at its path, never by writing it again whole or elsewhere.

1. **Coverage**: every requirement of `Request` is in a criterion, every criterion in a `Covers:`.
   Every answer of the interview and every case of Step 4 has its place.
   No criterion asks for what the user did not.
   Every decision comes from the request, the docs, the code, a rule or an answer or delegation of the user.
   A choice of yours found here is a question: go back to Step 5 before saving the plan.
2. **Placeholders**: none of the patterns of `Plan file` and no placeholder of template.md left.
3. **Consistency**: names, contracts and paths match across tasks and with the code; `Progress` and the header too.
4. **Order**: run the tasks in your head one by one: each holds what `Tasks` says and stays within `Constraints`.
5. **Tests and risk**: each task follows `Tests` and `High risk`.
6. **Rules**: every id in a `Rules:` exists and is not retired, and no rule that applies to the task is missing.
7. **One change**: nothing is deferred to another phase or plan, except the parts of a confirmed split.
8. **No code**: no code blocks, snippets or implementations.

## Output format

- **Plan**: the path of the plan file.
- **Overview**: the `Overview` section of the plan, verbatim.
- **Decisions**: every `Delegated:` decision of the plan, or "none".
- **Next steps**: adjust the plan, or implement, review and check it with `/polix:build <plan>`.
- **Pending**: open blockers and stale or missing docs, with `/polix:add-rule` when rules are missing; or "none".
