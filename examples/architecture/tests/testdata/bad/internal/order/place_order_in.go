package order

// PlaceOrderPort is a struct, not an interface (GOARCH-12); there is no PlaceOrderRequest nor PlaceOrderResponse
// (GOARCH-11, GOARCH-16).
type PlaceOrderPort struct{}

// Notifier is a second interface in the entry (GOARCH-12).
type Notifier interface{ Notify() }

func PlaceOrderCommand() { placeOrder() }

// Helper is a second exported function in the entry (GOARCH-15) that calls the logic (GOARCH-32).
func Helper() { _, _ = PlaceOrder(PlaceOrderInput{}) }

// placeOrder uses a name of the logic that is not its function nor its output (GOARCH-23).
func placeOrder() { _, _ = PlaceOrder(PlaceOrderInput{}) }
