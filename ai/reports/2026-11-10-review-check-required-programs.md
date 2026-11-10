# Review of the current change: check the programs a repository requires

2026-11-10; base `3a8b36b` on `main`, with uncommitted changes in the working tree.
Intent given: implement `ai/plans/check-required-programs.md`, a top-level key `requirements` in `polix.yaml` (a list
of program names) that polix looks up on the PATH.
On the first prompt of a session, again after a start or when the missing set changes, the hook injects a once-only,
never-blocking context asking the AI to install the missing programs, and `polix doctor` warns once per missing one.
All five tasks of the plan were built.

## Verdict

approved with comments

## Findings

- major, bug, `internal/app/workflow/workflow.go:203`: a name with a path separator (`bin/tool`, `./scripts/lint`)
  passes the validation, and `exec.LookPath` resolves it against the working directory of the process instead of the
  PATH, so the hook and `polix doctor` depend on the folder they run from.
  It contradicts the docs, which say a program found only in the current folder counts as missing (`docs/polix.md`
  Requirements, `docs/reference.md` `### requirements`, `examples/polix.yaml` and its copy in the skill `workflow`).
  Evidence: with `requirements: [bin/tool]` and an executable `bin/tool`, `polix config check` says `ok`;
  `polix doctor --offline` from the root gives `ok requirements 1 programs of requirements on the PATH`, and
  `polix doctor --offline --dir <repo>` from another folder gives `warn requirements bin/tool is not on the PATH`.
  Fix: reject in `validator.settings`, with its test, a name holding `/` or `\` (or whose base name is not itself),
  with a message that asks for the program name only.
- minor, bug, `internal/app/hook/answer/logic_session.go:276` and `internal/app/setup/diagnose/logic.go:838`: a
  repeated name is handled differently: the doctor removes repeats, the hook does not, and `polix config check` is
  silent.
  Evidence: with `requirements: [go, does-not-exist, does-not-exist]` the hook injects `...not on the PATH:
  does-not-exist, does-not-exist. ...` with the key `requirements: does-not-exist, does-not-exist`, while
  `polix doctor` gives one `warn`.
  Fix: report a repeated name as a problem in `validator.settings`, or remove repeats from `missing` in the hook
  before building the text and the key, as the doctor does.
- minor, bug, `internal/app/workflow/workflow.go:260`: making `InjectsOnce` true with `requirements` also makes
  `observeFinished` (`internal/app/hook/answer/logic_stop.go:76` and `:88`) write a `finished` line on every
  `subagent_stop`, though the context of the requirements never goes to a subagent: one useless write per subagent.
  Evidence: with `version: 1` and `requirements: [go]`, a `SubagentStop` of `Explore` appends
  `{"kind":"observed",...,"event":"subagent_stop","actor":"Explore","key":"Explore","outcome":"finished"}` to
  `.git/polix/log.jsonl`.
  Fix: keep `InjectsOnce` as it was and write `started` in `session` when `InjectsOnce()` holds or `Requirements` is
  not empty, or with a domain predicate of its own for `start`; adjust `TestInjectsOnceRequirements` and its comment.

## Checks

- `go vet ./...`: ok.
- `go test ./...`: ok in every package, `tests/architecture`, `tests/install` and `tests/setup` included.
- `golangci-lint run ./...`: 0 issues; `interfacebloat` stays at max 19, with the only `//nolint` on the port of
  AnswerHook (20 methods).
- `go build` into the scratchpad: ok.
- `polix config check`: `ok: polix.yaml and 49 rules of ai/rules/`.
- `polix config fmt --check`: `ok: polix.yaml is formatted`.
- `polix rules check --changed`: 34 checks, 0 failed.
- `polix doctor --offline` on this repository: `ok requirements 2 programs of requirements on the PATH`.
- `claude plugin validate plugins/polix`: validation passed.
- End to end in a scratch repository: one `warn` for `does-not-exist` in the doctor; a `UserPromptSubmit` exits 0 and
  injects the context; a second prompt of the session injects nothing; after a `SessionStart` the prompt injects it
  again; `requirements: go` gives the form error at line 2; `[go, ""]` gives `a program name cannot be empty`.

## Held

- GO-2: every new message, doc and comment is in English.
- GO-3: `//nolint:interfacebloat // <reason>` on the line of the port, the linter still enabled.
- GO-4: golangci-lint and the GOARCH tests pass.
- DOC-1: `docs/polix.md` and `examples/polix.yaml` updated, with `docs/reference.md`, README, CHANGELOG and the copy
  of the skill.
- XPLAT-1: tests with the real `exec.LookPath` for `go` without extension in `answer/out_test.go` and
  `diagnose/out_test.go`, checking `.exe` on Windows.
- GOARCH-18: the logic of `session` and the `Decide` of the doctor stay pure.
- GOARCH-20: the lookups go through the port, from the shell (`carry` and `lookRequirements`).
- GOARCH-21 and GOARCH-36: `answer.md`, `diagnose.md`, `load.md` and `format.md` describe the new behaviour and name
  `Missing`, `Look`, `lookRequirements` and `requirementsYAML`.
- GOARCH-30: `Step.Valid` rejects an empty non-nil `Programs`, and `learnMissing` clones the slice.
- The deterministic GOARCH rules pass with `polix rules check --changed`.
- AC1 to AC10 each have a test.

## Pending

- The tests did not run on Windows nor Linux (AC10, `PATHEXT`), only on macOS: left to the CI matrix.
- `just check` was not run by the reviewer, since it replaces the binary of the hooks; its commands ran with a binary
  built in the scratchpad.
- The path of Codex (`polix hook codex`) was not tested end to end; only the shared logic covers it.
