# LogWorkflow

Prints the last sessions of the state log, one file per session in `<git-dir>/polix/sessions/`, grouped by session:
the sanctioned way to read it, since the commands that name the state folder and write files are denied. The old
`<git-dir>/polix/log.jsonl` is never read.

## Trigger

```
polix log [--dir <dir>] [--tail <n>] [--json]
```

- `--dir`: any folder of the repository; defaults to the current one.
- `--tail` (`-n`): how many of the last sessions, each shown whole; 5 by default, 0 for all.
- `--json`: print only the entries of those sessions, as JSON lines with the known fields of an entry.

## Request

`Request`, filled by the command from its flags: `Dir`, `Tail` and `JSON`, from `--dir`, `--tail` and `--json`.

## Logic

`Decide(sessions []state.State, tail int) Output` picks the sessions to show. It takes the state of the file of each
session as the shell read it through the port, in no given order, and `tail` from the request.

`Output.Sessions` holds the sessions that have entries, each whole and in file order, ordered by their last
activity, the most recent last: by the latest `at` of their entries, parsed as RFC 3339, so the offsets of the clock
compare right; a session without an `at` that parses comes first, and the ties go by id. Only the last `tail` are
kept, or all of them when `tail` is 0 or less or there are no more. `Output.IDs` holds the id of each, at the same
index: the session of its first entry. `Output.Skipped` sums the lines skipped in the file of every session, those
not shown and those without entries included.

The loader of the log and the domain decide the rest:

- `LoadState` (`internal/app/state/load`): every line that decodes is an entry, kept as it is; a line that does not
  decode is left out.
- `state.State.Skipped`: the lines over `state.MaxLine` (4 MiB), skipped as they are read.

## Response

`Response`: `Sessions`, each session to show as a `Session`, with its `ID` and its `Entries` in file order, each
entry an `Entry` with the fields of a line of the log (`At`, `Kind`, `Agent`, `Session`, `Event`, `Actor`,
`AgentID`, `Branch`, `Hash`, `Before`, `Head`, `Refs`, `Rule`, `Fact`, `Action`, `Line`, `Key`, `Outcome` and
`Reason`; `AgentID`, the id of the subagent that acted, only on the lines of a subagent whose agent gives one;
`Head`, the HEAD commit, only on the observed lines finished with a hash, passed, blocked and snapshot that know it;
`Refs`, what git moved without HEAD (the stash and an operation stopped halfway), only on a snapshot line written
while there was one; `Before`, the content hash the event started from, only on a blocked line of a stop whose runs
changed the content; `Rule` and `Fact` only come from the lines an older polix wrote, never from a new one), and
`Skipped`, the lines of the files of every session skipped for being too long.

The command writes to stderr `polix: <n> lines of the log are longer than <max> bytes and were skipped` once when
`Skipped` is not 0, then the sessions to stdout. With `--json` it writes only the entries, each as a JSON line, in
the same order and without headers. Else it writes, for each session, the line `== session <id> ==` and then each of
its entries as one line with the time, the kind, the event, the actor, what happened (the outcome with the verb of a
decision, such as `skip run` for a verb its repeat rules spared and `notify notify` for a notice shown to the user,
with its text as the reason, or with the key of an observed line: `finished polix:reviewer`, `passed just test`,
`injected <text, file or rule id>`, `notified <text>`, or just `blocked` and `started`), the first 8 characters of
the hash and the first line of the reason. A line of a kind polix no longer writes (`fact`, `injected`,
`stop_block`) shows its kind and the fields it shares with the current lines, the rule of an old `injected` line and
the name of an old `fact` line included. Without sessions it prints nothing. It exits with 0.

## Port

`Port`, declared in `in.go` and implemented by `Adapter` in `out.go`:

- `ReadLog`: lists `<git-dir>/polix/sessions/` (`state.DirName`, `state.SessionsDir`) and loads each file with the
  extension of a session (`state.FileExt`, `.jsonl`) through the field `State` of the adapter, the loader of
  `LoadState` (`internal/app/state/load`) that `cmd/polix` builds once and passes to `Command`, which reads it
  through `internal/platform/jsonl` with the limit `state.MaxLine` and decodes its lines. The other files and the
  folders of the sessions folder are left out, and so is the old `log.jsonl`, outside it; without the folder there
  are no sessions.

## Errors

The use case fails, printing the error and exiting with 1, when:

- `--dir` is not inside a git repository.
- The sessions folder exists and cannot be listed, or the file of a session exists and cannot be read.

## Flow

1. `Command` turns the flags into a `Request` and injects `Adapter{Repo: repo, State: loadState}`, with the loader
   of the log it was given, as the `Port`.
2. The shell reads the file of every session through the port.
3. `Decide` picks the sessions to show and their order: `Output`.
4. The shell builds the response from the output: each session with its id and its entries, and the skipped lines.
5. `Command` prints the warning of the skipped lines and the sessions, with their headers or as JSON lines.
