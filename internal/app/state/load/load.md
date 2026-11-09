# LoadState

Loads the log of polix (`<git-dir>/polix/log.jsonl`), so every use case that reads it shares one reading of its
format, and a log that cannot be read is data of the result.

## Trigger

Not a command: `cmd/polix` (`main`) calls `Command` once, which returns the loader, a `state.Load`, and passes it to
the `Command` of every use case that reads the log: `AnswerHook` and `LogWorkflow`. Each puts it in a field of its
adapter, `State`, which calls it from its port method (`LoadState` of `AnswerHook`, `ReadLog` of `LogWorkflow`) with
the log of the repository it opened:

```
loadState(ctx, path) state.Loaded
```

## Request

`Request`, filled by the loader from its argument:

- `Path`, the file of the log, with the separators of the system.

## Logic

`Decide(entries []state.Entry, skipped int, err error) Output` builds the state with the domain:

1. With `err` (a log that cannot be read), an empty state and the error (`Loaded.Fail`).
2. Else the `state.State` of the entries, in file order, and the count of the lines skipped, followed by `Valid`,
   and the `state.Loaded` of it, followed by `Valid`: a negative count fails them, with an empty state and the
   error.

## Response

`Response`: `Loaded`, the `state.Loaded` the loader returns: `State` (the entries and `Skipped`) and `Err`.

## Port

`Port`, implemented by `Adapter` in `out.go`:

- `ReadLog`: reads the log through `internal/platform/jsonl`, the lines over `state.MaxLine` skipped and counted,
  and decodes each line with the unexported type `entryJSON` (the names of the log, such as `at`, `kind`,
  `outcome` and `reason`, the shape `AnswerHook` writes): every line that decodes is an entry, kept as it is, even
  without kind; a line that does not decode is left out. A missing log has no lines and no error; a log that cannot
  be read is the error.

## Errors

None: the loader never fails. A log that cannot be read is an empty state with its error in `Err`, which each use
case turns into its own: `AnswerHook` lets the loop guard fail open, and `LogWorkflow` fails with it.

## Flow

1. `Command` builds the loader around `Adapter` as the `Port`, once per process.
2. Every call turns its argument into a `Request` and runs the shell: it reads the log through the port, calls
   `Decide` once and builds the response.
3. Unlike the loaders of polix.yaml, of the rules and of the update cache, it keeps no result: `AnswerHook` appends
   to the log in the process that reads it, so every call reads the file again.
