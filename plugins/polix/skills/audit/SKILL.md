---
name: audit
description: >
  Audits the repository, or part of it, for bugs, flaws and inconsistencies with polix:auditor subagents in parallel.
  It merges their reports into one ranked by severity, saves it in the folder of reports and fixes nothing.
  It runs only when the user or the workflow of polix asks for it.
effort: medium
model: sonnet
user-invocable: true
disable-model-invocation: true
argument-hint: "[paths and an optional focus, e.g. internal/app/setup security; empty for the whole repository]"
---

# Auditing

Split the scope into areas, audit each with its own `polix:auditor` subagent in parallel and merge their reports.
Save the merged report in the folder of reports: `reports.path` of `polix.yaml`, `ai/reports/` without it.

## Requirements

- The subagent `polix:auditor`.
  Without it, as in Codex, read `../../agents/auditor.md` next to this skill and follow it yourself for each area.
- The template of the report, `template.md` next to this skill.
  Without it, stop and tell the user.

## Input

- **Scope**: the paths or areas from the arguments.
  Without them, the whole repository.
- **Focus**: the kinds of finding from the arguments: bug, security, robustness, inconsistency, dead or stale, tests.
  Without them, all.

## Output

- **Report**: the path of the report file.
- **Summary**: the scope, the areas and their auditors, the findings per severity and the three that matter most.
- **Findings**: each as severity, category, confidence, `file:line`, what is wrong, the evidence and the fix direction.
- **Checks**: the checks run and their result.
- **Coverage**: what was read whole, and what was only skimmed or skipped.
- **Pending**: what could not be audited and why, and the question of which findings to fix.

## Rules

- Never edit a file of the project: the report file is the only file you write.
- Launch every auditor in a single message, so they run in parallel.
- Launch at most 6 auditors.
- Give each auditor an area no other auditor has.
- Let only the first auditor run the checks of the project; the others get `Checks: skip`.
- Add no findings of your own; drop only duplicates.
- Talk to the user in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Audit progress:
- [ ] Step 1: Resolve the scope and the focus (see `Input`).
- [ ] Step 2: Split the scope into areas (see `Areas`) and show them to the user in one line each.
- [ ] Step 3: Launch one `polix:auditor` per area in a single message (see `Prompt`) and wait for every report.
- [ ] Step 4: Relaunch once each auditor that failed or returned no report; report it in `Pending` if it fails again.
- [ ] Step 5: Merge the reports (see `Merge`).
- [ ] Step 6: Write the merged report to its file from `template.md` (see `Report file`).
- [ ] Step 7: Relay the merged report and the path of its file (see `Output`).
- [ ] Step 8: Stop and return control to the user.
```

## References

### Areas

- List the files with `git ls-files` and leave out vendored, generated, lock and binary files.
- Split the code by its top-level units: packages, modules, services or apps.
- Group small units so every area has a similar number of lines (`wc -l`).
- Add one area for the docs: README, docs/, examples and config schemas, with the focus `inconsistency`.
- When the scope is under about 2000 lines or a single unit, audit it with one auditor.

### Prompt

Send each auditor this block, in the language of the session:

```text
Scope: <the paths of its area>
Focus: <the focus, or "all">
Checks: <run for the first auditor, skip for the others>
```

### Merge

- Merge findings with the same root cause, keeping every `file:line` and the highest severity.
- Sort by severity, then by confidence (`confirmed` before `likely`).
- Take the checks from the auditor that ran them.
- Join the coverage and the pending of every auditor.

### Report file

Path: a file of the folder of reports, `<reports>`.
Name it as the project names its reports: the convention of `AGENTS.md`, of `polix.yaml` or of the reports there.
Without one, name it `<reports>/<date>-audit-<name>.md`, where `<date>` is today as `YYYY-MM-DD`.
`<name>` is short kebab-case English from the scope and the focus (`repository` for the whole one).
When the file exists, add a numeric suffix (`-2`) instead of overwriting it.

- Fill `template.md`: its sections in its order, every `<...>` replaced.
- Write it in the language of the docs of the repository, English when there are none.
- One sentence per line, each line at most 120 characters, with no tables.
