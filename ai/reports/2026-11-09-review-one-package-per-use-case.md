# Review of the reorganization of internal/ by depth, one package per use case

Date: 2026-11-09.
Base: f24e4c0 (main), branch `one-package-per-use-case`, with every change uncommitted in the working tree.
Intent, as given: reorganize `internal/` by depth as `ai/plans/one-package-per-use-case.md` describes.
`pkg/` moves to `internal/platform/<lib>/`, the domains to `internal/app/<noun>/`, and the 13 use cases to
`internal/app/<noun>/<verb>/` with fixed names (`Command`, `Request`, `Response`, `Port`, `Adapter`, `Decide`,
`Output`, `run`).
The setup end-to-end tests move to `tests/setup/`.
The GOARCH rules and their AST tests are rewritten by depth: GOARCH-31 and GOARCH-40 retired, GOARCH-9 the direction
of the imports, and the fixed import lists moved to depguard (GOARCH-41, 42 and 43) with tests that run it.
The behaviour of the binary does not change.

## Verdict

Approved with comments.

## Findings

- major, intent and consistency, `polix.yaml:4-5` and `polix.yaml:12-13`: two changes to the workflow of the
  repository are unrelated to the reorganization, and one leaves its comment wrong.
  `max_stop_retries` drops from 3 to 1 while the comment above still says the agent may stop "after 3 blocks", and
  `guidance: edit: false` turns off the edit guidance polix injects; the plan only changes the three `risk.weights`.
  Evidence: `git diff -- polix.yaml` shows `-max_stop_retries: 3` and `+max_stop_retries: 1` under the unchanged
  comment, and `+guidance:` with `+  edit: false`.
  Fix: revert both before committing, or commit them separately on purpose with the comment updated.
- major, security, `internal/platform/testenv/testenv.go:130-136` (`buildFakeCLI`): the fake CLI the tests run is
  cached at a predictable path shared by every user and reused without checking who owns it.
  Evidence: the binary lives at `os.TempDir()/polix-fakecli-<hash>-<goos>-<goarch>/fakecli`, `MkdirAll` accepts an
  existing folder and `os.Stat(bin)` returns an existing file as is; on Linux another user of `/tmp` can plant it,
  and the tests put it on the PATH as `claude` and `codex`; nothing removes old binaries.
  The old `TestMain` built into an `os.MkdirTemp` folder with mode 0700 and removed it.
  Fix: build once per test binary into an `os.MkdirTemp` folder, or cache under `os.UserCacheDir()`; a shared cache
  must check that its folder belongs to the current user with mode 0700.

## Checks

- `go build ./... && go vet ./... && go test ./...`: exit 0, 38 packages ok, `tests/architecture`, `tests/install`
  and `tests/setup` included.
- `golangci-lint run ./...` (v2.15.0): 0 issues.
- A binary of the change built into a scratch folder: `polix config check` ok with 46 rules, `polix config fmt
  --check` ok, `polix rules check` with 32 checks and 0 failed, `polix doctor --offline` with no failure and one warn
  on the plugin version of a dev build.
- `--help` of polix and of every command and subcommand, against a build of `git archive main`: identical output
  and exit codes.
- A build with `-X github.com/mlophez/polix-ai/internal/platform/build.Version=9.9.9` prints `9.9.9`.
- `pkg/` no longer exists, and no folder of `internal/` is deeper than `<noun>/<verb>/<helper>`, `testdata` aside.
- The moved domain and library files differ from `main` only in import paths, comments and the exports of Task 1.
- `ParseHooks` drops handlers without a command and empty groups, which never hold "polix hook claude", so
  `DiagnoseSetup` gives the same findings.
- The string literals of every use case file are identical before and after.
- The moved end-to-end setup tests keep their cases once helper names are normalized.
- No `//nolint` was added or changed; depguard v2.2.1 normalizes file names with `filepath.ToSlash`, so its globs
  match on Windows too.

## Held

- GOARCH-1 to GOARCH-43: every deterministic rule is decided by its test, whose subtest `module` finds nothing in
  the repository and whose subtest `detects` finds its faults in `testdata/bad` and `testdata/depguard`.
- GOARCH-31 and GOARCH-40 are in `ai/rules/retired.yaml` with a reason, their tests are gone, and
  `TestRulesChecks` passes.
- GOARCH-17, GOARCH-19, GOARCH-20, GOARCH-22 and GOARCH-30: folder names, and `testenv` in `internal/platform/` as
  support for tests.
- GO-2: the docs are in English.
- GO-3: no new `//nolint` and no exclusion in `.golangci.yml`.
- XPLAT-1: `testenv` uses `filepath`, the `.exe` suffix and the fallback without symlinks; `scripts/dist.sh` and
  `tests/install` use the new path of `-X`.
- DOC-1 and GOARCH-21: `docs/polix.md`, `docs/architecture.md`, `README.md` and `CONTRIBUTING.md` name the new
  paths, with no `pkg/`, old file suffix or old `<Verb><Noun>` symbol left.

## Pending

- CI has not run: `TestGOARCH41`, 42 and 43 need golangci-lint on the PATH of the steps Test and Rules of
  `.github/workflows/ci.yml`, which rely on `golangci/golangci-lint-action@v8` leaving it there.
- Windows and Linux were not run locally (the fallback of `testenv` without symlinks, the paths of depguard).
- The manual check of a Claude Code session after `polix install` in a scratch repository needs an interactive agent.
- `AGENTS.md`, a protected file, still names packages that no longer exist; the plan leaves it to a person.
- `just check` was not run as such, since its `install` step replaces the binary the hooks run; its four polix steps
  were run with a binary in a scratch folder.
