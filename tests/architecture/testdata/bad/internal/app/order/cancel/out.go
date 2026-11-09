package cancel

// Adapter embeds base, which promotes Close, but lacks Cancel (GOARCH-5).
type Adapter struct{ base }

// options is an unexported type of the out file, which GOARCH-24 admits next to Adapter.
type options struct{}

// Timeout is exported (GOARCH-24).
const Timeout = 1

// cache is a variable of the out file (GOARCH-24).
var cache = map[string]string{}

// Refund is exported but is not a method of the port (GOARCH-24), and it calls the logic (GOARCH-32).
func (Adapter) Refund() { Decide() }

// connect is a function, not a method of the adapter (GOARCH-24).
func connect() {}

// base gives the adapter Close by promotion; Cancel is still missing (GOARCH-5). An unexported type of the out
// file and its method, which GOARCH-24 admits.
type base struct{}

func (base) Close() {}
