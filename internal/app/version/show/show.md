# ShowVersion

Prints the version of the running polix, so a person, a script or the update of an older polix learns which
binary runs.

## Trigger

```
polix version
```

## Request

`Request`, filled by the command:

- `Version`, the version of the running polix (`build.Current`): the one the linker set, else the module version
  of a tagged build, else `dev`.

## Logic

`Decide(running string) Output` decides what to print: `Output.Version`, the version of the running binary as
it is.

## Response

`Response`: `Version`, the version to print. The command writes it in one line to stdout and exits with 0.

## Port

`Port`, declared in `in.go` and implemented by `Adapter` in `out.go`, has no methods: the entry passes the version in
the request.

## Errors

None: the command only fails on unexpected arguments.

## Flow

1. `Command` fills a `Request` with `build.Current` and injects `Adapter` as the `Port`.
2. The shell calls `Decide` and builds the response from its output.
3. `Command` prints the response.
