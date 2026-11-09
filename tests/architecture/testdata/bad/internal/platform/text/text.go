// Package text imports internal/platform/util, which imports it: an import cycle (GOARCH-7).
package text

import "example.com/app/internal/platform/util"

var _ = util.X

func Cut(s string) string { return s }
