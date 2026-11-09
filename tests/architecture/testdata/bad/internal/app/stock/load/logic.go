package load

import "example.com/app/internal/app/stock"

type Output struct{ Loaded stock.Loaded }

// Decide takes predeclared types and returns its output (GOARCH-23), a value of the domain it validates (GOARCH-29).
func Decide(count int, err error) Output {
	loaded := stock.Loaded{Count: count, Err: err}
	if verr := loaded.Valid(); verr != nil {
		return Output{}
	}
	return Output{Loaded: loaded}
}
