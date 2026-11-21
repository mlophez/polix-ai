package main

import stockadapters "example.com/app/internal/app/stock/adapters"

// main builds the adapters struct stockadapters.Log twice instead of once (GOARCH-6).
func main() {
	_ = &stockadapters.Log{}
	_ = &stockadapters.Log{}
}

// helper is a second function of cmd (GOARCH-6).
func helper() {}
