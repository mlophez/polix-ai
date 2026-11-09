package cancel

// Slip is an exported type of a sibling of out.go, which exports no name but the methods of Adapter (GOARCH-24).
type Slip struct{}

// receipt is an unexported type that decodes a format, which GOARCH-24 admits in the out files, but its method
// calls the adapter (GOARCH-32).
type receipt struct{}

func (receipt) refund(a Adapter) { a.Refund() }
