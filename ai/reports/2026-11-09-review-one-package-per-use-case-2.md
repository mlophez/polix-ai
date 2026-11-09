# Review of the reorganization of internal/ by depth, after the fixes of the first review

Date: 2026-11-09.
Base: f24e4c0 (main), branch `one-package-per-use-case`, with the change staged and only two lines of `polix.yaml`
left out for a separate commit.
Intent, as given: the same as `ai/reports/2026-11-09-review-one-package-per-use-case.md`, plus the check of its two
findings.
`internal/platform/testenv` builds the fake CLI in a private folder of the cache of the user, and the staged
`polix.yaml` keeps `max_stop_retries: 3` and has no `guidance` block.

## Verdict

Approved.

## Findings

None.

## Checks

- The staged `polix.yaml` only moves the three `risk.weights` to the new paths and puts `actor: session` after
  `changed:` in two rules, a formatting fix the binary of `main` already asked for.
- With the index copy of `polix.yaml`, `ai/rules/` and the architecture tests in a scratch repository,
  `polix config check` and `polix config fmt --check` pass.
- `buildFakeCLI` of `internal/platform/testenv/testenv.go` builds into
  `<UserCacheDir>/polix-testenv/fakecli-<hash>-<goos>-<goarch>/`, created with mode 0700 and narrowed by `Chmod`,
  which fails on a folder of another user; the build is moved into place with one atomic `Rename`.
- After `go test -count=1 ./internal/platform/testenv/ ./internal/app/setup/... ./tests/setup/`, both folders of the
  cache have mode `drwx------` and belong to the user; the 8 packages pass.
- `go build ./cmd/polix`, `go vet ./...` and `go test ./...`: exit 0, `tests/architecture`, `tests/install` and
  `tests/setup` included.
- `golangci-lint run ./...`: 0 issues.
- With the binary of the change, `polix config check` (46 rules), `polix config fmt --check` and
  `polix rules check --changed` (32 checks, 0 failed) pass; `polix doctor --offline` has no failure.
- `--help` of the root command and of 16 commands and subcommands against a binary of `main`: identical output and
  exit codes.
- `git status --porcelain`: nothing untracked, and only `polix.yaml` not staged.

## Held

- The security finding of the first review is fixed: the binary the tests put on the PATH lives in a folder only the
  user can use.
- The finding on `polix.yaml` is handled: the staged copy matches its comment and has no `guidance` block.
- GOARCH-1 to GOARCH-43: the deterministic checks pass.
- GOARCH-43: `testenv` is imported only from test files.
- XPLAT-1: `testenv` builds its paths with `filepath` and the `.exe` suffix; on Windows the cache of the user is
  `%LocalAppData%`.
- GO-3: no new `//nolint`.

## Pending

- CI has not run; on Linux and Windows `TestGOARCH41`, 42 and 43 need golangci-lint on the PATH.
- The fallback of `testenv` without symlinks on Windows was not run.
- When `os.UserCacheDir()` fails (no `HOME` nor `XDG_CACHE_HOME`), `Machine` now fails the test instead of using
  the system temp; this does not happen on the runners of CI nor on this machine.
- `just check` was not run as such, since its `install` step replaces the binary the hooks run.
- `AGENTS.md`, a protected file, still names packages that no longer exist; the plan leaves it to a person.
- The manual check of a Claude Code session after `polix install` needs an interactive agent.
