package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/transcripts"
)

func (app *App) sessionShowCommand() *cobra.Command {
	var out outputFlag
	cmd := &cobra.Command{
		Use:               "show TARGET",
		Short:             "Show what a session's transcript says about it",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: app.completePast(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := app.onePast(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if out.format != "text" {
				return out.encode(app.Out, p)
			}
			state := "not running"
			if p.Running {
				state = "running"
			}
			tw := tabwriter.NewWriter(app.Out, 0, 0, 2, ' ', 0)
			for _, kv := range [][2]string{
				{"name", p.Name},
				{"id", p.SessionID},
				{"state", state},
				{"folder", p.Cwd},
				{"branch", p.Branch},
				{"started", stamp(p.Started)},
				{"active", stamp(p.LastActive) + " (" + ago(app.Now(), p.LastActive) + ")"},
				{"version", p.Version},
				{"last prompt", firstLine(p.LastPrompt, 100)},
				{"transcript", p.Path},
				{"size", sizeDetail(p.Disk)},
			} {
				fmt.Fprintf(tw, "%s:\t%s\n", kv[0], dash(shell.Printable(kv[1])))
			}
			for _, c := range p.Copies {
				fmt.Fprintf(tw, "older copy:\t%s\n", c)
			}
			return tw.Flush()
		},
	}
	out.add(cmd, app.Config.Output.Format)
	return cmd
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

// firstLine is the first line of s, cut to at most limit characters.
func firstLine(s string, limit int) string {
	s, _, cut := strings.Cut(s, "\n")
	if r := []rune(s); len(r) > limit {
		return string(r[:limit-1]) + "…"
	}
	if cut {
		return s + " …"
	}
	return s
}

// sizeDetail is a session's size, with what makes it up.
func sizeDetail(d transcripts.Disk) string {
	return fmt.Sprintf("%s (transcript %s, sidecar %s, copies %s, file history %s)", style.Size(d.Total),
		style.Size(d.Transcript), style.Size(d.Sidecar), style.Size(d.Copies), style.Size(d.FileHistory))
}
