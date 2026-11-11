# LoadRule

Loads the rules of a folder of rules (`rules.path` of polix.yaml, `ai/rules/` by default), so every use case that
reads the rules shares one reading of their format and one result per folder, and decides on its own what an absent
folder, an invalid rule or a file that cannot be read means to it.

## Trigger

Not a command: `cmd/polix` (`main`) calls `Command` once, which returns the loader, a `rule.Load`, and passes it to
the `Command` of every use case that reads the rules: `AnswerHook`, `CheckRule`, `CheckSetup`, `DiagnoseSetup` and
`FormatRule`.
Each puts it in a field of its adapter, `Rules`, which calls it from its port method `LoadRules` with the root of the
repository it opened and the folder it was asked for:

```
rules(ctx, root, dir) rule.Loaded
```

## Request

`Request`, filled by the loader from its arguments:

- `Root`, the root of the repository, with the separators of the system.
- `Dir`, the folder of rules, relative to the root, with slashes.

## Logic

`Decide(rules, retired []rule.Rule, exists bool, err error) Output` validates the rules read, in the order of the
files, with the domain (`Rule.Valid`), and finds the ids repeated (`rule.Rules.Repeated`), with `retired`, the rules
of `retired.yaml`, whose ids are taken first:

1. The ids repeated, on the active rules in the order of the files, before they are sorted: one problem for each
   rule whose id an earlier rule or a retired rule already has, `<file> <id>: id repeated, also in <other file>`,
   naming the last earlier one (`<dir>/retired.yaml` for a retired id). A repeat among the retired rules
   themselves is never reported.
2. `Strict`, where a reading that admits no invalid rule stops: the first invalid rule, as
   `<file>: <why it is not valid>`, else the first id repeated, with the text of its problem, else `err`.
3. With `err` (a file that cannot be read or is not yaml), no rules, problems nor ids repeated, and `Err` is `err`.
4. Without it, every rule, invalid ones included, ordered by id, stably, and the problem of each invalid one in the
   words of `polix config check`, in the order of the files: `<file>: a rule has no id`,
   `<file> <id>: no description`, `<file> <id>: check <check> needs deterministic: true` or
   `<file> <id>: check <check> is an absolute path, not relative to the root`; then the problems of the ids
   repeated, which `Repeated` holds too.
5. The result is a `rule.Loaded`, followed by `Valid`; one that is not valid fails with its error
   (`Loaded.Fail`).

## Response

`Response`: `Loaded`, the `rule.Loaded` the loader returns: `Rules`, `Exists` (whether the folder exists),
`Problems`, `Repeated` (the problems of the ids repeated, the last ones of `Problems`), `Err` and `Strict`.

## Port

`Port`, implemented by `Adapter` in `out.go`:

- `ReadRules`: reads the `*.yaml` of the folder through `internal/platform/file`, `retired.yaml`
  (`rule.RetiredFile`) left out, and decodes them in the order of their names with the unexported types
  `fileYAML` and `ruleYAML`, each rule with its description trimmed and its file (`<dir>/<name>`), not
  validated; whether the folder exists. A missing folder has no rules and no error; a file that cannot be read is
  the error; yaml that cannot be decoded is the error with its file, and the rules of the files before it.
- `ReadRetired`: reads `retired.yaml` of the folder through `internal/platform/file` and decodes its rules with
  `fileYAML`, each with its file (`<dir>/retired.yaml`), not validated, only for their ids. A file that is missing,
  cannot be read or is not yaml has no rules and is no error, so it changes nothing.

## Errors

None: the loader never fails. What stops the reading (`Err`) and where a strict reading stops (`Strict`) are data
of the result, which each use case reports as its own:

- `CheckSetup` and `DiagnoseSetup` take every rule, the problems of the invalid ones and of the ids repeated, and
  `Err`.
- `AnswerHook` takes every rule, invalid ones included, and `Err`.
- `CheckRule` takes `Strict`, and the rules only without it.
- `FormatRule` takes only `Repeated`, the ids repeated.

## Flow

1. `Command` builds the loader around `Adapter` as the `Port`, once per process.
2. The first call for a root and a folder turns them into a `Request` and runs the shell: it reads the rules and
   then the retired rules through the port, calls `Decide` once and builds the response.
3. The loader keeps the result for that root and folder for the life of the process, and returns it to every later
   call without reading again: a use case that wrote the rules and read them again in the same process would see
   the old ones, which no use case does.
