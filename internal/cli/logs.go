package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/transcripts"
)

// followEvery is how often logs -f looks for new transcript lines.
const followEvery = 500 * time.Millisecond

// followTail is how many events logs -f starts with when --tail is not given.
const followTail = 10

func (app *App) logsCommand() *cobra.Command {
	var follow bool
	var tail int
	var out outputFlag
	cmd := &cobra.Command{
		Use:   "logs TARGET[/TEAMMATE]",
		Short: "Show a session's recent output, or follow it with -f",
		Long: `Without flags, print a background session's recent terminal output, as claude
logs does.

-f follows the session's transcript instead, and prints each prompt, reply, tool
call, and turn's end as it happens, until Ctrl-C. The session keeps running.
--tail N starts with the last N events (10 by default with -f), or prints them
and exits without -f. Both work for interactive and finished sessions too.
-o json prints the transcript records themselves, one per line.

LEAD/NAME shows a teammate of the agent team LEAD leads, from its own
transcript, with the last 10 events unless --tail says otherwise.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: app.completeSessions(anySession, true),
		RunE: func(cmd *cobra.Command, args []string) error {
			if out.format == "yaml" {
				return fmt.Errorf("logs prints -o text or -o json")
			}
			sessions, err := app.Agents.Sessions(cmd.Context(), true)
			if err != nil {
				return err
			}
			// A name with a slash, such as fix/auth, is a session first; only
			// a target that names no session is read as LEAD/TEAMMATE.
			picked, err := target.Resolve(args, sessions)
			lead, mate, ofTeammate := strings.Cut(args[0], "/")
			if err != nil && ofTeammate {
				picked, err = target.Resolve([]string{lead}, sessions)
			} else {
				ofTeammate = false
			}
			if err != nil {
				return err
			}
			s := picked[0]
			if !follow && tail < 0 && !ofTeammate && out.format == "text" {
				return app.handOff(cmd.Context(), "logs", s.ID(), true)
			}
			path, err := app.Transcripts.Find(s.SessionID, s.Cwd)
			if ofTeammate && err == nil {
				if path, err = app.Transcripts.Teammate(s.SessionID, s.Cwd, mate); err != nil {
					err = noMatch{err}
				}
			}
			if err != nil {
				return err
			}
			if tail < 0 {
				tail = followTail
			}
			return app.printLog(cmd.Context(), path, tail, follow, out.format == "json")
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow the session's transcript until Ctrl-C")
	cmd.Flags().IntVarP(&tail, "tail", "t", -1, "start with the last N events (10 with -f)")
	// -1 means "not given"; help shows no default for it.
	cmd.Flags().Lookup("tail").DefValue = "0"
	out.add(cmd, "")
	cmd.Flags().Lookup("output").Usage = "print as text or json"
	return cmd
}

// printLog prints the last tail events of the transcript at path, then, with
// follow, each new one until Ctrl-C.
func (app *App) printLog(ctx context.Context, path string, tail int, follow, raw bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// A line still being written is held back until it ends.
	done := bytes.LastIndexByte(data, '\n') + 1
	events := transcripts.Events(data[:done])
	if raw {
		events = oneEach(events)
	}
	app.printEvents(events[max(0, len(events)-tail):], raw)
	if !follow {
		return nil
	}
	app.endsOnInterrupt = true
	offset := int64(done)
	head := headOf(data[:done])
	for {
		if app.Sleep(ctx, followEvery) != nil {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		all, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return err
		}
		if int64(len(all)) < offset || !bytes.Equal(headOf(all), head) {
			// Claude Code rewrote the transcript, such as on a compaction.
			app.warn("claudes: the transcript was rewritten; following it from the start")
			offset = 0
		}
		more := all[offset:]
		end := bytes.LastIndexByte(more, '\n') + 1
		events := transcripts.Events(more[:end])
		if raw {
			events = oneEach(events)
		}
		app.printEvents(events, raw)
		offset += int64(end)
		if head == nil {
			head = headOf(all[:offset])
		}
	}
}

// headOf is the first line of a transcript, which a rewrite changes. A
// transcript without a whole line has none yet.
func headOf(data []byte) []byte {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return bytes.Clone(data[:i])
	}
	return nil
}

// oneEach keeps one event per transcript record, for -o json, which prints the
// records themselves.
func oneEach(events []transcripts.Event) []transcripts.Event {
	var out []transcripts.Event
	for _, e := range events {
		if n := len(out); n > 0 && bytes.Equal(out[n-1].Raw, e.Raw) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (app *App) printEvents(events []transcripts.Event, raw bool) {
	for _, e := range events {
		if raw {
			var compact bytes.Buffer
			if json.Compact(&compact, e.Raw) == nil {
				fmt.Fprintln(app.Out, compact.String())
			}
			continue
		}
		who := fmt.Sprintf("%-6s", e.Who)
		switch e.Who {
		case "you":
			who = app.colors.Blue(who)
		case "claude":
			who = app.colors.Green(who)
		case "tool":
			who = app.colors.Yellow(who)
		case "turn":
			who = app.colors.Faint(who)
		case "team":
			who = app.colors.Magenta(who)
		}
		stamp := app.colors.Faint(e.Time.Local().Format(time.TimeOnly))
		lines := strings.Split(shell.PrintableText(e.Text), "\n")
		fmt.Fprintf(app.Out, "%s %s %s\n", stamp, who, lines[0])
		for _, l := range lines[1:] {
			if l == "" {
				fmt.Fprintln(app.Out)
			} else {
				fmt.Fprintf(app.Out, "%16s%s\n", "", l)
			}
		}
	}
}
