package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/pkg/cli"
	"github.com/mlophez/polix-ai/pkg/git"
)

// LogWorkflowRequest is what the caller asks of the use case LogWorkflow: the command fills it from its flags.
type LogWorkflowRequest struct {
	Dir  string // any folder of the repository
	Tail int    // how many of the last entries, 0 or less for all
	JSON bool   // print the entries as JSON lines
}

// LogWorkflowResponse is what the use case LogWorkflow gives back: the command prints it.
type LogWorkflowResponse struct {
	Entries []LogWorkflowEntry // the entries to show, in file order
	Skipped int                // lines of the log skipped for being longer than state.MaxLine
}

// LogWorkflowEntry is one entry of the log as the caller sees it, with the keys of a line of the log in JSON.
type LogWorkflowEntry struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Agent   string `json:"agent,omitempty"`
	Session string `json:"session,omitempty"`
	Event   string `json:"event,omitempty"`
	Actor   string `json:"actor,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Hash    string `json:"hash,omitempty"`
	Fact    string `json:"fact,omitempty"`
	Rule    string `json:"rule,omitempty"`
	Action  string `json:"action,omitempty"`
	Line    int    `json:"line,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// LogWorkflowPort is what the shell of LogWorkflow needs from the outside; LogWorkflowAdapter implements it.
type LogWorkflowPort interface {
	// ReadLog returns the state of the log of the repository: its entries and how many lines were skipped.
	ReadLog(ctx context.Context) (state.State, error)
}

// LogWorkflowCommand is the command `polix log`, the entry of the use case LogWorkflow.
func LogWorkflowCommand() *cobra.Command {
	var req LogWorkflowRequest
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
			res, err := logWorkflow(c.Context(), LogWorkflowAdapter{Repo: repo}, req)
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

// logWorkflow is the shell: it reads the log through the port, picks the entries with the logic and builds the
// response.
func logWorkflow(ctx context.Context, port LogWorkflowPort, req LogWorkflowRequest) (LogWorkflowResponse, error) {
	st, err := port.ReadLog(ctx)
	if err != nil {
		return LogWorkflowResponse{}, err
	}
	out := LogWorkflow(st, req.Tail)
	res := LogWorkflowResponse{Skipped: st.Skipped}
	for _, e := range out.Entries {
		res.Entries = append(res.Entries, LogWorkflowEntry(e))
	}
	return res, nil
}

// print writes the warning of the skipped lines to stderr and the entries to stdout, as JSON lines or as one
// line each: time, kind, event, actor, what happened, the start of the hash and the first line of the reason.
func (res LogWorkflowResponse) print(stdout, stderr io.Writer, raw bool) {
	if res.Skipped > 0 {
		fmt.Fprintf(stderr, "polix: %d lines of the log are longer than %d bytes and were skipped\n", res.Skipped,
			state.MaxLine)
	}
	for _, e := range res.Entries {
		if raw {
			line, _ := json.Marshal(e)
			fmt.Fprintln(stdout, string(line))
			continue
		}
		what := e.Fact
		if what == "" {
			what = strings.TrimSpace(e.Outcome + " " + e.Action + e.Rule)
		}
		reason, _, _ := strings.Cut(e.Reason, "\n")
		fmt.Fprintf(stdout, "%s  %-10s %-13s %-9s %-16s %.8s  %s\n", e.At, e.Kind, e.Event, e.Actor, what, e.Hash,
			reason)
	}
}
