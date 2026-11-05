// Package util imports internal/ from pkg/ (GOARCH-7) and pkg/text, which imports it back (GOARCH-7).
package util

import (
	"example.com/app/internal/order"
	"example.com/app/pkg/text"
)

var _ = order.Item{}

var _ = text.Cut
