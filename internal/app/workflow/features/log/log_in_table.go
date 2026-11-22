package log

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/text"
)

// The table of polix log: a header row and one block per event, in the columns of tableHeader. The code of the
// table is made of methods, since the shell of the entry decides nothing (GOARCH-37).

// cell is one line of ACTIONS or OBSERVED: its head, in the color of tone, and the rest of the line.
type cell struct {
	head string
	tone cli.Tone
	tail string
}

// block is one event of the table: its four lead values and the lines of its actions and of its observations.
type block struct {
	lead      [leadCols]string
	actions   []cell
	observed  []cell
	lineCount int // the rows of the block: the longer of the two lists
}

// view is a record seen as a block of the table.
type view struct{ rec state.Record }

// flat is a text on its way to a cell of the table.
type flat string

// row is the cells of one row of the table, one for each column.
type row [len(tableHeader)]cell

const (
	timeLayout = "2006-01-02 15:04:05" // TIME, in the local time zone
	sessionLen = 8                     // the characters of the session id SESSION shows
	actionLen  = 44                    // the longest line of ACTIONS, its three dots included
	columnGap  = "  "                  // between two columns
	leadCols   = 4                     // TIME, SESSION, EVENT and ACTOR: the columns of the first row of a block
)

// tableHeader is the header row: the name of each column.
var tableHeader = [...]string{"TIME", "SESSION", "EVENT", "ACTOR", "ACTIONS", "OBSERVED"}

// width is the characters of the line without its color codes.
func (c cell) width() int { return utf8.RuneCountInString(c.head) + utf8.RuneCountInString(c.tail) }

// printTable writes the header row and the blocks to w, with the colors the ui allows. Columns are padded to their
// widest value, counted without the color codes, and the trailing spaces of each row are left out.
func (res Response) printTable(w io.Writer, ui *cli.UI, blocks []block) {
	widths := res.widths(blocks)
	var header row
	for i, h := range tableHeader {
		header[i] = cell{tail: h}
	}
	fmt.Fprintln(w, res.render(ui, widths, header))
	for _, b := range blocks {
		for i := range b.lineCount {
			fmt.Fprintln(w, res.render(ui, widths, b.row(i)))
		}
	}
}

// widths is the width of each column: its widest value, the header included.
func (Response) widths(blocks []block) [len(tableHeader)]int {
	var widths [len(tableHeader)]int
	for i, h := range tableHeader {
		widths[i] = len(h)
	}
	for _, b := range blocks {
		for i, v := range b.lead {
			widths[i] = max(widths[i], utf8.RuneCountInString(v))
		}
		for _, c := range b.actions {
			widths[leadCols] = max(widths[leadCols], c.width())
		}
	}
	return widths
}

// row returns the i-th row of the block: the lead columns only on the first one, then its action and its observation.
func (b block) row(i int) row {
	var cols row
	if i == 0 {
		for j, v := range b.lead {
			cols[j] = cell{tail: v}
		}
	}
	if i < len(b.actions) {
		cols[leadCols] = b.actions[i]
	}
	if i < len(b.observed) {
		cols[leadCols+1] = b.observed[i]
	}
	return cols
}

// render joins the columns of a row, each padded to its width except the last one with text, and colored by the ui.
func (Response) render(ui *cli.UI, widths [len(tableHeader)]int, cols row) string {
	last := 0
	for i, c := range cols {
		if c.width() > 0 {
			last = i
		}
	}
	var b strings.Builder
	for i, c := range cols[:last+1] {
		if i > 0 {
			b.WriteString(columnGap)
		}
		b.WriteString(ui.Paint(c.tone, c.head))
		b.WriteString(c.tail)
		if i < last {
			b.WriteString(strings.Repeat(" ", widths[i]-c.width()))
		}
	}
	return b.String()
}

// blocks returns the block of each record of the sessions that has something to show, in order.
func (res Response) blocks() []block {
	var out []block
	for _, s := range res.Sessions {
		for _, r := range s.Records {
			if b := (view{r}).block(); b.lineCount > 0 {
				out = append(out, b)
			}
		}
	}
	return out
}

// block returns the block of the record: what its actions and observations show, empty of lines when none.
func (v view) block() block {
	r := v.rec
	b := block{lead: [leadCols]string{v.when(), flat(r.Session).first(sessionLen), flat(r.Event).str(),
		flat(r.Actor).str()}}
	switch r.Legacy {
	case "", state.LegacyDecision, state.LegacyObserved:
		for _, a := range r.Actions {
			if c := v.action(a); c.width() > 0 {
				b.actions = append(b.actions, c)
			}
		}
		for _, o := range r.Observations {
			if !v.hides(o) {
				b.observed = append(b.observed, v.observation(o))
			}
		}
	default:
		if c := v.old(); c.width() > 0 {
			b.actions = append(b.actions, c)
		}
	}
	b.lineCount = max(len(b.actions), len(b.observed))
	// A legacy line is a block of its own even when it says nothing, unless only a snapshot or a watched is left out.
	if r.Legacy != "" && len(r.Observations) == 0 {
		b.lineCount = max(b.lineCount, 1)
	}
	return b
}

// when is the time of the record in the local time zone, or its at as it is when it does not parse as RFC 3339.
func (v view) when() string {
	at, err := time.Parse(time.RFC3339, v.rec.At)
	if err != nil {
		return flat(v.rec.At).str()
	}
	return at.In(time.Local).Format(timeLayout)
}

// action is the line of ACTIONS of an action: its verb, then its values: the outcome when it adds something, the key
// or else the first line of the reason, and the duration of a run. It is cut to actionLen characters and its verb takes
// the color of its outcome.
func (v view) action(a state.Action) cell {
	var values []string
	if !v.silent(a) {
		values = append(values, flat(a.Outcome).str())
	}
	if key := flat(a.Key).str(); key != "" {
		values = append(values, key)
	} else if reason := flat(v.firstLine(a.Reason)).str(); reason != "" {
		values = append(values, reason)
	}
	if a.DurationMS != nil && a.Action == "run" {
		values = append(values, v.duration(*a.DurationMS))
	}
	return v.cell(flat(a.Action).str(), strings.Join(values, ", "), v.tone(a.Outcome))
}

// silent says whether the outcome of an action adds nothing to its verb: an inject injected, a notify notify, a block
// block and a deny deny.
func (view) silent(a state.Action) bool {
	switch a.Action + " " + a.Outcome {
	case "inject injected", "notify notify", "block block", "deny deny":
		return true
	}
	return false
}

// old is the line of ACTIONS of a line of a kind polix no longer writes: its kind, then its fact, its rule or the key
// or reason of its action.
func (v view) old() cell {
	r := v.rec
	value := flat(r.Fact).str()
	if value == "" {
		value = flat(r.Rule).str()
	}
	for _, a := range r.Actions {
		if value == "" {
			value = flat(a.Key).str()
		}
		if value == "" {
			value = flat(v.firstLine(a.Reason)).str()
		}
	}
	return v.cell(flat(r.Legacy).str(), value, 0)
}

// cell builds a line of ACTIONS from its verb and its values, cut to actionLen characters; the verb takes tone.
func (view) cell(verb, values string, tone cli.Tone) cell {
	line := verb
	switch {
	case verb == "":
		line = values
	case values != "":
		line = verb + ": " + values
	}
	line = text.Shorten(line, actionLen)
	if strings.HasPrefix(line, verb) {
		return cell{head: verb, tone: tone, tail: line[len(verb):]}
	}
	return cell{head: line, tone: tone}
}

// observation is the line of OBSERVED of an observation: its outcome and its key, dimmed.
func (view) observation(o state.Observation) cell {
	line := flat(o.Outcome).str()
	if key := flat(o.Key).str(); key != "" {
		line += ": " + key
	}
	return cell{head: line, tone: cli.ToneMuted}
}

// hides says whether an observation is left out of the table: a snapshot and a watched, and one that repeats an action
// of its record: passed after a run, injected after an inject, blocked after a block or a run that blocked, and
// notified after a notify.
func (v view) hides(o state.Observation) bool {
	switch o.Outcome {
	case state.OutcomeSnapshot, state.OutcomeWatched:
		return true
	case state.OutcomePassed:
		return v.acted(func(a state.Action) bool { return a.Action == "run" })
	case state.OutcomeInjected:
		return v.acted(func(a state.Action) bool { return a.Action == "inject" })
	case state.OutcomeBlocked:
		return v.acted(func(a state.Action) bool { return a.Action == "block" || a.Action == "run" && a.Outcome == "block" })
	case state.OutcomeNotified:
		return v.acted(func(a state.Action) bool { return a.Action == "notify" })
	}
	return false
}

// acted says whether an action of the record is one of those want says yes to.
func (v view) acted(want func(state.Action) bool) bool {
	return slices.ContainsFunc(v.rec.Actions, want)
}

// tone is the color of the verb of an action of outcome: red for block, deny and loop_guard, green for passed,
// injected and notify, muted for skip, none for the rest.
func (view) tone(outcome string) cli.Tone {
	switch outcome {
	case "block", "deny", "loop_guard":
		return cli.ToneRed
	case "passed", "injected", "notify":
		return cli.ToneGreen
	case "skip":
		return cli.ToneMuted
	}
	return 0
}

// duration shows the milliseconds of a run: under one second as 300ms, else in seconds with one decimal, truncated.
func (view) duration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%d.%ds", ms/1000, ms%1000/100)
}

// firstLine is the first line of s.
func (view) firstLine(s string) string {
	line, _, _ := strings.Cut(strings.ReplaceAll(s, "\r", "\n"), "\n")
	return line
}

// str makes the text one line: its line breaks and tabs become spaces, every other control character is removed and
// the runs of spaces become one, so no escape code of the log reaches the terminal.
func (f flat) str() string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, string(f))
	return strings.Join(strings.Fields(clean), " ")
}

// first is the first n characters of the one line of the text.
func (f flat) first(n int) string {
	runes := []rune(f.str())
	return string(runes[:min(n, len(runes))])
}
