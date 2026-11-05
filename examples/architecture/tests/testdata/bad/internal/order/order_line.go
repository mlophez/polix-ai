package order

// Line has an unexported field (GOARCH-25).
type Line struct {
	SKU   string `json:"sku"` // a tag in the domain (GOARCH-16)
	price int
	Tags  []string
}

// Note has no method Valid (GOARCH-26); Fix changes the Line it holds (GOARCH-28) and Code only returns a field of
// a field (GOARCH-27).
type Note struct {
	Text string
	Line Line
}

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

// Ignore validates a line but discards the error (GOARCH-29).
func Ignore() Line {
	l := Line{SKU: "z"}
	l.Valid()
	return l
}

func (n Note) Fix() Note {
	n.Line.SKU = "x"
	n.Line.price++
	return n
}

func (n Note) Code() string { return n.Line.SKU }

// newLine is a constructor (GOARCH-29).
func newLine(sku string) Line { return Line{SKU: sku} }

// Pointer builds a line through & and assigns the error of Valid to _ (GOARCH-29).
func Pointer() *Line {
	p := &Line{SKU: "p"}
	_ = p.Valid()
	return p
}

// Deferred defers Valid, so its error is lost (GOARCH-29).
func Deferred() Line {
	d := Line{SKU: "d"}
	defer d.Valid()
	return d
}

// Spawned calls Valid in a goroutine, so its error is lost (GOARCH-29, and a goroutine in the domain, GOARCH-4).
func Spawned() Line {
	g := Line{SKU: "g"}
	go g.Valid()
	return g
}

// Unbound assigns a line to _, so it is never validated (GOARCH-29).
func Unbound() {
	_ = Line{SKU: "u"}
}

// Bundle embeds a Line and Mark assigns its field through the promotion (GOARCH-28).
type Bundle struct{ Line }

func (b Bundle) Valid() error { return nil }

func (b Bundle) Mark() Bundle {
	b.SKU = "m"
	return b
}
