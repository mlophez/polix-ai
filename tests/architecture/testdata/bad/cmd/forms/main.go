package main

import stockadapters "example.com/app/internal/app/stock/adapters"

// main mentions the adapters struct stockadapters.Log in forms other than &stockadapters.Log{} (GOARCH-6).
func main() {
	_ = &stockadapters.Log{}
	_ = stockadapters.Log{}
	var log stockadapters.Log = stockadapters.Log{}
	_ = []stockadapters.Log{{}, {}}
	_ = make([]stockadapters.Log, 2)
	var arr [2]stockadapters.Log
	_, _ = log, arr
}
