# Task: run the tests of the rules from a folder configured in polix.yaml

Status: to plan (with the skill `polix:plan`) and implement. Noted on 2026-11-05.

## Why

Today the `check` of a rule of `ai/rules/` can only be the path of a script that polix runs as a program
(`rules.RunChecks`), with the interpreter of Windows hard-coded in `rules.Interpreter`. A rule decided by a test
in another language, such as the Go tests on the AST of the architecture rules GOARCH-* drafted in
`examples/architecture/tests/`, needs a wrapper script per rule only to call `go test -run <test>`.

## Proposal

polix.yaml says where the tests of the rules live, one file per rule, and how to run each kind of file:

```yaml
rules:
  tests: tests/rules                  # one file per rule, named after its id
  run:
    "*_test.go": [go, test, ./tests/rules, -run, "^Test{name}$"]
    "*.sh": [bash, "{file}"]          # only when the project mixes languages
```

```
tests/rules/
  goarch-4_test.go     -> func TestGOARCH4   (GOARCH-4)
  sh-2.sh              -> SH-2
```

- **From rule to file by convention**: the rule `GOARCH-4` is decided by the file `goarch-4.*` of the folder.
- **Placeholders** of the commands: `{file}` (path of the file), `{id}` (`GOARCH-4`) and `{name}` (`GOARCH4`, the id
  without separators, for the names of functions).
- **Deterministic by the file**: a rule is deterministic when it has a file in the folder; the fields
  `deterministic` and `check` of the rules are no longer needed and cannot drift from the tests.
- **One run per rule**: `polix rules check --changed` and `run: { rules: changed }` of the hooks still run only the
  rules whose scope matches the changed files, and a failure belongs to one rule.
- **No interpreter in the code**: the command of each kind of file is in `run`; `sh` and `py` could come as
  defaults, replacing `rules.Interpreter`.

## Open questions

1. Compatibility with the rules that already have `check: <script>`: keep it as a legacy mode, or migrate them.
2. Names of the keys (`rules.tests`, `rules.run`) and of the placeholders.
3. What `polix check` and `polix doctor` report for a file of the folder that no runner matches, a file whose id
   is not a rule, and a rule declared deterministic without a file.
4. Defaults when polix.yaml says nothing: no folder, or `tests/rules`.
5. Whether a run may batch several rules (`-run '^Test(GOARCH4|GOARCH9)$'`) for speed.

## Impact

- `internal/rules`: load, validate and run the checks through the runners; drop `Interpreter` if the defaults
  replace it.
- `internal/workflow`: parse and validate the new keys of polix.yaml.
- `internal/engine`: `run: { rules: changed }` through the runners.
- `internal/cli`: `polix check`, `polix rules check`, `polix doctor` (interpreters, files without runner).
- `docs/polix.md`, `examples/polix.yaml`, the skills `add-rule` and `doctor` and the agent `rules-author` of the
  plugin.
- `examples/architecture/tests/`: move the tests to the convention (`goarch-4_test.go`, `TestGOARCH4`).
