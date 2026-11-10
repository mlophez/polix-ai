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

`Request`, filled by the command from its flags: `Dir`, `Tail` and `JSON`, from `--dir`, `--tail` and `--json`.

## Logic

`Decide(st state.State, tail int) Output` picks the entries to show. It takes the state of the log as the shell
read it through the port, and `tail` from the request.

`Output.Entries` holds the last `tail` entries of the log in file order, or all of them when `tail` is 0 or less
or the log holds no more.

The loader of the log and the domain decide the rest:

- `LoadState` (`internal/app/state/load`): every line that decodes is an entry, kept as it is; a line that does not
  decode is left out.
- `state.State.Skipped`: the lines over `state.MaxLine` (4 MiB), skipped as they are read.

## Response

`Response`: `Entries`, each entry to show as an `Entry` with the fields of a line of the log (`At`, `Kind`, `Agent`,
`Session`, `Event`, `Actor`, `Branch`, `Hash`, `Head`, `Rule`, `Fact`, `Action`, `Line`, `Key`, `Outcome` and
`Reason`; `Head`, the HEAD commit, only on the observed lines finished with a hash, passed and blocked that know it;
`Rule` and `Fact` only come from the lines an older polix wrote, never from a new one), and `Skipped`, the lines of
the log skipped for being too long.

The command writes to stderr `polix: <n> lines of the log are longer than <max> bytes and were skipped` when
`Skipped` is not 0, then each entry to stdout: with `--json` as a JSON line, else as one line with the time, the
kind, the event, the actor, what happened (the outcome with the verb of a decision, such as `skip run` for a verb
its repeat rules spared, or with the key of an observed line: `finished polix:reviewer`, `passed just test`,
`injected <text, file or rule id>`, or just `blocked` and `started`), the first 8 characters of the hash and the
first line of the reason. A line of a kind polix no longer writes (`fact`, `injected`, `stop_block`) shows its kind
and the fields it shares with the current lines, the rule of an old `injected` line and the name of an old `fact`
line included. It exits with 0.

## Port

`Port`, declared in `in.go` and implemented by `Adapter` in `out.go`:

- `ReadLog`: loads the log through the field `State` of the adapter, the loader of `LoadState`
  (`internal/app/state/load`) that `cmd/polix` builds once and passes to `Command`, which reads it through
  `internal/platform/jsonl` with the limit `state.MaxLine` and decodes its lines; without the file, an empty log.

## Errors

The use case fails, printing the error and exiting with 1, when:

- `--dir` is not inside a git repository.
- The log exists and cannot be read.

## Flow

1. `Command` turns the flags into a `Request` and injects `Adapter{Repo: repo, State: loadState}`, with the loader
   of the log it was given, as the `Port`.
2. The shell reads the log through the port.
3. `Decide` picks the entries to show: `Output`.
4. The shell builds the response from the output and the skipped lines of the state.
5. `Command` prints the warning of the skipped lines and the entries.
