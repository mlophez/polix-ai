// File faults: each declaration breaks GOARCH-46.
package adapters

// Open is an exported function (GOARCH-46).
func Open(path string) *File { return &File{} }

// Mode is an exported constant (GOARCH-46).
const Mode = 1

// Default is an exported variable (GOARCH-46).
var Default = &File{}

// Reader is an exported interface (GOARCH-46).
type Reader interface{ Read() int }

// Hook is an exported function type (GOARCH-46).
type Hook func()

// Run is a method of Hook, an exported type that is not a struct (GOARCH-46).
func (h Hook) Run() { h() }

// Adapter is an exported struct named as a name reserved to the use cases (GOARCH-46, GOARCH-23).
type Adapter struct{}

// Decide is a method named as a name reserved to the use cases (GOARCH-46, GOARCH-23).
func (f *File) Decide() bool { return true }

// Command is a method named as a name reserved to the use cases (GOARCH-46, GOARCH-23).
func (f *File) Command() {}
