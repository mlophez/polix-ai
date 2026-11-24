---
name: spec
description: >
  Writes an outcome-driven plan, on a new branch, in the folder of plans from the brief the skill grill-me confirms.
  It writes the tasks that build it, has the subagent critic review it, and never runs in plan mode, codes nor commits.
  It runs only when the user asks for it.
user-invocable: true
disable-model-invocation: true
argument-hint: "[what to plan, e.g. add retries to the webhook sender, and where to save it; empty: the conversation]"
---

# Planning

Turn a request into a plan file that an executor builds task by task in order, knowing only the plan and the code.
The plan goes from the outcome expected to the criteria that prove it and the tasks that build it, and holds no code.
The plan is a Markdown file with a YAML frontmatter: each fact lives in one place, and the others name it by its id.
The skill `grill-me` interviews the user; this skill turns its brief into the plan and never interviews.
The folders of plans, of rules and of tasks are those the project names in `AGENTS.md`, `CLAUDE.md` or the session.
Without one, they are `ai/plans/`, `ai/rules/` and `ai/tasks/` below the root.

## Requirements

- [spec.md](spec.md), the template of a plan, next to this file.
  Without it, stop and tell the user.
- The skill `grill-me`, which interviews the user each time the brief opens or reopens.
  From a plugin, it and the subagent below carry its prefix, as `<plugin>:grill-me` and `<plugin>:critic`.
  Without the Skill tool, as in Codex, read `../grill-me/SKILL.md` next to this skill and follow it in this session.
- The subagent `critic`, which reviews the plan at Step 6.
  Without it, as in Codex, read `../../agents/critic.md` next to this skill and follow it yourself at Step 6.
  Follow it in a pass apart from the writing, reading only the plan file.
  The critique is then not independent: say so in `Pending`.

## Input

- **Request**: what to plan, from the arguments or the conversation.
  Without either, ask the user.
  It may be a file of the folder of tasks, as the skill `task` writes it.
  Then plan from its sections, and give its path as the `reference` of the plan.
  Once the plan file exists, set the task's `status: planned` and `plan: <path of the plan>`, and nothing else of it.
- **Brief**: a brief that `grill-me` returned and the user confirmed in this session for this request, if any.
- **Plan**: the plan file, from the arguments or the conversation.
  A file that already exists is the plan to adjust, or to close when its `status` is `review` (see `Closing a plan`).
  So is the plan the task's `plan` names, or one written earlier in this session, even with a new scope or title.
  A path that does not exist is where the new plan goes, used as is, outside the folder of plans too.
  A path ending in `/` is a folder: name the file inside it as `Plan file` says.
  Without a path, write the new plan where `Plan file` says.
  When two of these name different files, ask the user which one is the plan.

## Output

- **Plan**: the path of the plan file, its `status` and its `branch`; Step 7 already showed its outcome.
- **Decisions**: every decision of the plan with `source: delegated`, or "none".
- **Next steps**: adjust the plan, or build, verify and review it with the skill `build`.
- **Pending**: open blockers and stale or missing docs, with the skill `add-rule` when rules are missing.
  Also the decisions the user left deferred and the findings of `critic` left open.
  Also the uncommitted changes other than the plan and its task: the skill `build` starts only once they are
  committed; or "none".

## Limits

- Never plan in plan mode, nor call `EnterPlanMode`, since the skill writes the plan file and creates its branch.
  Started in plan mode, stop and tell the user to leave it and run the skill again.
- Approval never starts the build: Step 8 still applies.
- Never ask the user a decision of the plan yourself: `grill-me` asks them (see Steps 2 and 6).
  Ask the user only what `Input` leaves open, the existing branch of Step 4 and the approval of Step 7.
- Write only the plan file and its task's frontmatter, create only the branch of Step 4, run only the project's checks.
- Define tasks, never code: no snippets, implementations or signatures (see `Plan file`).
- One plan is one change, delivered whole: never phases or follow-up plans.
- Reference the rules of the folder of rules by id in the `Rules` of each task, never restate them.
- Name no executor, agent, skill or tool of the agent in the plan file but the subagents of `agents`.
  The commands of the project are fine.

## How to operate

Copy this checklist and track your progress:

```text
Plan progress:
- [ ] Step 1: Resolve the request, the brief and the plan to adjust, if any (see `Input`).
- [ ] Step 2: Get the confirmed brief from `grill-me` (see Step 2).
- [ ] Step 3: Read the project context and the code the tasks will change (see Step 3).
- [ ] Step 4: Create the branch of a new plan (see Step 4).
- [ ] Step 5: Write the plan file from the brief and the code (see Step 5).
- [ ] Step 6: Have `critic` review the plan once, and fix what it finds (see Step 6).
- [ ] Step 7: Show the user the outcome of the plan and ask for approval (see Step 7).
- [ ] Step 8: Report (see `Output`), stop and return control to the user.
```

Never reach Step 7 before Step 6 is done: the critique is what the approval relies on.
With a plan in `review`, follow `Closing a plan` first: it ends the skill, or goes on as an adjustment.
Every time `grill-me` reopens the brief, update the sections it changes, then run Step 6 again whole.
When adjusting a plan, follow `Adjusting a plan` at every step.

### Step 2: Get the confirmed brief from `grill-me`

1. With a `Brief` of `Input`, take it as it is and skip the rest of this step.
2. Otherwise invoke the skill `grill-me` with the request as its subject.
   Give as its `Known` the facts and decisions the conversation or the task file already settle.
   When adjusting a plan, give the plan's current sections as its `Adjust`, with what the adjustment changes.
3. Wait for the brief the user confirms, then go on with Step 3 of this checklist.
   Its `Pending` decisions go to `Pending`, and Step 7 never sets `status: approved` while one is open.

### Step 3: Read the project context and the code the tasks will change

1. Read the project context (see `Project context`).
2. Run the checks the project documents once in the background, editing nothing; without any, skip it.
   Its failures, when any, go to `Context` as failures that already exist; when it cannot run, say so in `Pending`.
   When adjusting a plan, skip it unless the adjustment adds tasks.
3. Take the `Facts` of the brief as leads, never as facts of the plan.
4. Read each file a task will change and each symbol the plan will name: never name one from a search hit alone.
5. The code wins over the docs and the brief when they disagree.
   A gap that changes a decision of the brief reopens it with `grill-me`; any other gap goes to `Pending`.

### Step 4: Create the branch of a new plan

1. When adjusting a plan, skip this step: the plan keeps its `branch`.
2. Choose the plan file's path (see `Plan file`) before writing it.
3. Name the branch as the plan file without its extension, unless `AGENTS.md` or `CLAUDE.md` names a convention.
4. When the branch exists locally (`git branch --list <name>`) or on a remote (`git branch -r --list '*/<name>'`),
   stop and ask the user whether to switch to it or take another name, which the plan file takes too.
5. Create it from the current branch with `git switch -c <name>`, so the uncommitted changes go with it.
   Switch to an existing branch the user chose with `git switch <name>`.

### Step 5: Write the plan file from the brief and the code

1. Create the plan file from [spec.md](spec.md) (see `Plan file`).
   Or open the plan to adjust and edit it as `Adjusting a plan` says.
2. Write each section in the order of spec.md, from the section of the brief `Brief to sections` names for it.
   Follow `Request`, `Outcome`, `Acceptance criteria`, `Tasks`, `High risk` and `Checks` for its form.
3. A section that needs a decision the brief does not settle is a reopened brief: invoke `grill-me` with `Adjust`.

### Step 6: Have `critic` review the plan once, and fix what it finds

The critic reads only the plan file, in a context of its own, as the builder will read it.
It judges whether an executor could build it without deciding anything itself.

1. Launch the subagent `critic` with the prompt of `Critic prompt`, and wait for its report.
2. Sort each finding of the report:
   - Mechanical: the plan, the request, the code, a rule or an answer of the user already settles its fix.
     Fix it with a targeted edit of the plan file at its path, `minor` ones too.
     Never write the plan again whole nor elsewhere.
   - Decision: its fix is a choice no source settles, as a fix `decision` says.
     Invoke `grill-me` once for all of them, with the findings and the plan's current sections as its `Adjust`.
     Write each answer of its brief into its section.
3. Never launch the critic again: it reviews the plan once.
   What the report leaves open, unfixed or unanswered, goes to `Pending`.
   Write each finding there with its severity and its first words.

Without the subagent `critic`, follow `critic.md` yourself as `Requirements` says, once.

### Step 7: Show the user the outcome of the plan and ask for approval

- Write as text, in the language of the session, the final result the plan builds, with the path of the plan.
  It is the summary of `Overview`, the `Interface` and the journeys, `Scope` and the titles of the criteria.
  Then the model and effort of each agent of `agents` in the frontmatter, which the user may change.
- Then the critique in a few lines: the findings fixed, those asked to the user and those left open.
- Then ask whether to approve the plan or adjust it, approve first, as text with the template of `grill-me`
  ([question.md](../grill-me/question.md)), saying the result is just above it.
- With a deferred decision open, reopen it with `grill-me` first; while it stays open, never offer to approve.
- Approved: set `status: approved`, and nothing else.
- Adjust: invoke `grill-me` with the comments as its `Adjust`, keeping `status: planned`.
  Then update the sections, run Step 6 and show the result again.
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
- The commands of `AGENTS.md` feed `Verification`, with the one that runs the checks of the rules when a task's rule
  has a `check`.
  A `check` is the file that decides the rule, never a command to run.
- Without facts in `AGENTS.md` or without the folder of rules, follow the conventions of the surrounding code.
  Say what is missing in `Pending`, never in the plan.

### Request

- Every requirement of the request, and of each free-text answer that changes it, goes to `Outcome`.
  Never drop nor weaken one: the review checks the work against `Outcome`.
- A decision of the brief goes to `Decisions`, with `On: outcome` when it is `what` and `On: approach` when `how`.
- The literals quote verbatim the strings, messages, commands and data of the request.
- Never add a requirement the user did not ask for.
- Never make a choice yourself: what the request, the brief, the docs, the code or a rule do not settle reopens the
  brief with `grill-me`, even when it looks minor, obvious or not decisive.
- A limit on how the solution is built is a constraint, and a measurable property of the result is a criterion.
  A choice among options is a decision.

### Brief to sections

Each section of spec.md comes from a section of the brief of `grill-me`, or from the code when never asked.

- `Overview`: its summary, `What changes`, `What does not change` and `Risk and size` from `Outcome`, `Scope` and
  `Risks`.
  `Problem` from `Problem`, `Affected` from `Affected`, `Terms` from `Terms` and `Context` from the code.
- `Specification`: `Outcome` from `Outcome`, `Interface` from `Contract` and `Journeys` from `Journeys`.
  The exact data of a journey becomes the literal of its step.
  `Scope` from `Scope`, `Constraints` from `Constraints` and `Acceptance criteria` from `Criteria`, each in EARS by
  this skill.
  Number the constraints `CON-01`, `CON-02`...; never renumber them, since the decisions, risks and tasks name them.
- `Decisions` from `Decisions`: `what` is `On: outcome` and `how` is `On: approach`.
  An answer of the user is `Source: interview`, and the approaches discarded go to the `Discarded` of the chosen one.
  Number them `DEC-01`, `DEC-02`...; never renumber them, since the design, risks and tasks name them.
- `Approach`: `Design` from `Approach`, naming what other modules use, never a private helper.
  `Risks` from `Risks`; what the design always does goes to `Cross-cutting`, never to `Risks`.
  `Runtime`, `Cross-cutting` and `Verification` from the code and the earlier sections.
- `Tasks`: from the code and the earlier sections; a question they raise belongs to an earlier section and reopens
  the brief.

### Outcome

- Write `Outcome` before the criteria and the tasks: the criteria prove it and the tasks build it.
- It describes the result as its users and callers see it, never how it is built.
  What a caller notices goes to `Interface`.
  What only a reader of the code notices goes to `Design`, which names `Interface` without restating it.
  A caller may be code: the actor of a journey may be the module that calls an internal contract.
- `Interface` names every command, option, message, file, format, API and event the change adds or changes.
  It gives their names, fields, defaults and errors, one heading per item.
- `Journeys` are critical user journeys, `CUJ-01`, `CUJ-02`..., one per behaviour, boundary and family of errors.
  Never renumber them: the criteria name them.
- Each journey opens with one sentence of its story: "As <actor>, I want <goal>, so that <benefit>".
  The story says why the journey matters; its steps say what happens.
- Each journey has `Given` the starting state, `When` the input or action and `Then` the exact result.
  Each is a list of steps, one item per step, with literal values.
  The result is the output, the exit code and the state left.
  `But` names what must not happen or must stay unchanged, such as the state of a rejected input; "None." otherwise.
- When a step shows exact data the code must reproduce, the step carries it as its literal.
  That is a string, message, request, command, body or output that is part of the contract.
  - It goes inside the item of the step: a line `LIT-NN (exactly):` or `LIT-NN (contains):`, then a fenced block.
  - Its id is `LIT-01`, `LIT-02`..., unique in the plan; never renumber them, since other places name them.
  - The literal lives in the step of the first journey that shows it.
    Another journey, a criterion, `Interface` or a task names its id, never copies its text.
  - `exactly` says the data is the whole result, and `contains` that the result holds it.
  - Put only what the criteria check.
    Leave out, or name as `<variable>`, the values that change on every run, such as generated ids and timestamps.
  - A literal holds data, never code of the implementation; its general shape, fields and types go to `Interface`.
  - A fence of the literal is longer than any run of backticks inside its text.
- A negation goes in the text of a step: Gherkin has no keyword for it.
- Every exact string the code must reproduce is the literal of a step: `Interface` names it, never holds its text.
- A family of errors of one input, such as each way a value can be wrong, is one journey, never one per error.
  Its `Then` has one step per case, each with the input it rejects and its message as the literal.
- Other combinations of the same journey, without a message of their own, go to the tests of its task.
- [journeys.md](journeys.md) shows each kind of journey and a criterion that checks them; never copy its systems.
- A change with no visible effect, such as a refactor, has journeys `Kept: yes` of the behaviour it puts at risk.
  Its criteria compare it with the base version (see `Tests`), and its `Interface` is the structure after it.
- A change to an internal contract has journeys whose actor is its caller, with the contract in `Interface`.
- `Scope` lists under `In` the areas of the code and of the product the plan touches.
  Under `Out` it lists what the plan leaves out and why.

### Acceptance criteria

- Each criterion checks one part of the outcome, and every journey is in the `Journeys` of a criterion.
- Each has its title in its heading, then `Requirement`, `Journeys` and `Verify`.
  Its status lives only in the map `acceptance` of the frontmatter, `pending` when the plan is written.
- `Requirement` is one EARS sentence (Easy Approach to Requirements Syntax), with the system as the subject:
  - Ubiquitous: "The <system> shall <response>".
  - Event: "When <trigger>, the <system> shall <response>".
  - State: "While <state>, the <system> shall <response>".
  - Unwanted behaviour: "If <condition>, then the <system> shall <response>", one for each error the outcome names.
  - Optional: "Where <feature is included>, the <system> shall <response>".
  - Combined, in this order: "While..., when..., the <system> shall <response>".
- `Requirement` states the general rule, and never restates the values of its journeys: `Journeys` names them.
- A non-functional requirement, such as performance, a limit, security or availability, is a criterion of its own.
  Its sentence names a measurable threshold and the conditions it holds under.
  For example: "The export shall finish in under 5 s for 10,000 invoices".
  Its `Journeys` is "none", and its `Verify` measures it.
- A guarantee the change gives is a criterion of its own, never only a sentence of `Cross-cutting` or a comment.
  That covers security or platforms, such as "never reads outside the root" or "a link is never followed".
  It has one journey per way it could break, from the cases of the brief: each input, platform and path that reaches it.
  For example, a walk of folders has a journey for `..`, a link in any segment, a Windows junction and name case.
- `Requirement` reads alone: it never points to another section, such as "as `Outcome` says".
- `Verify` lists commands to run with their expected result, or manual steps that show the criterion on the finished
  change.
  It names the tests or cases that prove this criterion, so two criteria never share a vague `Verify`.
  Each run runs only those tests or cases, such as `go test ./internal/export -run '^TestExport'`.
  Never a check of the whole project, such as the full test suite: it goes to the `Verification` of `Approach`.
  The verifier runs it as written, and the build sets the status from its report.
- Number them `AC-01`, `AC-02`...; never renumber them: the `Covers` of the tasks name them.

### Tasks

The build runs each task alone, in order, without asking: a builder builds it and a reviewer checks it.
Then the build commits it, and the next task starts from that commit; a verifier checks the whole plan at the end.

- Each task leaves the project working: no check of the project that passed before it fails after it.
  Never a task that breaks what a later one repairs.
- Slice by behaviour, never by layer: each task delivers one behaviour that a check shows, across the parts it needs.
  A refactor and a change no check can observe are the only tasks without one.
- Start with the thinnest path that runs end to end, then widen it task by task.
- When the change is hard to make in the current code, put first a refactor that keeps the behaviour, as its own task.
- Change a contract and every caller of it in the same task.
- A contract consumed outside the repository changes by expand-contract.
  The task widens it without breaking the current form.
  For example, it adds a new optional field, or a new version beside the old one.
  A journey `Kept: yes` shows the current consumers still work.
  Removing the old form goes to the `Out` of `Scope`, with the condition to do it.
- Size a task to one behaviour that its own checks show, verified and reviewed apart from the other tasks.
  Split a task that mixes behaviours, and merge tasks not verifiable apart.
- Estimate `Expected files` of each task: its `Files` plus the files that follow its change, such as the callers of
  each contract it changes, counted with `rg`, and their tests.
  A task over 20 gets one review too thin for its lines: split it by behaviour.
  To split the change of a contract with many callers, put first a refactor that keeps the behaviour, or change it by
  expand-contract: widen it, move the callers in later tasks, then remove the old form.
  When no split keeps each task working and verifiable apart, keep it and record why in a decision of `Decisions`
  that names the task, so the user sees it before approving.
- A criterion covered by several tasks is checked at the last task that covers it: list it in the `Covers` of each.
- The checks of the whole project go to the `Verification` of `Approach`, such as the full test suite, the linter or
  the build, from `AGENTS.md`; "None." when the project has none.
  What proves a criterion goes to its `Verify`, never to both.
  A task has no checks of its own but its `Differential`.
  The build runs its `Tests`, the `Verify` of the criteria whose last covering task it is, and the plan's checks.
- List the tasks in the order they run, with the ids `T1`, `T2`..., one heading `### T<N>: <title>` each.
- Each task has a summary for the reviewer under its heading, then its fields for the executor (see spec.md).
  - The summary says in two or three sentences what the task delivers and how, naming the part of `Design` it builds.
    It never restates its tests nor its criteria: `Tests` and `Covers` hold them.
  - `Files` lists the paths the behaviour of the task lives in, with no description: `Design` says what changes.
    Never list the callers, comments or fixtures that only follow the change: the build finds them.
  - `Expected files` is the number of files the task is expected to change, `Files` and the ones that follow.
  - Its status lives only in the map `tasks` of the frontmatter, `pending` when the plan is written.

### High risk

A task is high risk when it touches one of these:

1. Data, or effects that cannot be undone.
2. Security: authentication, authorization, secrets, input from outside.
3. A contract that others consume: an API, an event, a schema, a file format, a command line.
4. Concurrency.
5. Delivery or environment: build, release, install, deployment, configuration of the machine.
6. Existing behaviour that the task changes and no test covers.

Keep each high-risk change in a task with nothing unrelated to it, unless that splits one behaviour.
Name its items in the task's `Risk` and add its differential check to its `Differential` (see `Checks`).

### Checks

A task is checked by its `Tests`, the `Verify` of the criteria it closes and the plan's `Verification`.
When the project gives a method in the context of this skill, follow it for which tests each task needs and how.
Without one, plan the tests the work of each task needs, and none for a task no test can observe.

- A task's `Tests` name the tests it writes or keeps, per test file; "none" when it has none, and its summary says why.
- Label each test `New`, which the task writes, or `Kept`, which exists already and stays green after the change.
- Each `New` test names the journeys `CUJ-NN` or the criteria `AC-NN` it checks, and what it checks.
- A journey `Kept: yes` has a `Kept` test, or the differential check when no test can run both versions.
- A high-risk task adds a differential check to its `Differential`.
  It runs the affected commands, or the tests of the affected functions, with the existing and the new inputs.
  It compares the base version with the working tree.
  The base version is `HEAD` exported with `git archive HEAD | tar -x -C <temporary directory>`.
  Any difference in output, errors or exit code that no criterion asks for is a regression.
  A difference that comes from changes in the working tree before the run, earlier iterations included, is not.
- Tests and the programs they run write only to temporary directories, never inside the working tree.

### Critic prompt

What to send `critic` at Step 6, in the language of the session, and nothing else:

```text
Plan: <the path of the plan file>
```

### Readability

A reviewer reads `Overview` and `Specification` to approve the plan; the executor reads the parts it needs.

- `Overview` is an executive summary for a reader who reads nothing else.
  It opens with one sentence with the outcome, then `What changes`, `What does not change` and `Risk and size`.
  Its `Example` is the id of the simplest journey, never its data again.
  It names no path, identifier or code jargon; the sections that follow may.
- Each fact has one owner, and the other places name it by its id instead of restating it:
  - The behaviour belongs to `Outcome`, and the contract, its names, fields and errors, to `Interface`.
  - Each exact string, message or output belongs to the step that first shows it, and other places name its id.
  - A journey shows a case, and the criterion that checks it names it in `Journeys`, never restating its values.
  - Each decision lives once in `Decisions`: the choice and its source, never the behaviour again.
  - `Design` says how the parts are built and names `Interface` without restating it.
    It stops at the modules, types, functions and contracts other modules use, never a private helper.
  - A task's `Files` list paths, and its summary never repeats its fields.
- Write plain English in short, whole sentences, with the connectors that tie them: so, but, then, because.
  Prefer the possessive and compound nouns to chains of "of".
  For example, "the line's exit code" or "the interview answer", never "the exit code of the line".
- An enumeration is a list, one item per line, never a chain of sentences that start with "Also".
- Use a term as `Terms` defines it, and define in `Terms` every term a reviewer new to the request would not know.

### Plan file

Path: the `Plan` of `Input` when the request gives one.
Otherwise `<plans>/<name>.md`, where `<plans>` is the folder of plans; report the path.
Name it as the project names its plans: the convention of `AGENTS.md`, of `CLAUDE.md` or of the plans already there.
When the plans already there mix conventions, follow the one last added in the git history.
Without one, `<name>` is short kebab-case English from the request.
When another plan already has the name, choose a different one; this applies only to a new plan.
`<name>` holds no `.`: the build names the files of its passes `<name>.<task>.md` next to the plan.
The path is fixed once the file is first written, even when its title or scope changes.
Never rename, copy or move it, nor write it again under another name.

Write the whole file in English, whatever the language of the session or of the docs.
Only the literals keep their language: the strings, messages, commands and data the code must reproduce as is.
Write each text one sentence per line, never wrapped, with no line over 120 characters.
Rewrite a sentence longer than 120 characters as two whole sentences, never cut it into fragments.
The text of a literal keeps its lines as the code must reproduce them, at any length.
Describe in prose what changes and why, naming the paths, symbols and contracts involved.

The frontmatter is the contract the build reads and writes, so every program reads it the same:

- It holds the fields of spec.md, in its order, and nothing else; never delete one.
- A missing value is `none`, and an empty list `[]`.
- A value that holds `: ` or ` #`, or starts with an indicator of YAML, goes between double quotes.
  The indicators are `` ` ``, `"`, `'`, `[`, `{`, `*`, `&`, `!`, `|`, `>`, `%`, `@`, `-`, `?` and `,`.
- `tasks` maps the id of every task of the body to its status, in their order, each `pending`.
- `acceptance` maps the id of every criterion of the body to its status, in their order, each `pending`.

The body is Markdown, in the order of the headings of spec.md:

- The heading `#` is the title of the plan: one line that describes the change.
- Keep the headings of spec.md; a section with nothing to hold says "None." under its heading.
- Each journey, criterion, decision and task is a heading that opens with its id: `#### CUJ-01: <title>`.
  Ids are unique in the plan, so any reader finds each by its id.
- The fields of an item are a list of `**Name**: value`, as spec.md shows them.
- A list of ids or paths is written inline, separated by commas: `CUJ-01, CUJ-02`.

Create the file at Step 5 from [spec.md](spec.md), with its headings in its order.
Write `status: planned` and `created` first, and fill each section as Step 5 writes it.
By the end of Step 5, every `<...>` is replaced and every comment of the template is deleted.

The fields of the frontmatter, beyond what their placeholders say:

- `status`: `planned` when you write the plan, and `approved` once the user approves it (see Step 7).
  The build moves it to `in-progress`, then to `review` or `blocked`.
  From `review`, this skill moves it to `done` when the user closes it (see `Closing a plan`).
- `risk`: its `level` and the `reason` for it, in one line.
  `level` is `high` when a task touches an item of `High risk`.
  `medium` when one changes an internal contract or code that several callers share; `low` otherwise.
- `agents`: one item per subagent of the build, `builder`, `verifier` and `reviewer`, in that order.
  Each has its `name`, without the prefix of a plugin, and the `model` and `effort` the build launches it with.
  Each name appears once, and only these three.
  A value the user gives wins, and `inherit` leaves it to the frontmatter of the agent.
  Propose builder `sonnet`, verifier `sonnet` and reviewer `opus`, all three with `effort: medium`, at every
  `risk.level`, and show them at Step 7 so the user may change them.
- `reference`: the ticket, issue or link of the request or the conversation, or `none`; never invent one.
- `branch`: the branch Step 4 created or switched to for the plan; the build switches to it first.
- `artifacts`: the external resources of the change the user gives or the conversation names, or `[]`.
  Each is the link or path of an artifact of Claude Design, a prototype, a diagram, a mock-up or a document.
  Never invent one.
  The plan stays self-contained: what an artifact settles goes to `Outcome`, `Constraints` or a decision in words.
- `stats`: each of its fields `none`, and `timeline` `[]`; the build sets them.

The plan holds no derived data: no count of tasks or iterations, no list of progress, no checklist of criteria.
The maps `tasks` and `acceptance` of the frontmatter are their progress, and the build writes them.
The build writes the evidence, the iterations and their findings in the files of its passes, never in the plan.
Reference code by path and symbol (function, type, method), never by line number.

Never write placeholders:

- "TBD", "TODO", "implement later", "fill in details".
- "Add appropriate error handling" or "handle edge cases" without saying which and how.
- "Similar to T1" or "as above": the executor reads each task with only the code and the plan.
- Types, functions or files defined neither in a task nor in the code.

### Adjusting a plan

Revise the file in place with targeted edits, never writing it again whole nor elsewhere.
At Step 2, `grill-me` reopens only the decisions the adjustment touches and the ones that depend on them.
Read first the files of the passes of the build next to the plan, `<name>.*.md`, the last one first.
Their lines `Departure:` and their findings left open are what the adjustment usually answers.
Bring the new requirements of the user into `Outcome`.
Keep the ids of the literals, journeys, criteria and tasks, and add new ones after the last.
Set back to `pending` in `acceptance` a criterion that changes, or that a task to do covers.
Never rewrite a task `done` in `tasks`: it is built, so what changes it goes in a new task.
Rewrite in place a task `pending`, `departed` or `blocked`, and set it `pending`.
Add a new task after the last one, and to `tasks`; never renumber the tasks, since the files of the passes name them.
With a task not `done`, set `status` back to `planned` until the user approves the adjustment (see Step 7).
Recompute `risk`, never `created`, `branch` nor `stats`.
Keep `agents` as it is, even when `risk.level` changes, since the user may have chosen them.
Add the new `artifacts`.
Never edit nor delete a file of a pass.

### Closing a plan

A plan in `review` was built and verified, and its findings wait for the user, who closes it or adds tasks to it.

1. Read the files of the passes of the build next to the plan, `<name>.*.md`, and their findings `noted`.
2. Show them, in the language of the session, as the build reports them.
   The `blocking` and `major` ones in detail, grouped by root cause, with their `FND-NN` and `file:line`.
   The `minor` ones only counted, with the paths of the files that hold them.
3. Ask whether to close the plan or to add tasks, close first, as text with the template of `grill-me`
   ([question.md](../grill-me/question.md)), saying the findings are just above it.
4. Close: set `status: done`, and nothing else; end with `Output`.
5. Add tasks: the user names the findings to fix, by `FND-NN` or in their words.
   Invoke `grill-me` with them and the plan's current sections as its `Adjust`, then go on with Step 2 as
   `Adjusting a plan` says.
   Each new task names in its summary the `FND-NN` it fixes, and its `Files` the paths of their `Where`.
   Never edit the files of the passes: a finding fixed by a new task stays `noted` in them.
