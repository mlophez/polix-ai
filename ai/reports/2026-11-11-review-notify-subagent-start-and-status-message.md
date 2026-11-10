# Review of the current change: notify, subagent_start and the status message of the hooks

2026-11-11; base `b839109` on `main`, with uncommitted changes in the working tree.
Intent given: the five tasks of `ai/plans/notify-subagent-start-and-status-message.md`.
Beyond the plan, the notices of a call with several events are written as `notified` only when no event of it blocks.

## Verdict

approved with comments

## Findings

- minor, rule (GOARCH-27, its check passes), `internal/app/hook/hook_facts.go:75`: `Facts.Deferred` is a setter in all
  but name.
  Evidence: its body is `f.Pending = slices.Clone(entries); return f`, chained right before `Answered`, which already
  clears `Pending` on a block (`internal/app/hook/answer/logic.go:80`); the test `isSetter` only catches a direct `=`.
  Fix: fold it into `Answered`, which keeps the pending lines unless the answer blocks.
- minor, consistency, `README.md:283` and `README.md:293`: "How it works" still describes only exit 2, `additionalContext`
  and exit 0, without the new `systemMessage` for the user.
  Evidence: step 3 reads "It exits with code 2 and the reasons on stderr to block, or with code 0 and
  `additionalContext` to inject guidance."
  Fix: add `systemMessage`, the messages for the user with `notify` among them, to that step and to the diagram.

## Checks

- `go vet ./...` and `go test ./...`: every package ok, `tests/architecture`, `tests/setup` and `tests/install` included.
- `golangci-lint run ./...`: 0 issues; `gofmt -l .` and `git diff --check`: clean.
- `polix config check`, `polix config check examples/polix.yaml` and `polix config fmt --check`: ok.
- `polix rules check --changed`: 34 checks, 0 failed.
- `polix doctor --offline`: exit 0 with only the expected warnings of a development build.
- `examples/polix.yaml` and `plugins/polix/skills/workflow/reference.yaml`: identical.
- `claude plugin validate plugins/polix`: passed.
- End to end with a built binary on a throwaway repository:
  a `notify` on `prompt` printed one `systemMessage` and the second prompt nothing;
  `SubagentStart` gave `hookSpecificOutput` for `claude` and `codex` and an observed `started` of its actor;
  a commit with a notice on `pre_command` and a block on `pre_commit` exited 2, logged the notice as not shown and
  showed it on the next call;
  `polix config check` reported the unreachable entry, the swallowed notice, `block` on `subagent_start` and `inject`
  on `stop` with their lines.

## Held

- Deterministic, through `polix rules check --changed`: GO-4 and GOARCH-1 to 5, 7, 9, 11 to 16, 23 to 29, 32 to 39 and
  41 to 45.
- By reading: GO-2, GO-3, XPLAT-1, XPLAT-2, DOC-1, GOARCH-8, 10, 17 to 22 and 30.

## Pending

- `just check` was not run by the reviewer, since it overwrites the installed binary; its four commands ran with a
  built binary.
- Not verified against a running agent: whether Codex accepts `SubagentStart`, `statusMessage` and `systemMessage` on
  each event, and the manual steps of the plan in a Claude Code session.
