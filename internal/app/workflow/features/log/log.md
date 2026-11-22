# LogWorkflow

Prints the last session of the state log, or those `--tail`, `--all` and `--session` select, one file per session in
`<git-dir>/polix/sessions/`, as a table of events: the sanctioned way to read it, since the commands that name the state folder and write files are denied. Each
line of a file is the record of one event of the agent, with what polix did with it and what it observed on it (see
`docs/polix.md`), or a legacy line an earlier polix wrote, which is shown too. The old `<git-dir>/polix/log.jsonl` is never read.

## Trigger

```
polix log [--dir <dir>] [--tail <n>] [--all] [--session <id>] [--json]
```

- `--dir`: any folder of the repository; defaults to the current one.
- `--tail` (`-n`): how many of the last sessions, each shown whole; 1 by default, and a value below 1 is an error.
- `--all`: every session. With `--tail` it is refused by cobra, as flags of one exclusive group.
- `--session`: only the session whose id equals the value, else the only one whose id starts with it.
- `--json`: print only the lines of those sessions, each exactly as it is in the file, one JSON line each.

## Request

`Request`, filled by the command from its flags: `Dir`, `Tail`, `All`, `Session` and `JSON`, from `--dir`, `--tail`,
`--all`, `--session` and `--json`.

## Logic

`Decide(sessions []state.State, tail int, all bool, session string) Output` picks the sessions to show. It takes the
state of the file of each session as the shell read it through the port, in no given order, and `tail`, `all` and
`session` from the request.

`Output.Sessions` holds the sessions that have records, each whole and in file order, ordered by their last
activity, the most recent last: by the latest `at` of their records, parsed as RFC 3339, so the offsets of the clock
compare right; a session without an `at` that parses comes first, and the ties go by id. Only the last `tail` are
kept, or all of them when `all` is set, `tail` is 0 or less or there are no more. `Output.IDs` holds the id of each, at the same
index: the session of its first record. `Output.Skipped` sums the lines skipped in the file of every session, those
not shown and those without records included.

When `session` is set, `Decide` keeps only the session whose id equals it, else the only one whose id starts with it,
and ignores `tail` and `all`. With no match `Output.Sessions` is empty and `Output.NoMatch` is set; with several,
`Output.Sessions` is empty and `Output.Matches` holds their ids sorted. The shell turns both into errors.

The loader of the log and the domain decide the rest:

- `stateadapters.Log` (`internal/app/state/adapters`): every line of a file is a `state.Record`, kept with the line as it was
  read (`Raw`): a line with the key `kind` is a legacy record (a flat line an earlier polix wrote, with one action or
  one observation, the rule or the fact of an old kind), one without it an event line, with its `input`, its `actions`
  and its `observed` list. A line that does not decode, or is neither (no session, or an event line with no action nor
  observation), is left out.
- `state.State.Skipped`: the lines over `state.MaxLine` (4 MiB), skipped as they are read.

## Response

`Response`: `Sessions`, each session to show as a `Session`, with its `ID`, the session of its first record, and its
`Records` in file order, each a `state.Record`: the line of the log decoded, with its `Raw` line, and `Skipped`, the lines
of the files of every session skipped for being too long.

The command writes to stderr `polix: <n> lines of the log are longer than <max> bytes and were skipped` once when
`Skipped` is not 0, then the sessions to stdout. With `--json` it writes only the `Raw` line of each record, exactly as it
is in the file, in the same order and without headers. Else it writes a borderless table (`log_in_table.go`).

The table has a header row, `TIME`, `SESSION`, `EVENT`, `ACTOR`, `ACTIONS` and `OBSERVED`, and one block per record, in
file order, the sessions ordered by their last activity. Columns are separated by two spaces and padded to their widest
value, counted without color codes; there are no borders, no blank lines and no trailing spaces.

- The first row of a block holds `TIME` (the `at` in the local time zone as `2006-01-02 15:04:05`, or as it is when it
  does not parse), `SESSION` (the first 8 characters of the id), `EVENT` and `ACTOR`. The rows that follow leave them
  empty and hold the rest of the actions and observations, one per row.
- An element of `ACTIONS` is `<action>: <values>`, the values joined by a comma and a space: the outcome, left out
  when it adds nothing (an `inject` that `injected`, a `notify` that `notify`, a `block` that `block`, a `deny` that
  `deny`), then the key or else the first line of the reason, then, on a `run`, its duration: `300ms` under a second,
  else seconds with one decimal, truncated (8450 ms is `8.4s`). Each line is cut to 44 characters, the three dots
  included.
- An element of `OBSERVED` is `<outcome>: <key>`, or the outcome alone.
- A legacy decision shows in `ACTIONS` and a legacy observed line in `OBSERVED`, as the same elements; a line of an
  old kind (`fact`, `injected`, `stop_block`) in `ACTIONS` as `<kind>: <fact, rule or key>`. A legacy line is a block of
  its own, even when it says nothing, unless it is a `snapshot` or a `watched`.
- Left out: `snapshot` and `watched`; an observation that repeats an action of its record (`passed` after a `run`,
  `injected` after an `inject`, `blocked` after a `block` or a `run` that blocked, `notified` after a `notify`); and
  every record left with no action and no observation to show. When no record of the chosen sessions has anything to
  show, neither the table nor `--json` prints anything.
- Before a key or a reason goes in a cell, its line breaks and tabs become spaces, runs of spaces become one and every
  other control character is removed, so no escape code of the log reaches the terminal.
- Colors, when `cli.NewUI` finds the output allows them (so never on a pipe, in a file, in CI or with `NO_COLOR`): the
  name of each action takes the color of its outcome (red for `block`, `deny` and `loop_guard`, green for `passed`,
  `injected` and `notify`, dimmed for `skip`) and every `OBSERVED` line is dimmed.

Without sessions it prints nothing. It exits with 0.

## Port

`Port`, declared in `log_in.go` and implemented by `Adapter` in `log_out.go`:

- `ReadLog`: lists `<git-dir>/polix/sessions/` (`state.DirName`, `state.SessionsDir`) and loads each file with the
  extension of a session (`state.FileExt`, `.jsonl`) through the field `State` of the adapter, the loader of
  `stateadapters.Log` (`internal/app/state/adapters`) that `cmd/polix` builds once and passes to `Command`, which reads it
  through `internal/platform/jsonl` with the limit `state.MaxLine` and decodes its lines. The other files and the
  folders of the sessions folder are left out, and so is the old `log.jsonl`, outside it; without the folder there
  are no sessions.

## Errors

The use case fails, printing the error and exiting with 1, when:

- `--dir` is not inside a git repository.
- `--tail` is below 1: `--tail must be 1 or more; use --all for every session`.
- `--session` is empty: `--session needs a session id`.
- `--session` comes with `--tail` or `--all`: `--session selects one session: drop --tail and --all`. The entry checks
  `--session` before cobra checks the exclusive group of `--all` and `--tail`, so this error wins over the one of cobra.
- `--session` matches no session: `--session "<value>" matches no session`.
- `--session` matches several sessions: `--session "<value>" matches several sessions: <ids>`, the full ids sorted
  alphabetically.
- The sessions folder exists and cannot be listed, or the file of a session exists and cannot be read.

## Flow

1. `Command` turns the flags into a `Request` and injects `Adapter{Repo: repo, State: loadState}`, with the loader
   of the log it was given, as the `Port`.
2. The shell reads the file of every session through the port.
3. `Decide` picks the sessions to show, by `--tail`, `--all` or `--session`, and their order: `Output`.
4. The shell builds the response from the output: each session with its id and its records, and the skipped lines.
5. `Command` prints the warning of the skipped lines and the sessions, as the table or, with `--json`, the lines of
   their records as they are in the file.
