// Package util imports text, which imports it back (GOARCH-7).
package util

import "example.com/app/internal/platform/text"

var _ = text.Cut
