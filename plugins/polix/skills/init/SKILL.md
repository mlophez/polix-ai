---
name: init
description: >
  Writes the general context of the project in AGENTS.md, imported by CLAUDE.md, and the README.md from its templates.
  It explores the project, interviews the user on what the code does not answer and writes the three files.
  It runs only when the user or the workflow of polix asks for it.
effort: high
model: inherit
user-invocable: true
disable-model-invocation: true
---

# Initializing the project context

Write `AGENTS.md` from [context.md](context.md) with the project facts and `CLAUDE.md` with the line `@AGENTS.md`.
Write `README.md` from [readme.md](readme.md) for human readers.

## Requirements

- The templates next to this file: [context.md](context.md) and [readme.md](readme.md).
  Without them, stop and tell the user.

## Input

- **Project**: the current `AGENTS.md` when present, `README.md`, the folder tree and `Justfile`/`Makefile`.
  Also the manifests (`package.json`, `pyproject.toml`, `go.mod`, `pom.xml`...) and test configurations.
  Also the CI pipelines (`.github/workflows/`, `.gitlab-ci.yml`, `bitbucket-pipelines.yml`...).
  Also `polix.yaml` and the folders of `rules.path`, `plans.path` and `reports.path` when present (`ai/rules/`,
  `ai/plans/` and `ai/reports/` without them).

## Rules

- Follow [context.md](context.md) and [readme.md](readme.md): keep their headings and fixed text.
- Replace every `<...>` with what it describes, and every `<!-- ... -->` guidance comment with its content,
  following the conditions it states.
- Keep the first comment of [context.md](context.md) as is in `AGENTS.md`: it says only this skill or a human edits it.
- Keep the section `## polix` of the current `AGENTS.md` verbatim, as the last section: it is the fragment that
  tells the agent how to install polix, and the only section outside [context.md](context.md).
- Write facts in English that describe the project as it is; never rules on how to write its code.
- Be concise and brief, never verbose: cut every word that adds no fact.
- Write every list item of `AGENTS.md` and `README.md` as a short summary on one line of at most 120 characters.
  Never wrap it; the detail belongs in `docs/`.
- Fill `README.md` from the facts of `AGENTS.md`, keeping the valid content of the current `README.md` as the proposal.
  Ask only for what they do not cover: requirements, setup steps and usage examples.
- A section without content holds `TBD`; one that does not apply to the project holds `Not applicable.`
- Never write an inferred value the user did not confirm.
- Only write `AGENTS.md`, `CLAUDE.md` and `README.md`.
- Interview in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Init progress:
- [ ] Step 1: Explore the project and propose the content of each section (see Step 1).
- [ ] Step 2: Interview the user (see Step 2).
- [ ] Step 3: Write `AGENTS.md`, `CLAUDE.md` and `README.md` (see `Rules`).
```

### Step 1: Explore the project and propose the content of each section

Read the project read-only (see `Input`).
Propose the content of each section of [context.md](context.md) and [readme.md](readme.md).
State the origin of each proposal (e.g. "inferred from pyproject.toml") so the user knows it needs validation.

### Step 2: Interview the user

- Show the proposals and ask for what the project does not answer, never one question at a time.
  Use `AskUserQuestion` for closed questions, at most 4 per call, and grouped free text for open ones.
- Apply the corrections of the user.
- Confirm once before writing, listing the existing files that will be overwritten.

## Output format

- **Files**: `AGENTS.md` with its number of lines, `CLAUDE.md` and `README.md`.
- **Pending**: the sections of each file left as `TBD`, or "none".
