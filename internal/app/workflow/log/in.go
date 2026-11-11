// Package log is the use case LogWorkflow, the command `polix log`: it prints the last entries of the state log.
// It never imports the standard log, whose name it takes.
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
	Tail int    // how many of the last entries, 0 or less for all
	JSON bool   // print the entries as JSON lines
}

// Response is what the use case LogWorkflow gives back: the command prints it.
type Response struct {
	Entries []Entry // the entries to show, in file order
	Skipped int     // lines of the log skipped for being longer than state.MaxLine
}

// Entry is one entry of the log as the caller sees it, with the keys of a line of the log in JSON.
type Entry struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Agent   string `json:"agent,omitempty"`
	Session string `json:"session,omitempty"`
	Event   string `json:"event,omitempty"`
	Actor   string `json:"actor,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Hash    string `json:"hash,omitempty"`
	Head    string `json:"head,omitempty"` // the HEAD commit of an observed line about a content
	Refs    string `json:"refs,omitempty"` // what git moved without HEAD, on a snapshot line
	Rule    string `json:"rule,omitempty"` // the rule id of a line of kind injected an older polix wrote
	Fact    string `json:"fact,omitempty"` // the name of a line of kind fact an older polix wrote
	Action  string `json:"action,omitempty"`
	Line    int    `json:"line,omitempty"`
	Key     string `json:"key,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// Port is what the shell of LogWorkflow needs from the outside; Adapter implements it.
type Port interface {
	// ReadLog returns the state of the log of the repository: its entries and how many lines were skipped.
	ReadLog(ctx context.Context) (state.State, error)
}

// Command is the command `polix log`, the entry of the use case LogWorkflow: it injects the adapter with the loader
// of the log loadState as the port.
func Command(loadState state.Load) *cobra.Command {
	var req Request
	c := &cobra.Command{
		Use:   "log",
		Short: "Print the last entries of the state log",
		Long: "Print the last entries of the state log: the sanctioned way to read it, since commands that name the " +
			"state folder and write files are denied.",
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
	c.Flags().IntVarP(&req.Tail, "tail", "n", 20, "number of entries, 0 for all")
	c.Flags().BoolVar(&req.JSON, "json", false, "print the entries as JSON lines")
	return c
}

// run is the shell: it reads the log through the port, picks the entries with the logic and builds the response.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	st, err := port.ReadLog(ctx)
	if err != nil {
		return Response{}, err
	}
	out := Decide(st, req.Tail)
	res := Response{Skipped: st.Skipped}
	for _, e := range out.Entries {
		res.Entries = append(res.Entries, Entry(e))
	}
	return res, nil
}

// print writes the warning of the skipped lines to stderr and the entries to stdout, as JSON lines or as one
// line each: time, kind, event, actor, what happened (the outcome with the action, the rule, the fact or the first
// line of the key, which for an inject is its text), the start of the hash and the first line of the reason. A line
// of a kind polix no longer writes shows its kind, and its rule or its fact.
func (res Response) print(stdout, stderr io.Writer, raw bool) {
	if res.Skipped > 0 {
		fmt.Fprintf(stderr, "polix: %d lines of the log are longer than %d bytes and were skipped\n", res.Skipped,
			state.MaxLine)
	}
	for _, e := range res.Entries {
		if raw {
			line, err := json.Marshal(e)
			if err != nil {
				fmt.Fprintln(stderr, "polix:", err)
				continue
			}
			fmt.Fprintln(stdout, string(line))
			continue
		}
		key, _, _ := strings.Cut(e.Key, "\n")
		what := strings.TrimSpace(e.Outcome + " " + e.Action + e.Rule + e.Fact + key)
		reason, _, _ := strings.Cut(e.Reason, "\n")
		fmt.Fprintf(stdout, "%s  %-10s %-13s %-9s %-16s %.8s  %s\n", e.At, e.Kind, e.Event, e.Actor, what, e.Hash,
			reason)
	}
}
