---
name: workflow
description: >
  Writes or changes polix.yaml, the workflow polix applies to the sessions of the agents on every event.
  It runs only when the user or the workflow of polix asks for it.
  It never writes the rules (the folder rules/ below path): that is the skill add-rule.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: false
paths: polix.yaml
argument-hint: "[what to enforce, e.g. no commit without tests; empty to review the current workflow]"
---

# Writing the workflow

Turn what the user wants to enforce into `polix.yaml`, at the root of the repository, in two layers (see `Format`).
polix measures and never judges: judging the code belongs to a reviewer subagent.

## Requirements

- `polix` on the `PATH`.
  Without it, stop and point to the skill `doctor` of polix.

## Input

- **Request**: what to enforce, from the arguments or the conversation.

## Output

- **Guardrails**: each item added to a shortcut of `guardrails`, with its shortcut, or "none".
- **Entries**: each entry added to `on`, with its event, its verb and its kind (gate, reminder or warning), or "none".
- **Diagram**: the output of `polix graph` for the events changed.
- **Try it**: how to see each new gate act in a branch, with `polix log`, before it reaches the team.
- **Pending**: wishes left without an entry and why, or "none".

## Limits

- Extend the current `polix.yaml`; never rewrite unrelated entries.
- Only write `polix.yaml`; rules of the code go to the folder of rules with the skill `add-rule` of polix.
- Never remove an option `polix config check` calls unknown before `polix update` rules out an outdated polix.
- Never lower `requires`.
- Talk to the user in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Workflow progress:
- [ ] Step 1: Read `Format`, reference.yaml, polix.yaml, the folder of rules and the Justfile, Makefile or package.json.
- [ ] Step 2: Ask until each wish maps to a shortcut of `guardrails`, or else to an event, its conditions and one verb.
- [ ] Step 3: Show the items and the entries to add and confirm once before writing.
- [ ] Step 4: Write them (see `Writing entries`) and run `polix config check` until it reports no problem.
- [ ] Step 5: Run `polix graph` and show the diagram of the events changed.
```

## References

### Format

[reference.yaml](reference.yaml) is an annotated example of every option of `polix.yaml`.

- The first layer is the shortcuts of `guardrails`, the basics every project wants, each a short list in its one form.
- They come in the order `polix config fmt` writes them: `builtin`, then the protections, then the lifecycle shortcuts.
- `builtin` comes first: a map of four switches, each `true` when left out, for what polix applies by itself.
  `forbid_commit_no_verify` refuses `git commit --no-verify` or `-n` in every repository.
  `forbid_git_destructive_commands` refuses a forced `git push`, `git reset --hard` and a forced `git clean`.
  Both blocks are presets: items of `forbidden_commands` placed before the written ones.
  `format_polix_files` formats `polix.yaml` and the rules after each edit.
  `check_polix_files` runs `polix config check` on every commit that stages them, so the hooks of git get installed.
- The protections say what no actor may do: `protected_files`, `protected_branches` and `forbidden_commands`.
- The lifecycle shortcuts each run a command at one moment of the work, in the order the work reaches them.
  The verb says what the command does: `exec_` prepares, `format_` changes files, `check_` gates.
- `protected_files` and `protected_branches` are lists of strings.
- `forbidden_commands` (`pattern`, `reason`) is a list of maps.
- `exec_on_start` (`run`, `message`) is a list of maps: it runs on every session start, and a failure only informs.
- `format_on_edit` (`run`, `path`, `message`) is a list of maps: it runs on each edited file of its `path`.
- `format_on_save` (`run`, `changed`, `message`) is a list of maps: it runs right before the agent stages files.
  A failure blocks the `git add` or `git stage` of the agent.
- `check_on_stop` (`run`, `changed`, `message`, `wait_agents`) is a list of maps: it must pass before the turn ends.
  An item of `check_on_stop` waits for the subagents still running unless `wait_agents: false`.
  With `also_on_save: true` it also runs on every commit, after the items of `check_on_save`.
  `run_on_stop` is its old name: rename it to `check_on_stop` wherever it appears.
- `check_on_save` (`run`, `changed`, `message`) is a list of maps: git runs it on every commit, a person's included.
  It takes the vocabulary of `githooks`: its `changed` matches the staged files, and only `{branch}` and `vars` expand.
- The second layer is the entries of `on`, for what no shortcut covers.
  That is the orchestration of subagents (builders, reviewers), the ticket, the plan and any other condition or verb.
- `protected_files` and `forbidden_commands` are protections of the engine: they deny before every entry of `on`.
  They hold for every actor, without exceptions, also on files git ignores.
- polix expands the other shortcuts into entries of `on`, and `check_on_save` into entries of `githooks.pre_commit`.
  `polix graph` draws them with their shortcut name, those of `githooks` in a subgraph `githooks.<event>`.
- An entry says when it applies (`when`, `unless`) and what it does (`action`, with one verb).
  The verb is `block`, `run` or `inject`.
- The entries of `on` skip edits outside the repository; edits of files git ignores fire them like any other.
- Conditions are facts of the repository, the event and what polix observed by itself.
- A reviewer subagent, such as `polix:reviewer` or `polix:rules-reviewer`, judges the code.
  The workflow requires its end with `finished`.

### Writing entries

- Write a wish a shortcut covers as an item of that shortcut, in its one form, never as an entry of `on`.
  A path no agent writes goes in `protected_files`, a command no agent runs in `forbidden_commands`.
  A branch nobody commits on goes in `protected_branches`.
  A command that prepares the environment on every session start goes in `exec_on_start`.
  A formatter or linter of each edited file goes in `format_on_edit`.
  A formatter that must run right before the agent stages files goes in `format_on_save`.
  A check that must pass before the end of the turn goes in `check_on_stop`.
  A check that must pass on every commit, whoever makes it, goes in `check_on_save`.
  Tell the user that `check_on_save` also gates the commits people make.
- Never write the format of `polix.yaml` and the rules, the check of them on commit, the ban of `--no-verify`
  nor the destructive git commands: `guardrails.builtin` applies them by default.
  Remove the items of `format_on_edit`, `check_on_save` and `forbidden_commands` that only repeat them.
  Set a key of `builtin` to `false` only when the user asks to turn that practice off.
- A check that must pass both before the end of the turn and on every commit is one item of `check_on_stop` with
  `also_on_save: true`, never the same item written twice or through YAML anchors.

- Write an entry of `on` only for what no shortcut says: another event, condition or verb, an exception, a subagent.
- When `protected_branches` or a lifecycle shortcut falls short, write the entries it stands for in `on`.
  Those of `check_on_save` go in `githooks.pre_commit`.
  Take them from [reference.yaml](reference.yaml), remove the items of the shortcut and tune the entries.
- When a protected path or a forbidden command needs an exception (an actor, a branch), write a `block` in `on`.
  Give it an `unless` instead of an item of `guardrails`, which has no exceptions.
- Write a gate that must hold as a plain `block`, which blocks every attempt until its `unless` holds.
- Add `once: true` to a `block` only for a reminder the agent may rightly decline.
  An example is checking whether the docs need an update.
- Write a warning as an `inject`, never as a `block`.
- Require a check with a `run` of polix; never with a condition on a command the AI ran, which never counts.
- Require a subagent with `unless: { finished: <type> }`.
  Name in the `changed` of the `when` every file it looks at.
- Name an agent of a plugin with its prefix in `actor` and `finished` (`polix:reviewer`, never `reviewer`).
  Name an agent of `.claude/agents/` by its bare name.
- Write an exception as a `not` in `when`, or as a condition in `unless`; never put `contains` inside `any`.
- Order the entries of an event by priority: the first entry that blocks ends the evaluation.
- Add `always: true` only to a `run` whose result depends on more than the files.
  Add it also to an `inject` that must reach the AI on every event.
- Put the reason of an entry in a YAML comment above it.
- Write the messages of `block`, `run` and `inject` in the language of the team, as instructions the agent can follow.
- When `polix config check` names an old name with its new spelling, rewrite it as told.
  Old names include `protected`, `base`, `max_stop_blocks`, `max_stop_retries`, `if`, `deny`, `record`, `done` and `turn_stop`.
- When the change uses a key, event, condition or verb the file did not use yet, set `requires`.
  Set it to the output of `polix version`, unless it is `dev` or `requires` is already newer.
- When `path` changes, tell the user to move the folders `rules/`, `plans/`, `reports/` and `tasks/` of the old one.
