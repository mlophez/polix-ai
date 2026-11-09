# LoadVersion

Loads the cache of the checks of the latest release (`update.json`), so every use case that reads it shares one
reading of its format and one result per file, and an empty or broken cache only means asking again.

## Trigger

Not a command: `cmd/polix` (`main`) calls `Command` once, which returns the loader, a `version.Load`, and passes it
to the `Command` of every use case that reads the cache: `UpdateVersion` and `DiagnoseSetup`. Each puts it in a field
of its adapter, `Cache`, which calls it from its port method `LoadCache` with the file it resolved
(`update.json` in `POLIX_CACHE_DIR`, else `<user cache dir>/polix`):

```
cache(ctx, path) version.Loaded
```

## Request

`Request`, filled by the loader from its argument:

- `Path`, the file of the cache, with the separators of the system.

## Logic

`Decide(cache version.Cache, exists bool, err error) Output` validates the cache read with the domain:

1. With `err` (a file that cannot be read or decoded), an empty cache and the error (`Loaded.Fail`).
2. Else the `version.Loaded` of the cache, followed by `Valid`, which asks `Cache.Valid`: a latest release that is
   not a version fails it, with an empty cache and the error.

## Response

`Response`: `Loaded`, the `version.Loaded` the loader returns: `Cache`, `Exists` (whether the file exists) and
`Err`.

## Port

`Port`, implemented by `Adapter` in `out.go`:

- `ReadCache`: reads the file through `internal/platform/file` and decodes it with the unexported type `cacheJSON`
  (`checked_at`, `latest` and `failed_at`); whether it exists. A missing file is an empty cache and no error; a file
  that cannot be read or decoded is the error.

## Errors

None: the loader never fails. A file that cannot be read or decoded, or a cache that is not valid, is an empty cache
with its error in `Err`, which the use cases leave out: they take the cache, and an empty one only means asking
again.

## Flow

1. `Command` builds the loader around `Adapter` as the `Port`, once per process.
2. The first call for a file turns it into a `Request` and runs the shell: it reads the cache through the port,
   calls `Decide` once and builds the response.
3. The loader keeps the result for that file for the life of the process, and returns it to every later call
   without reading again: a use case that saves the cache and loads it again in the same process would see the old
   one, which no use case does (`UpdateVersion` and `DiagnoseSetup` load it once, before they save it).
