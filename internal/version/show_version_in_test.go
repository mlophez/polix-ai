package version

import (
	"bytes"
	"context"
	"testing"
)

// showVersionFake is the port of ShowVersion, which needs nothing.
type showVersionFake struct{}

func TestShowVersionShell(t *testing.T) {
	res, err := showVersion(context.Background(), showVersionFake{}, ShowVersionRequest{Version: "0.2.0"})
	if err != nil || res.Version != "0.2.0" {
		t.Fatalf("showVersion = %+v, %v", res, err)
	}
	var out bytes.Buffer
	res.print(&out)
	if out.String() != "0.2.0\n" {
		t.Fatalf("print = %q", out.String())
	}
}

// polix version prints the version of the binary, dev for a development build such as the tests.
func TestShowVersionCommand(t *testing.T) {
	c := ShowVersionCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs(nil)
	if err := c.Execute(); err != nil || out.String() != "dev\n" {
		t.Fatalf("polix version: %q %v", out.String(), err)
	}
}
