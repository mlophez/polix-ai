package place

// Adapter is not a struct (GOARCH-24).
type Adapter map[string]string

// NewAdapter is a constructor, a function of the out file (GOARCH-24, GOARCH-29).
func NewAdapter() Adapter { return Adapter{} }
