# testdata/bad

A tree that breaks every GOARCH rule with a deterministic check, so each test proves that its check detects the
violation. It is parsed, never compiled: the go.mod only names the module, so the tests know which imports are
of the module.

It follows the layout by depth of the repository: the domains `cart`, `order` and `stock` in `internal/app/`, the use
cases `order/features/place`, `order/features/cancel`, `order/features/ship`, `stock/features/count`,
`stock/features/drop` and `stock/features/take` below them, the helpers `pricing` and `tax` of `order/features/place`,
the adapters packages `order/adapters` and `stock/adapters`, and the libraries `text` and `util` in
`internal/platform/`. `order/refund` is a use case folder placed directly under its noun instead of `features/`
(GOARCH-1), `order/features/shared.go` a Go file directly in `features/`, `cart/features` a `features/` whose only
content is a folder without Go files, and `order/features/ship/in.go` and `order/features/ship/ship_input.go` two files
of a use case without its verb and a kind. `order/helpers.go` is the good case of a domain file named after its topic.

`stock/adapters` is the good case of an adapters package: one exported struct, `Log`, in `log.go`, which the structs of
`order/adapters` share with `File`, with unexported fields that GOARCH-25 leaves out and a decoding type `fileYAML`
whose exported method `UnmarshalYAML` GOARCH-46 admits (`order/adapters/file.go`). `order/adapters/file_faults.go`
declares each exported name that GOARCH-46 reports, `order/adapters/notes.go` is named after no struct of its package,
`order/adapters/convert` is a folder below an adapters package, and `order/adapters/file_imports.go` imports a use case
and another adapters package, which GOARCH-9 reports; `cart/io.go` and `order/features/ship/ship_out_files.go` import an
adapters package from a domain and from a use case, which it also reports.

`cmd/app/main.go` builds the struct `stockadapters.Log` twice (GOARCH-6). Every `_test.go` of `internal/app/` and every
file of `cmd/` may import an adapters package (GOARCH-9): `order/features/cancel/cancel_out_receipt_test.go` imports
`stock/adapters`, and `order/features/place`, a use case it may not import; `stock/io_test.go` and
`stock/features/count/count_out_files_test.go` and `cmd/app/files.go` import it too.

`stock/features/take` is the good case of the sibling files of a use case: it has a sibling of each kind
(`take_in_load.go`, `take_logic_count.go`, `take_out_load.go`), an unexported interface of formats and a decoding type,
and a document, `take.md`, and keeps the rules of the files of a use case. `checkAccepts` fails if one of those checks
reports it, as it does for the other good cases a test names.

Conventions:

- Every violation carries a comment that names the rule it breaks, so a reader can tell why a line is there.
- The `want` strings of `tests/architecture/goarchNN_test.go` quote the messages of the checks; many pin a line
  number, so append to a file rather than insert in it, and run `go test ./tests/architecture` after any change.
- The `testdata/` folders below the helper `pricing` and the library `text` prove that GOARCH-1 admits them at any
  level; `TestGOARCH1` fails if any of its violations names a `testdata/` folder.
- GOARCH-8, 10, 17 to 22 and 30 have no check: nothing here breaks them on purpose, and `goarch08_test.go` skips.
- GOARCH-41, 42 and 43 are lists of depguard, which needs a module that compiles: their faults live in
  `testdata/depguard`. The imports of `order/order.go` and `order/features/place/place_logic.go` that break GOARCH-41
  stay for the faults of other rules that use them, and no test here reads them.
- GOARCH-44 reads the domain files: `order/events.go` holds an `init` and a package variable whose value is a
  function, and `stock/units.go` is its good case, a method on the named type `Stock`, with
  `stock/units_test.go` proving that the test files are left out.
- GOARCH-45 reads the signature of `Decide`: `stock/features/drop/drop_logic.go` takes five parameters.
- GO-4 has its own module, `testdata/limits`, since golangci-lint needs one that compiles: `limits.go` breaks the
  three limits once each and `limits_test.go` breaks them too without being reported.
