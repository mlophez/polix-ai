package stock

// unitsOf is a function that is not a method in a test of the domain, which GOARCH-44 leaves out.
func unitsOf(s Stock) int { return s.Units() }

// unitsOfAll is a package variable of a test whose value is a function, which GOARCH-44 leaves out too.
var unitsOfAll = func(ss []Stock) int { return len(ss) }
