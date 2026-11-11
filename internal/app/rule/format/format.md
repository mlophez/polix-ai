# FormatRule

Formats every rules file of the folder of rules (`rules.path` of polix.yaml, `ai/rules/` by default) in place, as
`polix config fmt` does with polix.yaml, so every rules file reads the same: `rules` first, the keys of each rule in
the order of its schema, and one blank line between two rules.

## Trigger

```
polix rules fmt [--check] [--dir <dir>]
```

- `--check`: only tell whether the files are formatted, without writing them; exits with 1 when one is not.
- `--dir`: any folder of the repository, whose rules are formatted; defaults to the current one.

It takes no argument.

## Request

`Request`, filled by the command from its flags:

- `Dir` and `Check`, from `--dir` and `--check`.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.

## Logic

`Decide(dir string, files map[string][]byte, repeated []string, check bool) Output` decides what to write. It takes
what the shell read through the port: `dir`, the folder of rules (`Workflow.RulesPath`), `files`, the data of each
`*.yaml` of it by name, `retired.yaml` included, and `repeated`, the ids repeated among its rules as the loader of
the rules words them, or the error where the loader stops; and `check`, from the request.

1. Without files (no folder, or no `*.yaml` in it) there is nothing to format: `Empty`.
2. It formats each file, in the order of the names, with `formatData` of its logic (`logic_order.go`). When it
   fails, `<dir>/<name>: <error>` is a problem of `Problems`; when the result differs from the data read,
   `<dir>/<name>` goes to `Changed` and the result to the data to write.
3. Each id repeated is a problem too, after those of the files: `<file> <id>: id repeated, also in <other file>`, an
   id that an earlier rule or a rule of `retired.yaml` already has. When the loader stopped at a file, its error is
   the problem instead, since no repeat can be ruled out; a problem already reported, as the error of a file that is
   not yaml, is not reported twice.
4. With a change, `Fix` is the command that formats the files, `polix rules fmt`.
5. All or nothing: `Write` holds the data to write by name only without any problem and without `check`; else it is
   nil and nothing is written, so a repeated id stops `polix rules fmt` without writing, with or without `--check`.

`formatData` lays a rules file out with `internal/platform/yamlfmt`, in the order of the keys that `order` gives and
with the sequence that `spaced` separates:

- The layout of `polix config fmt`: two spaces of indentation, sequences indented below their key, flow collections
  on one line, quoted and block scalars as in the source, every comment and the blank lines between items kept, a
  comment above a key moving with it, and at most one blank line in a row.
- `rules` goes first at the top, the other keys after it in their order in the source.
- The keys of each rule follow `id`, `description`, `scope`, `deterministic`, `check` and `reason`; an unknown key
  goes after them, in its order in the source. The rules never move.
- Exactly one blank line before each rule but the first, above the comments of the rule when it has them, unless
  both rules are written in flow style on one line each, without comments above or below them; and none
  between `rules:` and the first rule; the blank lines inside a rule stay as they are, and no other sequence (the
  `scope` of a rule in block style) is separated. A rule that ends in a block scalar with `+` (`|+`, `>+`) gets no
  blank line after it, which would be part of the scalar: its own blank lines separate it.
- It refuses what yamlfmt refuses (anchors, aliases, explicit tags, keys that are not scalars, comments inside a
  flow collection, directives and other document markers), and a result that would decode to other data or lose a
  comment.

## Response

`Response`, from the output and the files written: `Dir`, the folder of rules; `Empty`; `Problems`, those of the files
and then the ids repeated or the error where the loader of the rules stops; `Changed`;
`Written`, each file written as `<dir>/<name>` in the order of the names; and `Fix`. The command prints:

- With problems (files that cannot be formatted, ids repeated, the error of the loader), each to stderr, then
  `<file> is not formatted: run <fix>` to stderr for each file of `Changed`, and exits with 1.
- With an empty folder, `ok: no rules to format in <dir>` to stdout, and exits with 0.
- When it wrote files, `ok: formatted <file>` to stdout for each, and exits with 0.
- With `--check` and changes, `<file> is not formatted: run <fix>` to stderr for each file of `Changed`, with `Fix`
  as `<fix>`, and exits with 1.
- Else `ok: the rules are formatted` to stdout, and exits with 0.

## Port

`Port`, declared in `in.go` and implemented by `Adapter` in `out.go`:

- `LoadWorkflow`: loads polix.yaml of the repository through the field `Workflow` of the adapter, the loader of
  `LoadWorkflow` (`internal/app/workflow/load`) that `cmd/polix` builds once and passes to `Command`; without
  polix.yaml, the default workflow. A polix.yaml that cannot be read, cannot be parsed, has an unknown top level key
  or requires a newer polix is an error; the other problems of a workflow that parses are left to
  `polix config check`.
- `ReadRuleFiles`: reads the `*.yaml` of the folder through `internal/platform/file`, by name, `retired.yaml`
  included and subfolders left out; a missing folder has no files and no error.
- `WriteRuleFile`: writes one file of the folder through `internal/platform/file`, keeping its mode and a symlink to
  it.
- `RepeatedIDs`: the ids repeated among the rules of the folder, retired ones included, through the field `Rules` of
  the adapter, the loader of `LoadRule` (`internal/app/rule/load`) that `cmd/polix` builds once and passes to
  `Command`: its `rule.Loaded.Repeated`, each as `<file> <id>: id repeated, also in <other file>`. When the loader
  stops at `Err`, a file that cannot be read, is not yaml or does not decode as rules (a `scope` that is not a list),
  its `Loaded` holds no repeats, and the rules after that file are not read: `RepeatedIDs` returns the text of `Err`
  alone instead, so `polix rules fmt` writes nothing, as `polix rules check` stops there too.

## Errors

The use case fails, printing the error and exiting with 1, without reading nor writing any rules file, when:

- `--dir` is not inside a git repository, or an argument is given.
- polix.yaml cannot be read, is not valid yaml, has an unknown top level key or requires a newer polix.
- `rules.path` is absolute, the root itself, outside the root or a glob (`Rules.ValidPath`): the error is
  `polix.yaml: rules.path "<path>" <problem>`, in the words of `polix config check`.

It fails too when the folder or a file of it cannot be read, before writing anything, or when a file cannot be
written. A file that cannot be formatted, an id repeated, or a file where the loader of the rules stops, is not an
error of the use case but a problem of the response.

## Flow

1. `Command` turns the flags into a `Request`, opens the repository of `--dir` and injects
   `Adapter{Root: root, Rules: loadRules, Workflow: loadWorkflow}`, with the loaders of the rules and of polix.yaml
   it was given, as the `Port`.
2. The shell loads polix.yaml through the port and checks its `rules.path` with `Rules.ValidPath`.
3. The shell reads the rules files of `Workflow.RulesPath`, and then the ids repeated among their rules, through the
   port.
4. `Decide` decides: `Output`.
5. The shell writes each file of the data to write, in the order of the names, through the port.
6. The shell builds the response, and the command prints it and sets the exit code.
