# testdata/bad

A tree that breaks every GOARCH rule with a deterministic check, so each test proves that its check detects the
violation. It is parsed, never compiled: the go.mod only names the module, so the tests know which imports are
of the module.

It follows the layout by depth of the repository: the domains `cart`, `order` and `stock` in `internal/app/`, the
use cases `order/load`, `order/place`, `order/cancel`, `order/ship`, `stock/count`, `stock/drop`, `stock/load` and
`stock/take` below them,
the helpers `pricing` and `tax` of `order/place`, and the libraries `text` and `util` in `internal/platform/`.

`stock/load` is the good case of a load use case: its `Command` returns the loader of its domain, `stock.Load`,
which `stock/stock_load.go` declares with its result `stock.Loaded`, and it keeps every other rule of a use case;
`order/load` is its bad case, a load use case whose domain declares no loader and whose `Command` returns a cobra
command, and `cmd/app/main.go` builds the loader of `stock/load` twice.

The tests of a use case may import a load use case, to build its loader as `main` does (GOARCH-9):
`order/cancel/out_receipt_test.go` imports `stock/load`, the good case, and `order/place`, a use case it may not
import; `stock/stock_load_test.go` is a test of a domain, which imports no use case, not even a load one.

`stock/take` is the good case of the sibling files of a use case: it has a sibling of each kind (`in_load.go`,
`logic_count.go`, `out_load.go`), an unexported interface of formats and a decoding type, and keeps the rules of
the files of a use case. `checkAccepts` fails if one of those checks reports it, as it does for the other good
cases a test names.

Conventions:

- Every violation carries a comment that names the rule it breaks, so a reader can tell why a line is there.
- The `want` strings of `tests/architecture/goarchNN_test.go` quote the messages of the checks; many pin a line
  number, so append to a file rather than insert in it, and run `go test ./tests/architecture` after any change.
- The `testdata/` folders below the helper `pricing` and the library `text` prove that GOARCH-1 admits them at any
  level; `TestGOARCH1` fails if any of its violations names a `testdata/` folder.
- GOARCH-8, 10, 17 to 22 and 30 have no check: nothing here breaks them on purpose, and `goarch08_test.go` skips.
- GOARCH-41, 42 and 43 are lists of depguard, which needs a module that compiles: their faults live in
  `testdata/depguard`. The imports of `order/order.go` and `order/place/logic.go` that break GOARCH-41 stay for the
  faults of other rules that use them, and no test here reads them.
- GOARCH-44 reads the domain files: `order/order_events.go` holds an `init` and a package variable whose value is a
  function, and `stock/stock_units.go` is its good case, a method on the named type `Stock`, with
  `stock/stock_units_test.go` proving that the test files are left out.
- GOARCH-45 reads the signature of `Decide`: `stock/drop/logic.go` takes five parameters.
- GO-4 has its own module, `testdata/limits`, since golangci-lint needs one that compiles: `limits.go` breaks the
  three limits once each and `limits_test.go` breaks them too without being reported.
