# Review of the limits of size and complexity, the domains without free functions and the load use cases

2026-11-09; base `7f14076`, branch `worktree-valiant-imagining-lagoon`, with the whole change uncommitted.
Intent given: implement `ai/plans/limit-size-and-complexity.md`, tasks 1 to 12 and its section "Adjustments during
the implementation", without changing the behaviour of the binary.
`ai/tasks/architecture-follow-ups.md` was left out, as an unrelated note committed separately.

## Verdict

approved with comments

## Findings

- major, intent, `internal/app/state/load/in.go:1` and `internal/platform/agentconf/agentconf.go:1`: two parts are
  built differently from the plan, and its section "Adjustments during the implementation" records neither.
  The log of `state` has its own load use case, `state/load`, injected into AnswerHook and LogWorkflow and read at
  every call, where the plan kept it in the out files of AnswerHook.
  The settings files of the agents are read and written by a new library, `internal/platform/agentconf`, which the
  out files of the setup use cases wrap, where the plan put them in those out files.
  Both choices hold (`polix log` also reads the log; the settings files are technology shared by four use cases,
  GOARCH-19), but `docs/architecture.md:169` still says the settings live in the out files of the setup use cases.
  Evidence: `ai/plans/limit-size-and-complexity.md:159-160`, `:548-549`, `:611-614`; `cmd/polix/main.go`
  (`loadState := stateload.Command()`); `rg 'agentconf\.' internal/app`.
  Fix: record both decisions and their reasons in the Adjustments of the plan, and say in `docs/architecture.md:169`
  that the out files of the setup use cases wrap `internal/platform/agentconf`.
- minor, consistency, `docs/polix.md:206`: step 2 still says the logic of the hook (`internal/app/hook`) applies the
  protections, which now live in the logic files of AnswerHook.
  Evidence: `git diff docs/polix.md` touches lines 201-203 and 209, not 206.
  Fix: point step 2 at the logic of AnswerHook (`internal/app/hook/answer`).
- minor, tests, `internal/app/setup/diagnose/out.go:45`: DiagnoseSetup encodes the update cache with its own
  `cacheJSON`, and no test checks the bytes it writes, where `Cache.Encode` was checked byte for byte before.
  Evidence: `internal/app/setup/diagnose/out_test.go:71-73` only asserts that `SaveCache` returns no error.
  Fix: assert the bytes written, as `version/update/out_test.go` does, or read them back with
  `versionload.Command()`.

## Checks

- `go vet ./... && go test ./...`: passed, `tests/architecture` and `tests/setup` included.
- `golangci-lint run ./...`, with gocyclo, gocognit and `file-length-limit` on: 0 issues.
- `polix rules check --changed` with a binary of the worktree: 34 checks, 0 failed.
- `polix config check`, `polix config fmt --check` and `polix doctor --offline`: ok, with the expected warnings of a
  dev build.
- `go vet ./...` for windows/amd64 and linux/arm64: ok.
- Old against new binary: `polix graph`, `polix risk` and `polix log` give the same output in this repository; 17
  variants of `polix.yaml` and a missing file decode to the same errors and workflows.
- Manual reading of the families of AnswerHook, its loop guard and injection, and of the strict mode of `rule/load`,
  against the old code: same behaviour.

## Held

- GO-3 and GO-4: the only exclusion is the one of the test files; no `//nolint` on the limits; `TestGO4` passes.
- GOARCH-1 to GOARCH-44: every deterministic check passes; no free function nor YAML or JSON in the domains.
- GOARCH-9 as amended: only test files import the load use cases.
- GOARCH-10 and GOARCH-19: `agentconf` is technology without domain.
- GOARCH-18, GOARCH-20 and GOARCH-22: the logic and the domain stay pure; the shell of AnswerHook only sequences.
- AC8: the engine of AnswerHook uses the name of an event only as data.
- XPLAT-1, DOC-1 and GO-2.

## Pending

- `just check` was not run as written, since it installs polix on the PATH; its checks ran with a binary in the
  scratchpad.
- A real session of Claude Code (AC12) and an old against new run of `polix hook` were not done; the answers rest on
  the unit and adapter tests of AnswerHook and on the comparisons of the builders.
- The steps of the plan that break the code on purpose were not run; the bad testdata of `TestGO4`, `TestGOARCH44`
  and `TestGOARCH32` covers them.
- The PowerShell and Windows CI tests were not run.
