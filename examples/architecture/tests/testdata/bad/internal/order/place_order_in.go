package order

// PlaceOrderPort is a struct, not an interface (GOARCH-12); there is no PlaceOrderRequest nor PlaceOrderResponse
// (GOARCH-11).
type PlaceOrderPort struct{}

// Notifier is a second interface in the in file (GOARCH-12) and does not start with PlaceOrder (GOARCH-31).
type Notifier interface{ Notify() }

func PlaceOrderCommand() { placeOrder() }

// Helper is a second exported function in the in file (GOARCH-15, GOARCH-31) that calls the logic (GOARCH-32)
// and uses a name of the logic that is not its function nor its output (GOARCH-23).
func Helper() { _, _ = PlaceOrder(PlaceOrderInput{}) }

// placeOrder uses a name of the logic that is not its function nor its output (GOARCH-23) and is not the shell
// (GOARCH-32).
func placeOrder() { _, _ = PlaceOrder(PlaceOrderInput{}) }
