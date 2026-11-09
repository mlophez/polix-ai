// Command fakecli stands for the CLIs of Claude Code and Codex in the tests that use testenv.Machine, on every
// system: named claude or codex, it records each call in $FAKE_STATE/calls and answers like
// `claude plugin ... --json`, keeping what is installed in $FAKE_STATE.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	st := os.Getenv("FAKE_STATE")
	args := strings.Join(os.Args[1:], " ")
	f, err := os.OpenFile(filepath.Join(st, "calls"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		fmt.Fprintln(f, name+" "+args)
		f.Close()
	}
	if name != "claude" {
		return
	}
	has := func(s string) bool { _, err := os.Stat(filepath.Join(st, s)); return err == nil }
	set := func(s string) { _ = os.WriteFile(filepath.Join(st, s), nil, 0o644) }
	unset := func(s string) { _ = os.Remove(filepath.Join(st, s)) }
	ok := `{"outcome":"ok"}`
	switch {
	case strings.HasPrefix(args, "plugin marketplace add"):
		set("added")
		fmt.Println(ok)
	case strings.HasPrefix(args, "plugin install"):
		if has("installed") {
			fmt.Println(`{"outcome":"ok","installedVersion":"1","availableVersion":"2"}`)
			return
		}
		set("installed")
		fmt.Println(ok)
	case strings.HasPrefix(args, "plugin update"):
		fmt.Println(ok)
	case strings.HasPrefix(args, "plugin uninstall"):
		if !has("installed") {
			fmt.Println(`{"outcome":"failed","failureCode":"not_installed","message":"not installed"}`)
			os.Exit(1)
		}
		unset("installed")
		fmt.Println(ok)
	case strings.HasPrefix(args, "plugin marketplace remove"):
		if !has("added") {
			fmt.Println(`{"outcome":"failed","failureCode":"not_configured","message":"not found"}`)
			os.Exit(1)
		}
		unset("added")
		fmt.Println(ok)
	case strings.HasPrefix(args, "plugin list"):
		if has("installed") {
			fmt.Println(`[{"id":"polix@polix","scope":"user","enabled":true}]`)
			return
		}
		fmt.Println("[]")
	default:
		fmt.Fprintln(os.Stderr, "unexpected")
		os.Exit(2)
	}
}
