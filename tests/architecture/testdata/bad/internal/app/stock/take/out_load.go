package take

import (
	"context"
	"encoding/json"
	"strconv"
)

// shelfJSON is an unexported type of a sibling of out.go that decodes the format the adapter reads, with its
// method (GOARCH-24).
type shelfJSON struct {
	Count int `json:"count"`
}

// Load is the method of the port, declared in a sibling of out.go (GOARCH-5, GOARCH-24).
func (a Adapter) Load(ctx context.Context, shop string) (string, error) {
	var s shelfJSON
	if err := json.Unmarshal([]byte(a.Data), &s); err != nil {
		return "", err
	}
	return s.text(shop), nil
}

// text renders what the type decoded.
func (s shelfJSON) text(shop string) string { return shop + " " + strconv.Itoa(s.Count) }
