package state

import "errors"

// Entry is one line of the log.
type Entry struct {
	At      string // RFC 3339, set by whoever appends it
	Kind    string
	Agent   string
	Session string
	Event   string
	Actor   string
	Branch  string
	Hash    string // content hash; on decisions only when the call already computed it
	Fact    string // fact entries
	Rule    string // injected entries: the rule id
	Action  string // decision entries: the action of the workflow entry, or builtin
	Line    int    // decision and stop_block entries: line of the entry in the workflow
	Outcome string
	Reason  string
}

// Entry kinds.
const (
	KindFact      = "fact"       // something polix observed, e.g. reviewed
	KindDecision  = "decision"   // what polix answered to an event
	KindInjected  = "injected"   // a rule already injected as context in a session
	KindStopBlock = "stop_block" // a stop polix blocked, to bound loops
)

// Valid tells why the entry is not valid to be appended: it has no kind or a negative line. A kind this polix
// does not know is accepted, as a log may hold lines of a newer polix. The lines read from the log are kept
// whatever they hold (see Parse).
func (e Entry) Valid() error {
	var errs []error
	if e.Kind == "" {
		errs = append(errs, errors.New("an entry needs a kind"))
	}
	if e.Line < 0 {
		errs = append(errs, errors.New("the line of an entry must not be negative"))
	}
	return errors.Join(errs...)
}
