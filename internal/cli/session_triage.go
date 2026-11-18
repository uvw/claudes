package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/exit"
	"github.com/uvw/claudes/internal/history"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/shell"
)

func (app *App) sessionTriageCommand() *cobra.Command {
	var in string
	since := app.daysFlag(0)
	var out render.Format
	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Sort past sessions into remove, review, and keep",
		Long: `Read every past session and sort it into one of three groups. It changes
nothing.

  remove  what session gc takes without flags, and trivial sessions
  review  one prompt or only slash commands, no file changed, at most ` + strconv.Itoa(history.ReviewToolCalls) + ` tool
          calls, and not named with /rename: whether to keep one takes reading
          its prompts
  keep    everything else, with the reason

-i and -l sort only the sessions run in a directory, or active within a
while, and leave out leftover directories, as session du does.

-o json gives each review item's first and last prompt, so an agent can judge
them. Remove the sessions you pick with claudes session gc ID...`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := app.triage(cmd.Context(), in, since.d)
			if err != nil {
				return err
			}
			if out != render.Text {
				return out.Encode(app.Out, report)
			}
			return app.printTriage(report, in != "" || since.d > 0)
		},
	}
	cmd.Flags().StringVarP(&in, "in", "i", "", "only sessions run in this directory or below it")
	cmd.Flags().VarP(&since, "since", "l", "only sessions active within this long, such as 7d, 12h, or today")
	cmd.Flags().Lookup("since").DefValue = ""
	app.addOutput(cmd, &out, app.Config.Output.Format)
	return cmd
}

// triage sorts the past sessions run in the directory in, and active within
// since, or every one when both are unset.
func (app *App) triage(ctx context.Context, in string, since time.Duration) (history.TriageReport, error) {
	list, err := app.pastSessions(ctx)
	if err != nil {
		return history.TriageReport{}, err
	}
	if !app.runningKnown {
		return history.TriageReport{}, exit.Refusal(errors.New("cannot tell which sessions are running, so session triage cannot sort them"))
	}
	keep, err := app.gcKeep(ctx, list)
	if err != nil {
		return history.TriageReport{}, err
	}
	// A leftover belongs to no session the filter could match.
	narrowed := in != "" || since > 0
	if list, err = app.filterPast(list, nil, in, since); err != nil {
		return history.TriageReport{}, err
	}
	items, err := history.Collect(app.Transcripts, list, keep, history.Kinds{Empty: true, Trivial: true, Copies: true, Leftovers: !narrowed}, app.Now())
	if err != nil {
		return history.TriageReport{}, err
	}
	return history.Triage(list, items, keep, app.Now()), nil
}

// printTriage prints r. narrowed is set when -i or -l left sessions out, so
// the remove command names each session rather than kinds.
func (app *App) printTriage(r history.TriageReport, narrowed bool) error {
	say := func(format string, args ...any) { fmt.Fprintf(app.Out, format+"\n", args...) }
	if len(r.Remove) > 0 {
		say("Remove %d session(s) and dir(s), %s:", len(r.Remove), render.Size(history.TriageSize(r.Remove)))
		table := [][]string{{"ID", "WHY", "SIZE", "DIR", "NAME"}}
		for _, t := range r.Remove {
			table = append(table, []string{render.Dash(t.ID), t.Why, render.Size(t.Size), render.DirCell(t.Cwd), render.NameCell(t.Label())})
		}
		if err := app.writeTable(app.Out, table, tableFit{}); err != nil {
			return err
		}
		say("\nTo remove them: %s\n", history.RemoveCommand(r.Remove, narrowed))
	}
	if len(r.Review) > 0 {
		say("Review %d session(s), %s: one prompt or only commands, and no file changed:", len(r.Review), render.Size(history.TriageSize(r.Review)))
		table := [][]string{{"ID", "TOOLS", "SIZE", "NAME", "FIRST PROMPT"}}
		for _, t := range r.Review {
			table = append(table, []string{t.ID, strconv.Itoa(t.Activity.ToolCalls), render.Size(t.Size), render.NameCell(t.Label()),
				shell.Printable(render.FirstLine(cmp.Or(t.Activity.FirstPrompt, t.LastPrompt), 60))})
		}
		if err := app.writeTable(app.Out, table, tableFit{fitLast: true}); err != nil {
			return err
		}
		say("\nRemove the ones you pick with: claudes session gc ID...\n")
	}
	counts := map[string]int{}
	for _, t := range r.Keep {
		counts[t.Why]++
	}
	reasons := make([]string, 0, len(counts))
	for why := range counts {
		reasons = append(reasons, why)
	}
	slices.SortFunc(reasons, func(a, b string) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b)) })
	say("Keep %d session(s):", len(r.Keep))
	for _, why := range reasons {
		say("  %4d  %s", counts[why], why)
	}
	return nil
}
