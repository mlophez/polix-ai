package main

import stockadapters "example.com/app/internal/app/stock/adapters"

// main builds the adapters struct stockadapters.Log in a loop, so it runs more than once (GOARCH-6).
func main() {
	var loads []func()
	for range 2 {
		loads = append(loads, func() { _ = (&stockadapters.Log{}).Load })
	}
	_ = loads
}
