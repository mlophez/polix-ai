# LogWorkflow

Prints the last entries of the state log, `<git-dir>/polix/log.jsonl`: the sanctioned way to read it, since the
commands that name the state folder and write files are denied.

## Trigger

```
polix log [--dir <dir>] [--tail <n>] [--json]
```

- `--dir`: any folder of the repository; defaults to the current one.
- `--tail` (`-n`): how many of the last entries; 20 by default, 0 for all.
- `--json`: print the entries as JSON lines with the known fields of an entry.

## Request

`LogWorkflowRequest`, filled by the command from its flags: `Dir`, `Tail` and `JSON`, from `--dir`, `--tail` and
`--json`.

## Logic

`LogWorkflow(st state.State, tail int) LogWorkflowOutput` picks the entries to show. It takes the state of the
log as the shell read it through the port, and `tail` from the request.

`LogWorkflowOutput.Entries` holds the last `tail` entries of the log in file order, or all of them when `tail` is
0 or less or the log holds no more.

The domain decides the rest:

- `state.Parse`: every line that decodes is an entry, kept as it is; a line that does not decode is left out.
- `state.State.Skipped`: the lines over `state.MaxLine` (4 MiB), skipped as they are read.

## Response

`LogWorkflowResponse`: `Entries`, each entry to show as a `LogWorkflowEntry` with the fields of a line of the log
(`At`, `Kind`, `Agent`, `Session`, `Event`, `Actor`, `Branch`, `Hash`, `Rule`, `Fact`, `Action`, `Line`, `Key`,
`Outcome` and `Reason`; `Rule` and `Fact` only come from the lines an older polix wrote, never from a new one), and
`Skipped`, the lines of the log skipped for being too long.

The command writes to stderr `polix: <n> lines of the log are longer than <max> bytes and were skipped` when
`Skipped` is not 0, then each entry to stdout: with `--json` as a JSON line, else as one line with the time, the
kind, the event, the actor, what happened (the outcome with the verb of a decision, such as `skip run` for a verb
its repeat rules spared, or with the key of an observed line: `finished polix:reviewer`, `passed just test`,
`injected <text, file or rule id>`, or just `blocked` and `started`), the first 8 characters of the hash and the
first line of the reason. A line of a kind polix no longer writes (`fact`, `injected`, `stop_block`) shows its kind
and the fields it shares with the current lines, the rule of an old `injected` line and the name of an old `fact`
line included. It exits with 0.

## Port

`LogWorkflowPort`, declared in `log_workflow_in.go` and implemented by `LogWorkflowAdapter` in
`log_workflow_out.go`:

- `ReadLog`: reads the log through `pkg/jsonl` with the limit `state.MaxLine` and parses it with `state.Parse`;
  without the file, an empty log.

## Errors

The use case fails, printing the error and exiting with 1, when:

- `--dir` is not inside a git repository.
- The log exists and cannot be read.

## Flow

1. `LogWorkflowCommand` turns the flags into a `LogWorkflowRequest` and injects `LogWorkflowAdapter{Repo: repo}` as
   the `LogWorkflowPort`.
2. The shell reads the log through the port.
3. `LogWorkflow` picks the entries to show: `LogWorkflowOutput`.
4. The shell builds the response from the output and the skipped lines of the state.
5. `LogWorkflowCommand` prints the warning of the skipped lines and the entries.
