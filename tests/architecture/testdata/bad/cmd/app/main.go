package main

import stockload "example.com/app/internal/app/stock/load"

// main builds the loader of stock/load twice instead of once (GOARCH-6).
func main() {
	_ = stockload.Command()
	_ = stockload.Command()
}

// helper is a second function of cmd (GOARCH-6).
func helper() {}
