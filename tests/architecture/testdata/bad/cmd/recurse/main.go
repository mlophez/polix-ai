package main

import stockadapters "example.com/app/internal/app/stock/adapters"

// main calls itself, so the literal runs more than once (GOARCH-6).
func main() {
	_ = (&stockadapters.Log{}).Load
	main()
}
