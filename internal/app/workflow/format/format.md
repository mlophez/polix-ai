# FormatWorkflow

Formats polix.yaml in place, as gofmt does with Go, so every polix.yaml reads the same: the settings first, the
workflow (`guardrails` and `on`) at the end, and the keys of each section in the order of the reference.

## Trigger

```
polix config fmt [--check] [--dir <dir>]
```

- `--check`: only tell whether polix.yaml is formatted, without writing it; exits with 1 when it is not.
- `--dir`: any folder of the repository; defaults to the current one.

## Request

`Request`, filled by the command from its flags:

- `Dir`, from `--dir`.
- `Check`, from `--check`.

## Logic

`Decide(data []byte, found, check bool) Output` decides what to write. It takes what the shell read through the
port: `data`, the content of polix.yaml, and `found`, whether it exists; and `check`, from the request.

1. Without polix.yaml there is nothing to format: `Missing`.
2. It formats the data with `formatData` of its logic (`logic_order.go`), which only needs YAML: a workflow with
   problems is formatted as well. When it fails, its error is the `Problem` and nothing is written.
3. `Changed` tells whether the formatted data differs from the data read. With a change and without `check`,
   `Write` holds the formatted data; else it is nil and nothing is written.

`formatData` lays polix.yaml out with `internal/platform/yamlfmt`, in the order of the keys that `order` gives from the
vocabulary of the domain:

- Two spaces of indentation, sequences indented below their key, flow collections on one line (`{ a: b }`,
  `[a, b]`), quoted scalars and block scalars (`|`, `>`) as in the source, one space before a line comment, the
  line comments of consecutive lines lined up, and at most one blank line in a row.
- Every comment and the blank lines between items are kept. A comment above a key moves with it; a comment below the
  last line of an item, indented deeper than the next item, stays below it. The comments right above the first key
  stay at the top of the file when that key moves, unless a blank line sets them apart from the top.
- The top level keys follow `version`, `requires`, `base_branch`, `max_stop_retries`, `vars`, `rules`, `plans`,
  `reports`, `guidance`, `risk`, `requirements`, `guardrails` and `on`: `requirements` goes right before
  `guardrails`, and its programs keep their order. Inside each section the keys follow the order of their schema:
  the shortcuts of `guardrails` and the keys of their items, the events of `on` in lifecycle order (`workflow.Events`),
  the keys of an entry (`when`, `unless`, `action`), the conditions (`workflow.Conditions`, also inside `any` and
  `not`), and the keys of each verb. An unknown key goes after the known ones, in its order in the source.
- The items of a list never move, nor the keys a user names: the variables of `vars`, the globs of `risk.weights`
  and of `rules.run`, whose order decides which one matches first.
- A leading document start marker (`---`) stays, with the comments above it. It refuses anchors, aliases, explicit
  tags, keys that are not scalars, comments inside a flow collection, directives and any other document marker. It
  checks its own result: when the formatted file would decode to other data or lose a comment, it fails instead.

## Response

`Response`, from the output: `Missing`, `Problem`, `Changed`, and `Written`, whether polix.yaml was written.
The command prints:

- Without polix.yaml, `polix.yaml not found` to stderr, and exits with 1.
- With a problem, `polix.yaml: <problem>` to stderr, and exits with 1.
- When it wrote polix.yaml, `ok: formatted polix.yaml` to stdout, and exits with 0.
- With `--check` and a change, `polix.yaml is not formatted: run polix config fmt` to stderr, and exits with 1.
- Else `ok: polix.yaml is formatted` to stdout, and exits with 0.

## Port

`Port`, declared in `in.go` and implemented by `Adapter` in `out.go`:

- `ReadWorkflow`: reads polix.yaml at the root of the repository and tells whether it exists.
- `WriteWorkflow`: writes polix.yaml, keeping its mode and a symlink to it.

## Errors

The use case fails, printing the error and exiting with 1, when:

- `--dir` is not inside a git repository.
- polix.yaml cannot be read or written.

A polix.yaml that cannot be formatted is not an error of the use case but its `Problem`.

## Flow

1. `Command` turns the flags into a `Request` and injects `Adapter{Repo: repo}` as the `Port`.
2. The shell reads polix.yaml through the port.
3. `Decide` decides: `Output`.
4. When the output holds data to write, the shell writes it through the port.
5. The shell builds the response, and the command prints it and sets the exit code.
