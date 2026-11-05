package order

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// PlaceOrderInput is exported by the logic and is not a domain type (GOARCH-23), and it carries a tag of a
// transport (GOARCH-16).
type PlaceOrderInput struct {
	ID string `json:"id"`
	At time.Time
}

type PlaceOrderOutput struct{ OK bool }

const placeOrderLimit = 10

// PlaceOrder reads the clock and uses the port (GOARCH-4); it takes a type that is not domain and returns more
// than its output (GOARCH-23).
func PlaceOrder(in PlaceOrderInput) (PlaceOrderOutput, int) {
	_ = time.Now()
	var _ PlaceOrderPort
	return PlaceOrderOutput{OK: in.ID != ""}, placeOrderLimit
}

// PlaceOrderTotal is a second exported function of the logic (GOARCH-23).
func PlaceOrderTotal(items []Item) int { return len(items) }

// placeOrderCache is state of the package (GOARCH-4).
var placeOrderCache = map[string]int{}

// placeOrderLog takes a context, writes to the console, draws a random number, starts a goroutine and panics
// (GOARCH-4).
func placeOrderLog(ctx context.Context) {
	fmt.Println("placed", rand.Int())
	go func() {}()
	panic("never")
}
