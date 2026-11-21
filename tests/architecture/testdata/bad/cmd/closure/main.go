package main

import stockadapters "example.com/app/internal/app/stock/adapters"

// main builds the adapters struct stockadapters.Log in a closure that can run more than once (GOARCH-6).
func main() {
	load := func() { _ = (&stockadapters.Log{}).Load }
	load()
	load()
}
