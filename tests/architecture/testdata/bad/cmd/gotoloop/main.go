package main

import stockadapters "example.com/app/internal/app/stock/adapters"

// main goes back with a goto, so the statement under the label runs more than once (GOARCH-6).
func main() {
	var loads []func()
again:
	_ = len(loads)
	loads = append(loads, (&stockadapters.Log{}).Load)
	goto again
}
