package place

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"example.com/app/internal/app/order"
	"example.com/app/internal/app/order/place/pricing"
	"example.com/app/internal/platform/git"
)

// Input is exported by the logic and is not a domain type (GOARCH-23), and it carries a tag of a transport
// (GOARCH-16).
type Input struct {
	ID string `json:"id"`
	At time.Time
}

type Output struct{ OK bool }

const limit = 10

// Decide reads the clock and uses the port (GOARCH-4), and the helper it uses is not pure (GOARCH-4); it takes a
// type that is not domain and returns an error besides its output (GOARCH-23).
func Decide(in Input) (Output, error) {
	_ = time.Now()
	var _ Port
	return Output{OK: in.ID != "" && pricing.Rate() < limit}, nil
}

// Total is a second exported function of the logic (GOARCH-23).
func Total(items []order.Item) int { return len(items) }

// cache is a table that record writes, so it is state (GOARCH-4).
var cache = map[string]int{}

// seen and clock are not tables: their values are computed (GOARCH-4).
var seen = len(cache)

var clock = time.Now()

// ErrEmpty is a sentinel error: no state.
var ErrEmpty = errors.New("empty")

// record writes to the console, starts a goroutine and panics (GOARCH-4); its imports of context, math/rand,
// log/slog and git break GOARCH-41, which depguard decides on testdata/depguard.
func record(ctx context.Context) {
	fmt.Println("placed", rand.Int())
	println("placed")
	slog.Info("placed")
	cache["placed"] = 1
	_ = git.Open
	go func() {}()
	panic("never")
}

// twice is a helper of the logic file that calls the logic (GOARCH-32).
func twice() { _, _ = Decide(Input{}) }
