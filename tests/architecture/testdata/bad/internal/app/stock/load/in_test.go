package load

import (
	"context"
	"testing"
)

// fakePort is the fake of the port (GOARCH-34).
type fakePort struct{}

func (fakePort) Read(context.Context, string) (int, error) { return 1, nil }

func TestRun(t *testing.T) {
	if _, err := run(context.Background(), fakePort{}, Request{Shop: "main"}); err != nil {
		t.Fatal(err)
	}
}
