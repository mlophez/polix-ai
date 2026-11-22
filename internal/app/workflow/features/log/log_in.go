// Package log is the use case LogWorkflow, the command `polix log`: it prints the last session of the state log, or
// those it selects, one file per session. It never imports the standard log, whose name it takes.
package log

import (
	"context"
	"errors"
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
	Dir     string // any folder of the repository
	Tail    int    // how many of the last sessions, 0 or less for all
	All     bool   // every session, whatever Tail says
	Session string // the one session to show, by its id or a prefix of it; empty for the sessions of Tail or All
	JSON    bool   // print only the lines of the records, as they are in the files
}

// Response is what the use case LogWorkflow gives back: the command prints it.
type Response struct {
	Sessions []Session // the sessions to show, the most recently active last
	Skipped  int       // lines of the files of every session skipped for being longer than state.MaxLine
}

// Session is one session of the log as the caller sees it: its id and its records.
type Session struct {
	ID      string         // the id of the session, the session of its first record
	Records []state.Record // the records of the session, in file order
}

// Port is what the shell of LogWorkflow needs from the outside; Adapter implements it.
type Port interface {
	// ReadLog returns the state of the file of each session of the repository, in no given order: its records and
	// how many lines were skipped; none without sessions.
	ReadLog(ctx context.Context) ([]state.State, error)
}

// Command is the command `polix log`, the entry of the use case LogWorkflow: it injects the adapter with the loader
// of the log loadState as the port.
func Command(loadState state.Load) *cobra.Command {
	var req Request
	c := &cobra.Command{
		Use:   "log",
		Short: "Print the last session of the state log, or the sessions --tail, --all or --session select",
		Long: "Print the last session of the state log, one file per session, each whole as a table with one " +
			"block per event, the most recently active last: the sanctioned way to read it, since commands " +
			"that name the state folder and write files are denied. --tail shows the last n sessions, --all every " +
			"session and --session the one whose id is the value or starts with it.",
		Args: cobra.NoArgs,
		// The checks run before cobra checks the exclusive group of --all and --tail, so --session wins over it.
		PreRunE: func(c *cobra.Command, _ []string) error {
			switch {
			case c.Flags().Changed("session"):
				if req.Session == "" {
					return errors.New("--session needs a session id")
				}
				if c.Flags().Changed("tail") || c.Flags().Changed("all") {
					return errors.New("--session selects one session: drop --tail and --all")
				}
			case req.Tail < 1:
				return errors.New("--tail must be 1 or more; use --all for every session")
			}
			return nil
		},
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
	c.Flags().IntVarP(&req.Tail, "tail", "n", 1, "number of the last sessions, each shown whole, 1 or more")
	c.Flags().BoolVar(&req.All, "all", false, "show every session")
	c.Flags().StringVar(&req.Session, "session", "", "show only the session whose id is this or starts with it")
	c.Flags().BoolVar(&req.JSON, "json", false, "print only the lines of the log, as they are in the files, one JSON line each")
	c.MarkFlagsMutuallyExclusive("all", "tail")
	return c
}

// run is the shell: it reads the sessions through the port, picks those to show with the logic and builds the
// response.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	sessions, err := port.ReadLog(ctx)
	if err != nil {
		return Response{}, err
	}
	out := Decide(sessions, req.Tail, req.All, req.Session)
	if out.NoMatch {
		return Response{}, fmt.Errorf("--session %q matches no session", req.Session)
	}
	if len(out.Matches) != 0 {
		return Response{}, fmt.Errorf("--session %q matches several sessions: %s", req.Session,
			strings.Join(out.Matches, ", "))
	}
	res := Response{Skipped: out.Skipped}
	for i, st := range out.Sessions {
		res.Sessions = append(res.Sessions, Session{ID: out.IDs[i], Records: st.Records})
	}
	return res, nil
}

// print writes the warning of the skipped lines to stderr and the sessions to stdout. With nothing to show, that is no
// record with an action or an observation the table shows, it writes nothing else. With raw it writes only the lines
// of the records, as they are in the file, one JSON line each; else the table of the events (see printTable), with
// the colors the output allows.
func (res Response) print(stdout, stderr io.Writer, raw bool) {
	if res.Skipped > 0 {
		fmt.Fprintf(stderr, "polix: %d lines of the log are longer than %d bytes and were skipped\n", res.Skipped,
			state.MaxLine)
	}
	blocks := res.blocks()
	if len(blocks) == 0 {
		return
	}
	if !raw {
		res.printTable(stdout, cli.NewUI(stdout), blocks)
		return
	}
	for _, s := range res.Sessions {
		for _, r := range s.Records {
			fmt.Fprintln(stdout, r.Raw)
		}
	}
}
