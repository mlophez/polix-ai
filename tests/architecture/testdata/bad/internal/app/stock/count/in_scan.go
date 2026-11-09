package count

// format is the one unexported interface of formats the in files may declare (GOARCH-12).
type format interface{ parse(raw string) int }

// source is a second interface of the in files, in a sibling of in.go (GOARCH-12).
type source interface{ read() string }

// Scan is exported by a sibling of in.go, which exports no name (GOARCH-15).
func Scan() {}

// sum decides with an ordering in a sibling of in.go (GOARCH-37).
func sum(n int) int {
	if n > 1 {
		return n
	}
	return 0
}
