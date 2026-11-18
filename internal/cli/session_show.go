package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/history"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/shell"
)

func (app *App) sessionShowCommand() *cobra.Command {
	var out render.Format
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
			p.ReadTally(true)
			if out != render.Text {
				return out.Encode(app.Out, p)
			}
			tw := tabwriter.NewWriter(app.Out, 0, 0, 2, ' ', 0)
			for _, kv := range history.Details(p, app.Now()) {
				fmt.Fprintf(tw, "%s:\t%s\n", kv[0], render.Dash(shell.Printable(kv[1])))
			}
			for _, c := range p.Copies {
				fmt.Fprintf(tw, "older copy:\t%s\n", shell.Printable(c))
			}
			return tw.Flush()
		},
	}
	app.addOutput(cmd, &out, app.Config.Output.Format)
	return cmd
}
