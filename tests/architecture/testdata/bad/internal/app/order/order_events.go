package order

// init runs when the package loads: a function that is not a method (GOARCH-44).
func init() {}

// onPlace is a package variable whose value is a function literal (GOARCH-44).
var onPlace = func(l Line) Line { return l }
