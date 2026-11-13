---
name: review
description: >
  Reviews a change without a plan (files, a range or the current change) with polix:reviewer in change mode.
  It relays the verdict on intent, bugs, rules, tests and docs, saves the report in the reports folder, fixes nothing.
  It runs only when the user or the workflow of polix asks for it.
effort: low
model: sonnet
user-invocable: true
disable-model-invocation: true
argument-hint: "[range or files, e.g. HEAD~3..HEAD or internal/app/setup; empty for the current change]"
---

# Reviewing

Review the change with one `polix:reviewer` subagent, relay its report and save it in the folder of reports.
The folder of reports is `reports.path` of `polix.yaml`, `ai/reports/` without it.
You write the intent, which only this session knows; the subagent judges the code.

## Requirements

- The subagent `polix:reviewer`.
  Without it, as in Codex, read `../../agents/reviewer.md` next to this skill and follow it yourself in this session.
- The template of the report, `template.md` next to this skill.
  Without it, stop and tell the user.

## Input

- **Change**: the range or files from the arguments.
  Without them, the current change: the branch against its base plus what is staged, not staged and untracked.
- **Intent**: what the change must achieve, from the conversation: the request or the ticket.
  Without any, send "none" and the subagent infers it.
  A change built from a plan is reviewed by the skill build, against the plan.

## Output

- **Report**: the path of the report file.
- **Verdict**: the verdict of the subagent and the intent it reviewed, noting when it was inferred.
- **Findings**: the findings, by severity, with `file:line`, what is wrong and the direction of the fix.
- **Checks**: the checks run and their result.
- **Pending**: what the subagent could not review, and the question of which findings to fix.

## Rules

- Never edit a file of the project: the report file is the only file you write.
- Relay the findings of the subagent as they are: add none of your own and drop none.
- Talk to the user in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Review progress:
- [ ] Step 1: Resolve the change (see `Input`); when `git status` and the diff show nothing, tell the user and stop.
- [ ] Step 2: Write the intent from the conversation (see `Input`).
- [ ] Step 3: Launch `polix:reviewer` with the prompt (see `Prompt`) and wait for its report.
- [ ] Step 4: Relay the report (see `Output`).
- [ ] Step 5: Write the report to its file from `template.md` (see `Report file`).
- [ ] Step 6: Stop and return control to the user.
```

## References

### Prompt

Send the subagent this block, in the language of the session:

```text
Change: <range or files, or "the current change">
Intent: <one to three sentences, or "none">
```

### Report file

Path: a file of the folder of reports, `<reports>`.
Name it as the project names its reports: the convention of `AGENTS.md`, of `polix.yaml` or of the reports there.
Without one, name it `<reports>/<date>-review-<name>.md`, where `<date>` is today as `YYYY-MM-DD`.
`<name>` is the current branch in kebab-case.
On the base branch, `<name>` is short kebab-case English from the intent of the change.
When the file exists, add a numeric suffix (`-2`) instead of overwriting it.

- Fill `template.md`: its sections in its order, every `<...>` replaced, from the report of the subagent.
- Write it in the language of the docs of the repository, English when there are none.
- One sentence per line, each line at most 120 characters, with no tables.
