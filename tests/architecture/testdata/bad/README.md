# testdata/bad

A tree that breaks every GOARCH rule with a deterministic check, so each test proves that its check detects the
violation. It is parsed, never compiled: the go.mod only names the module, so the tests know which imports are
of the module.

It follows the layout by depth of the repository: the domains `cart`, `order` and `stock` in `internal/app/`, the
use cases `order/place`, `order/cancel`, `order/ship`, `stock/count` and `stock/drop` below them, the helpers
`pricing` and `tax` of `order/place`, and the libraries `text` and `util` in `internal/platform/`.

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
