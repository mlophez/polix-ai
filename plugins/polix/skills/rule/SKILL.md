---
name: rule
description: >
  Audits the rules of the folder of rules of the project and asks about each finding alone.
  Then it adds or removes the rules the user asks for.
  It runs only when the user or the workflow of the project asks for it.
  It never changes the code nor the configuration of the tools that enforce the rules.
user-invocable: true
disable-model-invocation: false
paths: "**/rules/*.yaml"
argument-hint: "[the rules to add or remove, or the document to migrate, e.g. docs/conventions.md; empty to only audit]"
---

# Auditing and writing rules

Audit every rule of the folder of rules, settle each finding with the user, then add or remove rules.
The folder of rules is the one the project names in `AGENTS.md`, `CLAUDE.md` or the session, `ai/rules/` by default.
A rule has the keys of `Rule format`; any other key is kept for people and skills.

## Requirements

- A parser of YAML: the first of `yq`, `python3` with `yaml` or `ruby -ryaml` that runs.
  Without one, skip the parse of Step 7 and report it in `Pending`.
- The template [question.md](../grill-me/question.md) of the skill `grill-me`, which stays invocable.
  Without it, stop and tell the user.

## Input

- **Request**: the rules to add or remove, or the document to migrate them from, from the arguments or the conversation.
  Without one, audit only and then ask what to add or remove.

## Output

- **Audit**: the findings by rule id and check, each with the answer of the user, or "none".
- **Rules**: each rule added, removed, split or changed, with its id and file.
- **Pending**: the rules with `kind: unknown`, the deterministic rules left without `check`, the checks that failed
  with their output and the files not parsed; or "none".

## Limits

- Write no rule before the user answers its finding; the audit only reads.
- Ask about one finding per message with the template of `grill-me`, and wait for the answer.
- Never add a rule that repeats or overlaps an existing one: report the existing id.
- Never write a `scope`, a `subject` or a `check` the user did not confirm.
- Set `check` only to a file that already exists; leave a deterministic rule without one as pending.
- Only write the folder of rules and the migrated sources, `AGENTS.md` and `CLAUDE.md` included.
  In a source, remove only the rules the user confirmed; when the edit is denied, as for a protected file, leave it.
- Never configure when the rules are injected or checked: that belongs to the tools of the project.
- Talk to the user in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Rules progress:
- [ ] Step 1: Read every file of the folder of rules, to know the subjects, the kinds and the ids.
- [ ] Step 2: Audit every rule with the nine checks (see `Audit`), listing the findings and writing nothing.
- [ ] Step 3: Ask about each finding alone (see `Question per finding`) and apply each answer to the rules.
- [ ] Step 4: Ask what to add or remove, from the request or the user.
- [ ] Step 5: Split the additions into atomic rules, each with its file, id, kind and subject (see `Rule format`).
- [ ] Step 6: Propose `scope` and `check` from the code, say where each comes from, and confirm once before writing.
- [ ] Step 7: Write the rules, delete retired.yaml when the user agreed, then parse each file and run each new `check`.
```

### Step 3: Ask and apply

Apply the answer of each finding before the next question, so later findings see the current rules.
When a rule is split, the main part keeps the id and the check, the other parts take new ids, and every description
that names the id follows.
When a rule is removed, delete it: its id becomes free, and no file keeps retired rules.
When `retired.yaml` exists, no tool reads it any more: offer to delete it.

### Step 5: New ids

A new rule takes the number after the highest of its file, with the prefix of the file.
Reuse a gap only when the user asks.
Create a missing file with a `rules:` list ordered by id.

### Step 7: Check what is written

Parse each file written until it parses.
Run each new or changed `check` as the command of `AGENTS.md` that runs the checks of the rules does, from the root.
A check that fails goes to `Pending` with its output: the code may not follow the rule yet.

## References

### Rule format

```yaml
rules:
  - id: ARCH-3                       # prefix of the file and the next number after the highest
    kind: dependency                 # one of the kinds below
    subject: domain                  # free text: the part of the code it applies to
    description: >                   # one sentence: the trigger and what to do; a second one for the prohibition
      A domain imports only domains.
      Never import an adapter from a domain; take the value as a parameter.
    scope: ["internal/app/*/*.go"]   # non-empty globs of the files it applies to; the whole project is ["**"]
    deterministic: true              # true when a test decides it, false when only the review does
    check: tests/rules/arch-3.sh     # required when deterministic, forbidden when not; exit 0 when the code follows it
```

- Every field but `check` is required, and `check` exactly when `deterministic` is true.
- `kind` is case sensitive and checked against the list; `subject` is never checked, it only has to be non-empty.
- Word each rule with its trigger, exact names and the alternative of every prohibition; no vague words.
- One file per topic: `architecture.yaml` (ARCH), `code-style.yaml` (STYLE), `testing.yaml` (TEST).
  Also `security.yaml` (SEC), `design.yaml` (DESIGN), or the files the project already has.
- Globs use `/` and are relative to the root: `*` stays in a segment, `**` crosses segments, `*.md` matches anywhere.
- `check` names any file, relative to the root: a script, a test, any file a command can run.
  Without an extension known to the system, run it as a program, on Windows with `bash` or `python` by its extension.

### Kinds

- `layout`: where files and folders live. Example: each use case has its own folder with its files.
- `dependency`: what may import what. Example: a domain imports only domains.
- `declaration`: what a file or type must declare or hold. Example: each use case declares its document.
- `naming`: how things are called. Example: the files of a use case are named after its verb.
- `purity`: what a part must never touch. Example: the logic does no input or output.
- `data`: the shape and the limits of the data. Example: a rule has a non-empty scope.
- `flow`: the order or the way things run. Example: the shell loads, the logic decides, the output prints.
- `metric`: a measurable limit. Example: a function has at most 50 lines.
- `testing`: how the code is tested. Example: each rule of the architecture has one test.
- `docs`: what is documented and where. Example: a change of a command updates its document.
- `convention`: a style the code follows that no other kind names. Example: comments say why, never what.
- `unknown`: the kind is not decided or none fits; a valid kind, always pending in the audit.

### Audit

Run the nine checks on every rule and keep one finding per problem, with the ids it involves:

1. **Fields**: all present and valid; report every `kind: unknown` as pending.
2. **Atomicity**: one subject and one kind; when a rule has two of either, propose to split it.
3. **Duplicates and overlaps**: rules that say the same, even in part, across files.
4. **Contradictions**: rules of the same scope that cannot both hold.
5. **Wording**: the trigger, exact names, the alternative of each prohibition, no vague words.
   A permission that forbids nothing is a finding.
6. **Deterministic or review**: `deterministic` matches what decides the rule, a test or the review.
   A rule whose part a test decides and whose part is left to review is split.
7. **Real scope**: its globs match existing files and cover its subject.
8. **Real check**: its file exists and runs; a failing one is pending with its output.
9. **References**: the ids its description names exist.

### Question per finding

Ask each finding with the template of [question.md](../grill-me/question.md), one message, two to four options.
Put the recommended option first, name the rule ids and the check in the question, and give the fix of each option.
Number the questions in order with the total of findings.
An unanswered question is asked again; never apply a default.
