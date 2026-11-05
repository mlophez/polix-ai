package order

// Line has an unexported field (GOARCH-25).
type Line struct {
	SKU   string
	price int
}

// Note has no method Valid (GOARCH-26).
type Note struct{ Text string }

func (l Line) Valid() error { return nil }

// SetSKU is named as a setter (GOARCH-27).
func (l Line) SetSKU(sku string) Line {
	l.SKU = sku
	return l
}

// Rename only copies its parameter into a field: a setter with another name (GOARCH-27).
func (l Line) Rename(sku string) Line {
	l.SKU = sku
	return l
}

// Code only returns a field: a getter (GOARCH-27).
func (l Line) Code() string { return l.SKU }

// Discount changes the line through a pointer instead of returning a copy (GOARCH-27).
func (l *Line) Discount() { l.price-- }

// Relabel builds a line without calling Valid (GOARCH-29) and assigns its field outside the methods of Line
// (GOARCH-28).
func Relabel(l Line) Line {
	other := Line{SKU: "x"}
	other.SKU = l.SKU
	return other
}

// Lines validates the first line, but builds the second one without binding it to a variable (GOARCH-29).
func Lines() []Line {
	first := Line{SKU: "a"}
	if err := first.Valid(); err != nil {
		return nil
	}
	return []Line{first, {SKU: "b"}}
}
