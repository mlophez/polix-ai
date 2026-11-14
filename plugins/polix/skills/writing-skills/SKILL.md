---
name: writing-skills
description: >
  Writes or rewrites a skill of Claude Code in a fixed format: frontmatter, fixed sections and a short, imperative body.
  It runs only when the user or the workflow of polix asks for it.
  It never writes a subagent: that is the skill writing-agents.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: false
paths: "**/skills/**"
argument-hint: "[the skill to write or rewrite, e.g. .claude/skills/commit/SKILL.md; empty for the conversation]"
---

# Writing a skill

A skill is a way of doing one task: its input, limits, steps and output, nothing else.
Every skill has the same format, so the same thing is always in the same place.

## Requirements

- [template.md](template.md) next to this file.
  Without it, stop and tell the user.
- For a skill of a plugin, `claude` on the `PATH`.
  Without it, skip `claude plugin validate` and report it in `Pending`.

## Input

- **Task**: what the skill does and when it is used, from the arguments or the conversation.
- **Location**: `.claude/skills/<name>/SKILL.md` for the project or `~/.claude/skills/<name>/SKILL.md` for the user.
  For a plugin, `plugins/<plugin>/skills/<name>/SKILL.md`.
  Ask when the request does not say.
- **Current file**: when the skill exists, rewrite it whole to this format, keeping its behaviour.

## Output

- **File**: path and line count.
- **Changes**: for a rewrite, what was removed, merged or moved, one line each.
- **Pending**: checks, documentation or decisions left to the user, or "none".

## Limits

- Only write the file of `Location` and its supporting files.
- Keep the behaviour of a current file: change what it does only when the request asks for it.
- Never write a placeholder `<...>` nor a commented-out field into the file.

## How to operate

Copy this checklist and track your progress:

```text
Skill progress:
- [ ] Step 1: Resolve the input (see `Input`) and read the current file when it exists.
- [ ] Step 2: Read the conventions on skills of `AGENTS.md`, `CLAUDE.md` and the other skills of the location.
- [ ] Step 3: Write the `description`; if it needs more than 3 sentences, propose splitting the skill.
- [ ] Step 4: Fill template.md (see `Frontmatter` and `Body`) and delete the parts that do not apply.
- [ ] Step 5: Sort each constraint into `Limits` or its step (see `Limits section`).
- [ ] Step 6: Reread the file against `Style`, fix every line that breaks it and check it is under 500 lines.
- [ ] Step 7: For a plugin, run `claude plugin validate plugins/<plugin>` and the tests of the project.
```

## References

### Style

The style of every skill and subagent, also of those `writing-agents` writes:

- Be concise and brief, never verbose: cut every word that adds no instruction.
- Write each sentence on a single line, never wrapped over several; if it does not fit in 120 characters, split it.
- English, imperative, second person, present tense: "Read the plan", not "The skill should read the plan".
- One instruction per step or bullet.
  Give a reason only when the instruction would be misapplied without it, in one clause.
- Write only what the model would not do by default: no explanations of git, Markdown, Jira or general good practice.
- Say each thing once; elsewhere point to its section ("see `Limits`"), supporting file, `docs/` document or skill.
- No preamble, no closing summary, no motivation paragraphs, no restating the description in the body.
- No emphasis words or capitals (`IMPORTANT`, `MUST`, `NEVER`, `CRITICAL`): every instruction has the same weight.
  Bold only for field names.
- Examples only for an exact format (a command, a message, the prompt for a subagent), one code block per format.
- Concrete over vague: paths, commands, tool names and field names, never "the relevant files".
- Lines of at most 120 characters, no tables, no emojis.
- At most 500 lines per file.
  Beyond that, move reference material to supporting files (`<skill>/<topic>.md`).
  The step that needs it says when to read it.
- Follow the conventions on skills of the project's `AGENTS.md` or `CLAUDE.md` and of the tests of the location.

### Frontmatter

Write the fields in the order of [template.md](template.md).
Always write `name`, `description`, `effort`, `model`, `user-invocable` and `disable-model-invocation`.
Omit the others when not needed and never leave a field commented out.

- `name`: kebab-case naming the activity, equal to the folder name.
  Use a gerund plus object (`writing-skills`, `creating-tickets`) or a bare verb (`plan`, `commit`).
  An action noun (`review`, `pull-request`) also works; never a vague noun (`helper`).
- `description`: at most 3 sentences and 400 characters, starting with a verb in third person.
  It is the only part always in context: it says what and when, never how.
  If it needs more, the skill does more than one task: split it.
- `effort`: `low` for mechanical steps, `medium` by default, `high` for analysis or review.
- `model`: `inherit` by default (the model of the session).
  Use `sonnet` or `opus` only when the skill must not run on the model of the session.
- `user-invocable`: written even with its default value; `true` unless only the model may use the skill.
- `disable-model-invocation`: written even with its default value, `false` by default.
  Set it to `true` only for a skill that the user alone launches and no other skill invokes.
  When rewriting, keep the current value.
- `paths`: a glob of the files the skill works on, only when the model should see it only while on them.
- `argument-hint`: written for every user-invocable skill that takes arguments, and only then.
  It is the hint shown after `/<name>`: the arguments in brackets, an example and what an empty argument means.
  Example: `"[page, section or code area, e.g. overview, components/billing; empty for all docs/]"`.
  Always quote it, since YAML reads an unquoted value starting with `[` as a list.
- A skill that invokes another skill lists it in `Requirements`, so that skill keeps `disable-model-invocation: false`.

### Body

The sections of [template.md](template.md), in its order, read like a function.
`Requirements` are its preconditions, `Input` and `Output` its signature and `Limits` its invariants.
`How to operate` is its body and `References` its helpers.

- Required: the title, the opening paragraph, `Output` and `How to operate`.
  `Requirements`, `Input`, `Limits` and `References` only when they have content.
- `Requirements` lists what must exist before the first step (tools, MCP servers, files, branch, ticket).
  It also says what to do when each one is missing.
  `Input` is what the task receives, not what must exist.
- `Output` lists the fields of the answer; the last one says what is pending or blocked.
- Reference content goes under one `## References` section, the last one of the body.
  Each topic is a `### <Topic>` subsection (e.g. `### Jira ticket`), only when a step points to it.
  Without any, omit `## References`.
- `How to operate` is a checklist in a `text` code block that the model copies and ticks as it goes.
  It has a `<Task> progress:` line, then one `- [ ] Step N: <action>` line per step, in order.
  No nested items: a step repeated per file or element says so in its own line.
- A step that needs more than one line gets an optional `### Step N: <action>` subsection after the block.
  Its number and text match its checklist line.
- A phase of the development flow ends its checklist with the step "Stop and return control to the user".

### Limits section

The steps say what to do and in which order; `Limits` say what holds in every step.
A prohibition is no action, so the steps never replace a limit.

- Keep in `Limits` only what holds across steps, of four kinds:
  - **Scope**: what it never touches ("Never edit the code yourself").
  - **Authority**: what it never decides ("Never commit", "Ask only before Step 4").
  - **Trust**: whose word it never takes ("Count nothing as done on the builder's word").
  - **Evidence**: what every claim needs ("Give every finding the code at `file:line` or a command and its output").
- Keep a limit only when it counters a failure the model shows without it: stopping early, asking mid-run, trusting
  a report, inventing findings, going out of scope.
- Move a constraint that applies to one step into that step.
- Drop a limit that a mechanism already enforces: `tools`, a hook that blocks or a check that fails.
  `tools` without `Edit` nor `Write` enforces no read-only access while `Bash` stays: keep that limit.
- Word each limit as "Never ...", "Only ..." or what every element needs, one per bullet.
- Five to eight limits is the usual size; more dilutes the ones that matter.
