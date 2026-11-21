package main

import stockadapters "example.com/app/internal/app/stock/adapters"

// main builds the adapters struct stockadapters.Log once with &stockadapters.Log{}, the form GOARCH-6 admits.
func main() {
	_ = (&stockadapters.Log{}).Load
}
