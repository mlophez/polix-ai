# testdata/depguard

A module that breaks each import list of depguard in `.golangci.yml` once, so the tests of the rules that depguard
decides (`TestGOARCH41`, `TestGOARCH42`, `TestGOARCH43` and `TestGOARCH48`) prove that it detects them. Unlike `testdata/bad` it
compiles, since golangci-lint loads its types, and its `go.mod` declares the module path of polix, so the prefixes
of the lists match. `TestImportas` runs importas alone on it.

- `internal/app/order/order.go`: a domain that imports `encoding/json`, `os` and the library `git`
  (GOARCH-41-domain).
- `internal/app/order/features/place/place_logic.go`: a logic that imports `context` (GOARCH-41-logic).
- `internal/app/order/features/place/place_logic_disk.go`: a sibling of the logic that imports `os` (GOARCH-41-logic),
  and its test `place_logic_disk_test.go`, which imports `os` too and no list reports.
- `internal/platform/util/util.go`: a library that imports `internal/app/` (GOARCH-42).
- `internal/platform/util/util_git.go`: a library that imports another library, `git` (GOARCH-48), and its test
  `util_git_test.go`, which imports `git` too and no list reports.
- `cmd/order/main.go`: a file that is not a test and imports `internal/platform/testenv` (GOARCH-43).
- `internal/app/order/order_test.go`: a test that imports `os` and `testenv`, which no list reports.

- `internal/app/order/adapters/file.go`: an adapters package, imported by the two files below.
- `cmd/order/aliases.go`: imports `order/features/place` and `order/adapters` without the alias `<noun><verb>` and
  `<noun>adapters`, which importas reports (`TestImportas`); `cmd/order/aliased.go` imports them with it, and importas
  accepts it.

The `want` strings of `tests/architecture/goarch41_test.go` to `goarch43_test.go` pin line numbers: append to a
file rather than insert in it, and run `go test ./tests/architecture` after any change.
