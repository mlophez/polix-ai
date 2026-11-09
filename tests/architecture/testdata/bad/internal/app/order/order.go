package order

import (
	"os"        // the domain imports os (GOARCH-41, which depguard decides on testdata/depguard)
	. "strings" // a dot import (GOARCH-39)

	"example.com/app/internal/app/stock"
)

// Total uses stock, which imports order back (GOARCH-7), and comes before a type (GOARCH-13).
func Total() int { _ = os.Args; _ = ToUpper; return stock.Level }

// Item is not the entity Order (GOARCH-2) and its Valid has a pointer receiver (GOARCH-26).
type Item struct{ ID string }

// Pack carries a tag on an embedded field (GOARCH-16), its Valid returns bool (GOARCH-26), and Tag changes the
// Line it holds (GOARCH-28).
type Pack struct {
	Item `json:"item"`
	Line Line
}

func (i *Item) Valid() error { return nil }

func (p Pack) Valid() bool { return true }

func (p Pack) Tag() Pack {
	p.Line.Tags[0] = "x"
	p.Line.SKU += "y"
	return p
}
