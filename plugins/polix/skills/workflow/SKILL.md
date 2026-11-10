---
name: workflow
description: >
  Writes or changes polix.yaml, the workflow polix applies to the sessions of the agents on every event.
  It runs only when the user or the workflow of polix asks for it.
  It never writes the rules (rules.path, ai/rules/ by default): that is the skill add-rule.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: false
paths: polix.yaml
argument-hint: "[what to enforce, e.g. no commit without tests; empty to review the current workflow]"
---

# Writing the workflow

Turn what the user wants to enforce into `polix.yaml`, at the root of the repository, in two layers.
First the shortcuts of `guardrails`, the basics every project wants, each a short list in its one form:
`protected_files` and `protected_branches` (lists of strings), and `forbidden_commands` (`pattern`, `reason`),
`format_on_edit` (`run`, `path`, `message`) and `run_before_done` (`run`, `changed`, `message`), lists of maps.
Then the entries of `on`, for what no shortcut covers: the orchestration of subagents (builders, reviewers), the
ticket and the plan, and any other condition or verb.
`protected_files` and `forbidden_commands` are protections of the engine: they deny before every entry of `on`, for
every actor, without exceptions, also on files git ignores.
polix expands the other three shortcuts into entries of `on`; `polix graph` draws them with the name of their
shortcut.
An entry says when it applies (`when`, `unless`) and what it does (`action`, with one verb: `block`, `run` or
`inject`).
The entries of `on` skip edits of files git ignores: they govern the change, what a commit may hold.
Conditions are facts of the repository, the event and what polix observed by itself: polix measures and never judges.
Judging the code belongs to a reviewer subagent, such as `polix:reviewer` or `polix:rules-reviewer`, whose end
the workflow requires with `finished`.
In `actor` and `finished`, an agent of a plugin goes with its prefix (`polix:reviewer`, never `reviewer`); an agent
of `.claude/agents/` goes by its bare name.

## Requirements

- `polix` on the `PATH`.
  Without it, stop and point to the skill `doctor` of polix.

## Input

- **Request**: what to enforce, from the arguments or the conversation.

## Rules

- Extend the current `polix.yaml`; never rewrite unrelated entries.
- Write a wish a shortcut covers as an item of that shortcut, in its one form, never as an entry of `on`: a path no
  agent writes in `protected_files`, a command no agent runs in `forbidden_commands`, a branch nobody commits on in
  `protected_branches`, a formatter or linter of each edited file in `format_on_edit`, and a check that must pass
  before the end of the turn in `run_before_done`.
- Write an entry of `on` only for what no shortcut says: another event, condition or verb, an exception, a subagent.
- When `protected_branches`, `format_on_edit` or `run_before_done` falls short, write the entries it stands for in
  `on` (see [reference.yaml](reference.yaml)), remove its items and tune the entries.
- When a protected path or a forbidden command needs an exception (an actor, a branch), write it as a `block` in
  `on` with an `unless` instead of an item of `guardrails`, which has no exceptions.
- Write a gate that must hold as a plain `block`, which blocks every attempt until its `unless` holds.
- Add `once: true` to a `block` only for a reminder the agent may rightly decline, such as checking whether the
  docs need an update.
- Write a warning as an `inject`, never as a `block`.
- Require a check with a `run` of polix; never with a condition on a command the AI ran, which never counts.
- Require a subagent with `unless: { finished: <type> }`, and name in the `changed` of the `when` every file it
  looks at.
- Write an exception as a `not` in `when`, or as a condition in `unless`; never put `contains` inside `any`.
- Order the entries of an event by priority: the first entry that blocks ends the evaluation.
- Add `always: true` only to a `run` whose result depends on more than the files, or to an `inject` that must
  reach the AI on every event.
- Put the reason of an entry in a YAML comment above it.
- Write the messages of `block`, `run` and `inject` in the language of the team, as an instruction the agent can
  follow.
- When `polix config check` names an old name with its new spelling (`protected`, `base`, `max_stop_blocks`, `if`,
  `deny`, `record`, `done`, `turn_stop`...), rewrite it as told.
- When the change uses a key, event, condition or verb the file did not use yet, set `requires` to the output of
  `polix version`, unless it is `dev` or `requires` is already newer; never lower `requires`.
- Never remove an option `polix config check` calls unknown before `polix update` rules out an outdated polix.
- Only write `polix.yaml`; rules of the code go to the folder of rules with the skill `add-rule` of polix.
- When `rules.path`, `plans.path` or `reports.path` changes, tell the user to move the files of the old folder.
- Talk to the user in the language of the session.

## References

### Format

[reference.yaml](reference.yaml) is an annotated example of every option of `polix.yaml`.

## How to operate

Copy this checklist and track your progress:

```text
Workflow progress:
- [ ] Step 1: Read reference.yaml (see `Format`), polix.yaml, the folder of rules and the Justfile, Makefile or package.json.
- [ ] Step 2: Ask until each wish maps to a shortcut of `guardrails`, or else to an event, its conditions and one verb.
- [ ] Step 3: Show the items and the entries to add and confirm once before writing.
- [ ] Step 4: Write them and run `polix config check` until it reports no problem.
- [ ] Step 5: Run `polix graph` and show the diagram of the events changed.
```

## Output format

- **Guardrails**: each item added to a shortcut of `guardrails`, with its shortcut, or "none".
- **Entries**: each entry added to `on`, with its event, its verb and whether it is a gate, a reminder or a
  warning, or "none".
- **Diagram**: the output of `polix graph` for the events changed.
- **Try it**: how to see each new gate act in a branch, with `polix log`, before it reaches the team.
- **Pending**: wishes left without an entry and why, or "none".
