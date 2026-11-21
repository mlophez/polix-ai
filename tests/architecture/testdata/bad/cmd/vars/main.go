package main

import stockadapters "example.com/app/internal/app/stock/adapters"

// main builds the adapters struct stockadapters.Log with new and with a variable, which are two builds (GOARCH-6).
func main() {
	_ = new(stockadapters.Log)
	var log stockadapters.Log
	_ = &log
}
