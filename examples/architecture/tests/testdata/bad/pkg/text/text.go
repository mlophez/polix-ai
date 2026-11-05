// Package text imports pkg/util, which imports it: an import cycle in pkg/ (GOARCH-7).
package text

import "example.com/app/pkg/util"

var _ = util.X

func Cut(s string) string { return s }
