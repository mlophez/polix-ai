package cancel

// Adapter embeds base, which promotes Close, but lacks Cancel (GOARCH-5).
type Adapter struct{ base }

// options is a second type of the out file (GOARCH-24).
type options struct{}

// Timeout is exported (GOARCH-24).
const Timeout = 1

// cache is a variable of the out file (GOARCH-24).
var cache = map[string]string{}

// Refund is exported but is not a method of the port (GOARCH-24), and it calls the logic (GOARCH-32).
func (Adapter) Refund() { Decide() }

// connect is a function, not a method of the adapter (GOARCH-24).
func connect() {}

// base gives the adapter Close by promotion; Cancel is still missing (GOARCH-5). A second type and its method in
// the out file (GOARCH-24).
type base struct{}

func (base) Close() {}
