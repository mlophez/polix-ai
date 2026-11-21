package stock

// Units is a method of the named type Stock, which is not a struct: a domain declares it (GOARCH-44).
func (s Stock) Units() int { return int(s) }
