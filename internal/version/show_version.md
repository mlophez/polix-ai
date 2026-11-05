# ShowVersion

Prints the version of the running polix, so a person, a script or the update of an older polix learns which
binary runs.

## Trigger

```
polix version
```

## Request

`ShowVersionRequest`, filled by the command:

- `Version`, the version of the running polix (`build.Current`): the one the linker set, else the module version
  of a tagged build, else `dev`.

## Logic

`ShowVersion(running string) ShowVersionOutput` decides what to print: `ShowVersionOutput.Version`, the version of
the running binary as it is.

## Response

`ShowVersionResponse`: `Version`, the version to print. The command writes it in one line to stdout and exits with
0.

## Port

`ShowVersionPort`, declared in `show_version_in.go` and implemented by `ShowVersionAdapter` in
`show_version_out.go`, has no methods: the entry passes the version in the request.

## Errors

None: the command only fails on unexpected arguments.

## Flow

1. `ShowVersionCommand` fills a `ShowVersionRequest` with `build.Current` and injects `ShowVersionAdapter` as the
   `ShowVersionPort`.
2. The shell calls `ShowVersion` and builds the response from its output.
3. `ShowVersionCommand` prints the response.
