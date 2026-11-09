// Command order is a file that is not a test and imports testenv (GOARCH-43).
package main

import "github.com/mlophez/polix-ai/internal/platform/testenv"

func main() { println(testenv.Dir) }
