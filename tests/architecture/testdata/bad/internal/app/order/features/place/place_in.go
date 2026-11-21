package place

// Port is a struct, not an interface (GOARCH-12); there is no Request nor Response (GOARCH-11).
type Port struct{}

// Notifier is a second interface in the in file (GOARCH-12).
type Notifier interface{ Notify() }

func Command() { run() }

// Helper is a second exported function in the in file (GOARCH-15) that calls the logic (GOARCH-32) and uses a
// name of the logic that is not its function nor its output (GOARCH-23).
func Helper() { _, _ = Decide(Input{}) }

// run is the shell, without its signature (GOARCH-32), and uses a name of the logic that is not its function nor
// its output (GOARCH-23).
func run() { _, _ = Decide(Input{}) }
