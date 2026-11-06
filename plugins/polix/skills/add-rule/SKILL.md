---
name: add-rule
description: >
  Adds rules on how the code of the project must be written to the folder of rules (ai/rules/ by default), the
  rules polix injects and checks.
  Use it when the user asks to add a rule or a coding convention, or to migrate rules from a document into them.
  It never changes the code nor polix.yaml.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: false
argument-hint: "[the rule or the document to migrate, e.g. docs/conventions.md; empty to ask]"
---

# Adding rules

Turn each requested or migrated rule into an item of `rules:` of a file of the folder of rules.
The folder of rules is `rules.path` of `polix.yaml`, `ai/rules/` without it; polix names it at session start.
Polix reads `id`, `description`, `scope`, `deterministic` and `check`; any other key is kept for people and skills.

## Requirements

- `polix` on the `PATH`.
  Without it, stop and point to the skill `doctor` of polix.

## Input

- **Request**: the rules to add, or the document to migrate them from, from the arguments or the conversation.

## Rules

- Word each rule with its trigger, exact names and the alternative of every prohibition; no vague words.
- Never add a rule that repeats an existing one: report the existing id.
- Show a rule that contradicts another of the same scope with both ids, and let the user decide.
- Set `check` only to a command that already exists; leave a deterministic rule without one as pending.
- Never write a `scope` or `check` the user did not confirm.
- Only write the folder of rules and the migrated sources.
- When the request is about when rules are injected or checked, point to the skill `workflow` of polix.
- Talk to the user in the language of the session.

## References

### Rule format

```yaml
rules:
  - id: ARCH-3                       # prefix of the file and the next free number, never reused
    description: >                   # one sentence: the trigger and what to do; a second one for the prohibition
      HTTP clients live in internal/clients/, one package per service.
      Never call an external API from a handler.
    scope: ["internal/**/*.go"]      # globs of the files it applies to; without scope, the whole project
    deterministic: true              # true when a command can tell whether the code follows it
    check: tests/rules/arch-3.sh     # optional: the file that checks it, exit 0 when the code follows it
```

- One file per topic: `architecture.yaml` (ARCH), `code-style.yaml` (STYLE), `testing.yaml` (TEST).
  Also `security.yaml` (SEC), `design.yaml` (DESIGN), or the files the project already has.
- `retired.yaml` keeps retired rules with their `id`, `description` and `reason`; polix ignores it.
  Its ids stay taken.
- Globs use `/` and are relative to the root: `*` stays in a segment, `**` crosses segments, `*.md` matches anywhere.
- `check` names any file, relative to the root: a script, a Go test, any file a command can run.
  The first runner of `rules.run` in `polix.yaml` whose glob matches it (its base name, or its path when the glob has a `/`) runs it, from the root and
  without a shell; with no runner it runs as a program, on Windows with `bash` or `python` by its extension.

## How to operate

Copy this checklist and track your progress:

```text
Rules progress:
- [ ] Step 1: Read every file of the folder of rules, retired.yaml included, to know the topics and the free ids.
- [ ] Step 2: Split the request or the source into one rule per item and give each its file and id (see `Rule format`).
- [ ] Step 3: Propose `scope` and `check` from the code of the project, saying where each proposal comes from.
- [ ] Step 4: Show the rules to write, the duplicates and the contradictions, and confirm once before writing.
- [ ] Step 5: Write the rules, creating the missing files with a `rules:` list ordered by id.
- [ ] Step 6: Remove each migrated rule from its source.
- [ ] Step 7: Run `polix check` until it reports no problem, and `polix rules check` when a rule has a `check`.
```

## Output format

- **Rules**: each rule added, with its id and file.
- **Duplicates**: the rules skipped and the existing ids they repeat, or "none".
- **Contradictions**: the pairs of ids left to the user, or "none".
- **Pending**: the deterministic rules left without `check`, or "none".
