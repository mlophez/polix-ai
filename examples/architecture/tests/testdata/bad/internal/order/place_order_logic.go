package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"example.com/app/pkg/git"
)

// PlaceOrderInput is exported by the logic and is not a domain type (GOARCH-23), and it carries a tag of a
// transport (GOARCH-16).
type PlaceOrderInput struct {
	ID string `json:"id"`
	At time.Time
}

type PlaceOrderOutput struct{ OK bool }

const placeOrderLimit = 10

// PlaceOrder reads the clock and uses the port (GOARCH-4); it takes a type that is not domain and returns an
// error besides its output (GOARCH-23).
func PlaceOrder(in PlaceOrderInput) (PlaceOrderOutput, error) {
	_ = time.Now()
	var _ PlaceOrderPort
	return PlaceOrderOutput{OK: in.ID != ""}, nil
}

// PlaceOrderTotal is a second exported function of the logic (GOARCH-23).
func PlaceOrderTotal(items []Item) int { return len(items) }

// placeOrderCache is a table that placeOrderLog writes, so it is state (GOARCH-4).
var placeOrderCache = map[string]int{}

// placeOrderSeen and placeOrderClock are not tables: their values are computed (GOARCH-4).
var placeOrderSeen = len(placeOrderCache)

var placeOrderClock = time.Now()

// ErrPlaceOrderEmpty is a sentinel error: no state.
var ErrPlaceOrderEmpty = errors.New("empty")

// placeOrderLog takes a context, writes to the console, draws a random number, starts a goroutine, panics
// (GOARCH-4), and logs and uses git (GOARCH-4).
func placeOrderLog(ctx context.Context) {
	fmt.Println("placed", rand.Int())
	println("placed")
	slog.Info("placed")
	placeOrderCache["placed"] = 1
	_ = git.Open
	go func() {}()
	panic("never")
}

// placeOrderTwice is a helper of the logic file that calls the logic (GOARCH-32).
func placeOrderTwice() { _, _ = PlaceOrder(PlaceOrderInput{}) }
