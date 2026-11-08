// Package text holds small helpers on strings, the project of the demo of polix.
package text

import "strings"

// Words counts the words of s, separated by white space.
func Words(s string) int {
	return len(strings.Fields(s))
}
