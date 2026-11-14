---
name: writing-agents
description: >
  Writes or rewrites a subagent of Claude Code in the format of writing-skills plus what a clean context needs.
  It runs only when the user or the workflow of polix asks for it.
  It never writes a skill: that is the skill writing-skills.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: false
paths: "**/agents/*.md"
argument-hint: "[the subagent to write or rewrite, e.g. .claude/agents/reviewer.md; empty for the conversation]"
---

# Writing a subagent

A subagent does one task in a context of its own: it only knows its prompt, its file and the repository.
It follows the format of the skill `writing-skills` with the differences below.

## Requirements

- [template.md](template.md) next to this file and the skill `writing-skills` ([SKILL.md](../writing-skills/SKILL.md)).
  Without them, stop and tell the user.
- For a subagent of a plugin, `claude` on the `PATH`.
  Without it, skip `claude plugin validate` and report it in `Pending`.

## Input

- **Task**: what the subagent does, from the arguments or the conversation.
- **Launcher**: the skill, subagent or session that launches it, and the prompt it sends.
- **Location**: `.claude/agents/<name>.md` for the project or `~/.claude/agents/<name>.md` for the user.
  For a plugin, `plugins/<plugin>/agents/<name>.md`.
  Ask when the request does not say.
- **Current file**: when the subagent exists, rewrite it whole to this format, keeping its behaviour.

## Output

- **File**: path and line count.
- **Launcher**: the file aligned and what changed in it, or "none".
- **Changes**: for a rewrite, what was removed, merged or moved, one line each.
- **Pending**: checks, documentation or decisions left to the user, or "none".

## Limits

- Apply the `Limits` of the skill `writing-skills`.
- Only change the launcher to align its prompt block and what it relays (see Step 6).

## How to operate

Copy this checklist and track your progress:

```text
Subagent progress:
- [ ] Step 1: Read the `Limits`, `Style`, `Body` and `Limits section` of the skill `writing-skills`.
- [ ] Step 2: Resolve the input (see `Input`) and read the current file and the launcher when they exist.
- [ ] Step 3: Write the `description`; if it needs more than 3 sentences, propose splitting the subagent.
- [ ] Step 4: Fill template.md (see `Frontmatter` and `Body`) and delete the parts that do not apply.
- [ ] Step 5: Sort each constraint into `Limits` or its step (see `Limits section` of `writing-skills`).
- [ ] Step 6: Align the launcher: its prompt block matches `Input` and it relays every `Output` field.
- [ ] Step 7: Reread the file against `Style` and `Design`, fix every line that breaks them; keep it under 500 lines.
- [ ] Step 8: For a plugin, run `claude plugin validate plugins/<plugin>` and the tests of the project.
```

## References

### Design

- One subagent, one role: when the description joins unrelated jobs, split it.
- It does not see the conversation: never refer to "the user said" or to earlier context.
  Everything it needs comes in `Input`.
- It cannot ask the user: when a decision is missing, it stops and returns the question and its options.
- Least privilege: `tools` only lists what it uses.
  A read-only subagent gets no `Edit` or `Write`; one that must not delegate gets `disallowedTools: Agent`.
- It is one file and cannot read files next to it: its templates go inside it, under `References`.

### Frontmatter

Write the fields in the order of [template.md](template.md).
Always write `name`, `description`, `model`, `effort` and `color`.
Omit the others when not needed and never leave a field commented out.

- `name`: kebab-case, a role noun (`builder`, `reviewer`), equal to the file name.
- `description`: at most 3 sentences and 400 characters.
  It says what it does, who launches it and its main limit.
- `model`: `inherit` by default (the model of the launcher).
  Use `sonnet` or `opus` only when the task needs a specific model.
- `tools` or `disallowedTools`: only when restricting (see `Design`).
  `tools` with `Bash` but without `Edit` nor `Write` still writes files: add the limit that it never changes them.
- `color`: the display color of the subagent in the task list and transcript.
  Use one of `red`, `blue`, `green`, `yellow`, `purple`, `orange`, `pink` or `cyan`.
  Pick one no other subagent of the same location uses: `rg -n '^color:' <folder of agents>`.
- `skills`: only when the subagent runs a skill, as a YAML list of skill names preloaded into its context.
  The body then says which steps of the skill it runs and which ones the launcher does, without repeating the skill.

### Body

The sections of [template.md](template.md), in its order:

- Required: every section except `Requirements` and `References`, which follow the format of `writing-skills`.
- A missing requirement stops the subagent and goes to `Pending`.
- `Input` fields match one to one the lines of the prompt block the launcher sends (`Ticket:`, `Plan:`, `Notes:`).
  The block itself is written in the launcher, not here.
- `Output` has the fields the launcher relays or acts on.
  The last one is `Pending`: what was not done, and the questions or denials that stopped it.
- A subagent that reports fills a `### Report template` of its own `References`.
