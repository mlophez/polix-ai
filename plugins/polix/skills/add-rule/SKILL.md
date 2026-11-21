---
name: add-rule
description: >
  Adds rules on how to write the code to the folder of rules of the project, one YAML file per topic.
  It runs only when the user or the workflow of the project asks for it.
  It never changes the code nor the configuration of the tools that enforce the rules.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: false
paths: "**/rules/*.yaml"
argument-hint: "[the rule or the document to migrate, e.g. docs/conventions.md; empty to ask]"
---

# Adding rules

Turn each requested or migrated rule into an item of `rules:` of a file of the folder of rules.
The folder of rules is the one the project names in `AGENTS.md`, `CLAUDE.md` or the session, `ai/rules/` by default.
A rule has the keys of `Rule format`; any other key is kept for people and skills.

## Requirements

- A parser of YAML: the first of `yq`, `python3` with `yaml` or `ruby -ryaml` that runs.
  Without one, skip the parse of Step 7 and report it in `Pending`.

## Input

- **Request**: the rules to add, or the document to migrate them from, from the arguments or the conversation.
  Without either, ask the user.

## Output

- **Rules**: each rule added, with its id and file.
- **Duplicates**: the rules skipped and the existing ids they repeat, or "none".
- **Contradictions**: the pairs of ids left to the user, or "none".
- **Pending**: the deterministic rules left without `check`, the checks that failed and the files not parsed.
  Also the migrated rules a denied edit left in their source, with their lines; or "none".

## Limits

- Never add a rule that repeats an existing one: report the existing id.
- Show a rule that contradicts another of the same scope with both ids, and let the user decide.
- Set `check` only to a file that already exists; leave a deterministic rule without one as pending.
- Never write a `scope` or `check` the user did not confirm.
- Only write the folder of rules and the migrated sources, `AGENTS.md` and `CLAUDE.md` included.
  In a source, remove only the rules Step 4 confirmed; when the edit is denied, as for a protected file, leave it.
- Never configure when the rules are injected or checked: that belongs to the tools of the project.
- Talk to the user in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Rules progress:
- [ ] Step 1: Read every file of the folder of rules, retired.yaml included, to know the topics and the free ids.
- [ ] Step 2: Split the request or the source into one rule per item, each with its file and id (see `Rule format`).
- [ ] Step 3: Propose `scope` and `check` from the code of the project, saying where each proposal comes from.
- [ ] Step 4: Show the rules to write, the lines to remove from each source, the duplicates and the contradictions.
  Confirm once before writing.
- [ ] Step 5: Write the rules, creating the missing files with a `rules:` list ordered by id.
- [ ] Step 6: Remove each migrated rule from its source.
- [ ] Step 7: Parse each file written until it parses, and run each new `check` once (see `Rule format`).
```

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

- Word each rule with its trigger, exact names and the alternative of every prohibition; no vague words.
- One file per topic: `architecture.yaml` (ARCH), `code-style.yaml` (STYLE), `testing.yaml` (TEST).
  Also `security.yaml` (SEC), `design.yaml` (DESIGN), or the files the project already has.
- `retired.yaml` keeps retired rules with their `id`, `description` and `reason`; no rule of it applies.
  Its ids stay taken.
- Globs use `/` and are relative to the root: `*` stays in a segment, `**` crosses segments, `*.md` matches anywhere.
- `check` names any file, relative to the root: a script, a test, any file a command can run.
  Run it as the command of `AGENTS.md` that runs the checks of the rules does, from the root.
  Without one, run it as a program, on Windows with `bash` or `python` by its extension.
  A check that fails goes to `Pending` with its output: the code may not follow the rule yet.
