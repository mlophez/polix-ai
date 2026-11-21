// Package log is the use case LogWorkflow, the command `polix log`: it prints the last sessions of the state log,
// one file per session. It never imports the standard log, whose name it takes.
package log

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Request is what the caller asks of the use case LogWorkflow: the command fills it from its flags.
type Request struct {
	Dir  string // any folder of the repository
	Tail int    // how many of the last sessions, 0 or less for all
	JSON bool   // print only the entries, as JSON lines
}

// Response is what the use case LogWorkflow gives back: the command prints it.
type Response struct {
	Sessions []Session // the sessions to show, the most recently active last
	Skipped  int       // lines of the files of every session skipped for being longer than state.MaxLine
}

// Session is one session of the log as the caller sees it: its id and its entries.
type Session struct {
	ID      string  // the id of the session, the session of its first entry
	Entries []Entry // the entries of the session, in file order
}

// Entry is one entry of the log as the caller sees it, with the keys of a line of the log in JSON.
type Entry struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Agent   string `json:"agent,omitempty"`
	Session string `json:"session,omitempty"`
	Event   string `json:"event,omitempty"`
	Actor   string `json:"actor,omitempty"`
	AgentID string `json:"agent_id,omitempty"` // the id of the subagent that acted, when the agent gives one
	Branch  string `json:"branch,omitempty"`
	Hash    string `json:"hash,omitempty"`
	Before  string `json:"before,omitempty"` // the content hash the event of a blocked line started from, when its runs changed it
	Head    string `json:"head,omitempty"`   // the HEAD commit of an observed line about a content
	Refs    string `json:"refs,omitempty"`   // what git moved without HEAD, on a snapshot line
	Rule    string `json:"rule,omitempty"`   // the rule id of a line of kind injected an older polix wrote
	Fact    string `json:"fact,omitempty"`   // the name of a line of kind fact an older polix wrote
	Action  string `json:"action,omitempty"`
	Line    int    `json:"line,omitempty"`
	Key     string `json:"key,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// Port is what the shell of LogWorkflow needs from the outside; Adapter implements it.
type Port interface {
	// ReadLog returns the state of the file of each session of the repository, in no given order: its entries and
	// how many lines were skipped; none without sessions.
	ReadLog(ctx context.Context) ([]state.State, error)
}

// Command is the command `polix log`, the entry of the use case LogWorkflow: it injects the adapter with the loader
// of the log loadState as the port.
func Command(loadState state.Load) *cobra.Command {
	var req Request
	c := &cobra.Command{
		Use:   "log",
		Short: "Print the last sessions of the state log",
		Long: "Print the last sessions of the state log, one file per session, each whole under a header " +
			"`== session <id> ==`, the most recently active last: the sanctioned way to read it, since commands " +
			"that name the state folder and write files are denied.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := git.Open(c.Context(), req.Dir)
			if err != nil {
				return err
			}
			res, err := run(c.Context(), Adapter{Repo: repo, State: loadState}, req)
			if err != nil {
				return err
			}
			res.print(c.OutOrStdout(), c.ErrOrStderr(), req.JSON)
			return nil
		},
	}
	cli.DirFlag(c, &req.Dir, ".")
	c.Flags().IntVarP(&req.Tail, "tail", "n", 5, "number of sessions, each shown whole, 0 for all")
	c.Flags().BoolVar(&req.JSON, "json", false, "print only the entries, as JSON lines")
	return c
}

// run is the shell: it reads the sessions through the port, picks those to show with the logic and builds the
// response.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	sessions, err := port.ReadLog(ctx)
	if err != nil {
		return Response{}, err
	}
	out := Decide(sessions, req.Tail)
	res := Response{Skipped: out.Skipped}
	for i, st := range out.Sessions {
		s := Session{ID: out.IDs[i]}
		for _, e := range st.Entries {
			s.Entries = append(s.Entries, Entry(e))
		}
		res.Sessions = append(res.Sessions, s)
	}
	return res, nil
}

// print writes the warning of the skipped lines to stderr and the sessions to stdout: with raw only their entries,
// as JSON lines; else each session under the line `== session <id> ==`, and each entry as one line: time, kind,
// event, actor, what happened (the outcome with the action, the rule, the fact or the first line of the key, which
// for an inject is its text), the start of the hash and the first line of the reason. A line of a kind polix no
// longer writes shows its kind, and its rule or its fact.
func (res Response) print(stdout, stderr io.Writer, raw bool) {
	if res.Skipped > 0 {
		fmt.Fprintf(stderr, "polix: %d lines of the log are longer than %d bytes and were skipped\n", res.Skipped,
			state.MaxLine)
	}
	for _, s := range res.Sessions {
		if !raw {
			fmt.Fprintf(stdout, "== session %s ==\n", s.ID)
		}
		for _, e := range s.Entries {
			e.print(stdout, stderr, raw)
		}
	}
}

// print writes the entry to stdout as a JSON line with raw, else as one line of text (see Response.print).
func (e Entry) print(stdout, stderr io.Writer, raw bool) {
	if raw {
		line, err := json.Marshal(e)
		if err != nil {
			fmt.Fprintln(stderr, "polix:", err)
			return
		}
		fmt.Fprintln(stdout, string(line))
		return
	}
	key, _, _ := strings.Cut(e.Key, "\n")
	what := strings.TrimSpace(e.Outcome + " " + e.Action + e.Rule + e.Fact + key)
	reason, _, _ := strings.Cut(e.Reason, "\n")
	fmt.Fprintf(stdout, "%s  %-10s %-13s %-9s %-16s %.8s  %s\n", e.At, e.Kind, e.Event, e.Actor, what, e.Hash,
		reason)
}
