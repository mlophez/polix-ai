package order

// CancelOrderAdapter lacks the method Cancel of the port (GOARCH-5).
type CancelOrderAdapter struct{}

// options is a second type of the out file (GOARCH-24).
type options struct{}

// Timeout is exported (GOARCH-24).
const Timeout = 1

// cache is a variable of the out file (GOARCH-24).
var cache = map[string]string{}

// Refund is exported but is not a method of the port (GOARCH-24).
func (CancelOrderAdapter) Refund() {}

// connect is a function, not a method of the adapter (GOARCH-24).
func connect() {}
