# Polish the comment of format_on_edit in the polix.yaml of the demo and its copy in the README

## Overview

The example workflow file of the demo, which the README also shows in full, has one comment that reads like a label
("Format on edit") while every other comment of the file is a full sentence that says what the entry does.
This change rewrites that comment as a sentence in the same voice, in the file of the demo and in its copy in the
README, so both stay identical.
Nothing else changes: the behaviour of the demo is the same, so the recorded GIF stays valid and is not recorded again.

## Context

`docs/assets/demo/polix.yaml` is the workflow of the demo, and `README.md` shows it verbatim in its first yaml block.
Its comment above `format_on_edit` is the only one that is not a sentence, unlike the comments of `protected_files`,
`forbidden_commands`, `protected_branches`, `run_on_stop` and `pre_commit`.
The outcome is a consistent file whose README copy still matches it byte for byte.

## Decisions

- The change is limited to the comment above `format_on_edit`: a trial change with no effect on the behaviour.
- The new comment reads "Each Go file an agent edits is formatted.", in the voice of the other comments of the file.
- The GIF `docs/assets/demo.gif` is not recorded again: the comment does not change the session the tape records.
- The plan file follows the naming `YYYY-MM-DD-<slug>.md` that the `pre_edit` entry of `polix.yaml` enforces on
  `ai/plans/`, which wins over a name without a date.

## Approach

Only YAML comments change, so the contract of the file (its keys and values) and the parsed workflow stay the same.
The file of the demo is the source and the first yaml block of `README.md` is its copy: both are edited the same way
so that the copy keeps matching the file.
There is no error handling to add; `polix config check` on the folder of the demo confirms the file still parses.

### Discarded alternatives

- Add `golangci-lint run ./...` to `run_on_stop`: it changes the demo and forces a new recording of the GIF.
- Remove the forbidden `*rm -rf /*` entry: it shrinks what the README shows of `forbidden_commands`.

## Acceptance criteria

- AC1: The comment above `format_on_edit` in `docs/assets/demo/polix.yaml` is "Each Go file an agent edits is
  formatted." and no other line of the file changes.
- AC2: The first yaml block of `README.md` is identical to `docs/assets/demo/polix.yaml`.
- AC3: `polix config check -C docs/assets/demo` reports no problem.

## Out of scope

- Recording the GIF again with `just demo`: the behaviour of the demo does not change.
- The `polix.yaml` of the root of the repository and its uncommitted changes: they are not the file of the demo.
- `examples/polix.yaml`: it is a different reference file, not shown as the demo.

## Tasks

### Task 1: Rewrite the comment of format_on_edit in the demo and its copy in the README

- Covers: AC1, AC2, AC3.
- Depends on: none.
- Files: `docs/assets/demo/polix.yaml` (modify), `README.md` (first yaml block, modify).
- Rules: GO-2.
- Reuse: none.
- Changes: in `docs/assets/demo/polix.yaml`, the comment "Format on edit" above `format_on_edit` under `guardrails:`
  becomes "Each Go file an agent edits is formatted.", with the same indentation.
  In `README.md`, the same comment of the first yaml block, the copy of that file, changes the same way.
  No key, value, blank line or other comment changes in either file.
- Tests: none; the file has no Go test, and the checks of `Verification` cover the parse and the copy.
- Verification: `polix config check -C docs/assets/demo`, expected: "ok" and exit code 0.

## Risks

- The README copy drifts from the file: compare the first yaml block of `README.md` with the file before committing.

## Verification

- `polix config check -C docs/assets/demo`: prints "ok" and exits with 0.
- `git diff -- docs/assets/demo/polix.yaml README.md`: one changed comment line in each file and nothing else.
- Open `README.md` and check that its first yaml block matches `docs/assets/demo/polix.yaml` line by line.
