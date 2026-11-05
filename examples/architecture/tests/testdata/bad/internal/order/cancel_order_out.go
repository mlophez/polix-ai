package order

// CancelOrderAdapter embeds cancelOrderBase, which promotes Close, but lacks Cancel (GOARCH-5).
type CancelOrderAdapter struct{ cancelOrderBase }

// options is a second type of the out file (GOARCH-24) and does not start with CancelOrder (GOARCH-31).
type options struct{}

// Timeout is exported (GOARCH-24) and does not start with CancelOrder (GOARCH-31).
const Timeout = 1

// cache is a variable of the out file (GOARCH-24, GOARCH-31).
var cache = map[string]string{}

// Refund is exported but is not a method of the port (GOARCH-24), and it calls the logic (GOARCH-32).
func (CancelOrderAdapter) Refund() { CancelOrder() }

// connect is a function, not a method of the adapter (GOARCH-24, GOARCH-31).
func connect() {}

// cancelOrderBase gives the adapter Close by promotion; Cancel is still missing (GOARCH-5). A second type and its
// method in the out file (GOARCH-24).
type cancelOrderBase struct{}

func (cancelOrderBase) Close() {}
