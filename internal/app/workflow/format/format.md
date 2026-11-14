# FormatWorkflow

Formats polix.yaml, or the workflow file its argument names, in place, as gofmt does with Go, so every polix.yaml reads the same: the settings first, the
workflow (`guardrails` and `on`) at the end, and the keys of each section in the order of the reference.

## Trigger

```
polix config fmt [--check] [--dir <dir>] [file]
```

- `--check`: only tell whether the file is formatted, without writing it; exits with 1 when it is not.
- `--dir`: any folder of the repository, whose polix.yaml is formatted; defaults to the current one.
- `file`: a workflow file to format instead of polix.yaml, relative to the working folder, as
  `examples/polix.yaml`. It needs no repository, and cannot go with `--dir`.

## Request

`Request`, filled by the command from its argument and flags:

- `Dir`, from `--dir`.
- `File`, from the argument; empty for polix.yaml of the repository.
- `Check`, from `--check`.

## Logic

`Decide(data []byte, found, check bool, file string) Output` decides what to write. It takes what the shell read
through the port: `data`, the content of the file, and `found`, whether it exists; and `check` and `file` (`File`),
from the request. `File` of the output is `file`, or polix.yaml when it is empty.

1. Without polix.yaml there is nothing to format: `Missing`.
2. It formats the data with `formatData` of its logic (`logic_order.go`), which only needs YAML: a workflow with
   problems is formatted as well. When it fails, its error is the `Problem` and nothing is written.
3. `Changed` tells whether the formatted data differs from the data read. With a change and without `check`,
   `Write` holds the formatted data; else it is nil and nothing is written. With a change, `Fix` is the command that
   formats the file: `polix config fmt`, followed by the file of the request when there is one (even one named polix.yaml), between single quotes when it
   holds a blank or a character of the shell.

`formatData` lays polix.yaml out with `internal/platform/yamlfmt`, in the order of the keys that `order` gives from the
vocabulary of the domain:

- Two spaces of indentation, sequences indented below their key, flow collections on one line (`{ a: b }`,
  `[a, b]`), quoted scalars and block scalars (`|`, `>`) as in the source, one space before a line comment, the
  line comments of consecutive lines lined up, and at most one blank line in a row.
- Every comment and the blank lines between items are kept. A comment above a key moves with it; a comment below the
  last line of an item, indented deeper than the next item, stays below it. The comments right above the first key
  stay at the top of the file when that key moves, unless a blank line sets them apart from the top.
- The top level keys follow `version`, `path`, `requires`, `base_branch`, `max_stop_retries`, `vars`, `rules`,
  `risk`, `requirements`, `guardrails`, `on` and `githooks`: `path` goes right after `version`,
  `rules` holds only `run`, and `requirements` goes right before
  `guardrails`, and its programs keep their order. Inside each section the keys follow the order of their schema:
  the shortcuts of `guardrails` and the keys of their items, the events of `on` in lifecycle order (`workflow.Events`),
  the keys of an entry (`when`, `unless`, `action`), the conditions (`workflow.Conditions`, also inside `any` and
  `not`), the verbs of an action (`workflow.Actions`), and the keys of each verb (`workflow.RunKeys`,
  `workflow.InjectKeys`, `workflow.BlockKeys` and `workflow.NotifyKeys`, so `always` goes before the `message` of a
  `notify` and after the `command` and the `message` of a `run`). The events of `githooks` follow
  `workflow.GithookEvents` (`pre_commit`, `commit_msg`), and its entries are ordered as those of `on`, but for the
  conditions of their `when` and `unless`, which follow `workflow.GithookConditions` (`branch`, `changed`,
  `message`). An unknown key goes after the known ones, in its order in the source.
- One blank line separates two entries of an event of `on` or of `githooks`, and two items of `forbidden_commands`, `format_on_edit`
  and `run_on_stop`, above the comments of the later one, unless both are written on one line each, without comments
  above or below them (a comment at the end of the line is allowed), as `- pattern: "git clean -f*"`. The lists of texts (`protected_files`, `requirements`) and the lists inside an
  entry stay together, and the blank lines of the source are kept.
- The items of a list never move, nor the keys a user names: the variables of `vars`, the globs of `risk.weights`
  and of `rules.run`, whose order decides which one matches first.
- A leading document start marker (`---`) stays, with the comments above it.
- The anchors and aliases of the values stay where they are written: `&name` goes before an anchored scalar, flow
  collection or block scalar, `*name` is written wherever a value goes, and an anchored block collection has its anchor
  alone at the end of the line of its key or of its dash (`run: &test`, `- &entry`), with its content on the lines
  below.
- It refuses an anchor or alias in front of a key (`- &entry when: ...` anchors the key, not the entry), an anchor on
  the whole document, an anchor name defined twice, a merge key (`<<`) that uses an anchor or alias, explicit tags,
  keys that are not scalars, comments inside a flow collection, directives and any other document marker.
- It refuses a file whose canonical order would put an alias above its anchor, which is not YAML: it prints
  `line <n>: the alias *<name> would come before its anchor &<name>: move the anchor up`, with the line of the alias,
  and moving the anchor up fixes it. It checks its own result: when the formatted file would decode to other data or
  lose a comment, it fails instead.

## Response

`Response`, from the output: `File`, the file as the console names it (`File` of the request, or polix.yaml),
`Fix`, `Missing`, `Problem`, `Changed`, and `Written`, whether the file was written. The command prints, with `<file>` that
name:

- Without the file, `<file> not found` to stderr, and exits with 1.
- With a problem, `<file>: <problem>` to stderr, and exits with 1.
- When it wrote the file, `ok: formatted <file>` to stdout, and exits with 0.
- With `--check` and a change, `<file> is not formatted: run <fix>` to stderr, with `Fix` of the response as
  `<fix>`, and exits with 1.
- Else `ok: <file> is formatted` to stdout, and exits with 0.

## Port

`Port`, declared in `in.go` and implemented by `Adapter` in `out.go`:

- `ReadWorkflow`: reads the file of `Path`, polix.yaml at the root of the repository or the file of the argument,
  and tells whether it exists.
- `WriteWorkflow`: writes it, keeping its mode and a symlink to it.

## Errors

The use case fails, printing the error and exiting with 1, when:

- Without a file, `--dir` is not inside a git repository.
- A file is given together with `--dir`, the file is empty, or more than one file is given.
- The file cannot be read or written.

A polix.yaml that cannot be formatted is not an error of the use case but its `Problem`.

## Flow

1. `Command` turns the argument and the flags into a `Request` and injects `Adapter{Path: path}` as the `Port`: the
   file of the argument, or polix.yaml at the root of the repository of `--dir`.
2. The shell reads polix.yaml through the port.
3. `Decide` decides: `Output`.
4. When the output holds data to write, the shell writes it through the port.
5. The shell builds the response, and the command prints it and sets the exit code.
