package cli

import (
	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/mod"
	"github.com/uvw/claudes/internal/render"
)

func (app *App) modStatusCommand() *cobra.Command {
	var out render.Format
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Say whether the mod is installed, and which sessions loaded it",
		Long: `Say whether claudes-mod is installed and turned on, its version, and where it
comes from, then list the running sessions and the mod version each loaded. A
session loads the mod when it starts, or at /reload-plugins.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := app.modStatus(cmd.Context())
			if err != nil {
				return err
			}
			if out != render.Text {
				return out.Encode(app.Out, st)
			}
			return app.printModStatus(st)
		},
	}
	app.addOutput(cmd, &out, app.Config.Output.Format)
	return cmd
}

func (app *App) printModStatus(st modState) error {
	switch {
	case !st.Installed:
		app.say("claudes-mod is not installed. Install it with: claudes mod install")
	case !st.Enabled:
		app.say("claudes-mod %s is installed from %s, but turned off. Turn it on with: claude plugin enable %s",
			st.Version, render.Dash(st.Source), mod.ID)
	default:
		app.say("claudes-mod %s is installed from %s, for scope %s.", st.Version, render.Dash(st.Source), st.Scope)
	}
	const before, after = "Requests go through ", "."
	app.say("%s%s%s", before, app.dirIn(st.Dir, render.Width(before+after)), after)
	if len(st.Sessions) == 0 {
		app.say("No running sessions.")
		return nil
	}
	app.say("")
	table := [][]string{{"ID", "MOD", "DIR", "NAME"}}
	for _, s := range st.Sessions {
		loaded := app.colors.Green(s.Version)
		if s.Version == "" {
			loaded = app.colors.Faint("not loaded")
		}
		table = append(table, []string{s.ID, loaded, render.DirCell(s.Cwd), render.NameCell(s.Name)})
	}
	return app.writeTable(app.Out, table, tableFit{})
}
