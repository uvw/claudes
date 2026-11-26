package cli

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/compat"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/target"
)

func (app *App) pinCommand(pin bool) *cobra.Command {
	use, short, verb, offers := "pin", "Pin sessions, as Ctrl+T does in agent view", "pinned", unpinnedSession
	if !pin {
		use, short, verb, offers = "unpin", "Unpin sessions", "unpinned", pinnedSession
	}
	cmd := &cobra.Command{
		Use:               use + " TARGET...",
		Short:             short,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: app.completeSessions(offers, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := app.listed(cmd.Context(), true)
			if err != nil {
				return err
			}
			picked, err := target.Resolve(args, sessions)
			if err != nil {
				return err
			}
			// Each account keeps its own pins.json.
			ids := map[*account][]string{}
			var owners []*account
			for _, s := range picked {
				if s.JobID == "" {
					return refusal(fmt.Errorf("%s is an interactive session: only background sessions can be pinned", s.Label()))
				}
				a := app.accountOf(s)
				if _, ok := ids[a]; !ok {
					owners = append(owners, a)
				}
				ids[a] = append(ids[a], s.JobID)
			}
			for _, a := range owners {
				current, err := a.Jobs.Pins()
				if err != nil {
					return err
				}
				if err := app.refuse(app.inAccount(a, "pins.json"), compat.CheckPins(current)); err != nil {
					return err
				}
			}
			changed := map[*account][]string{}
			for _, a := range owners {
				if changed[a], err = a.Jobs.SetPinned(ids[a], pin); err != nil {
					return err
				}
			}
			for _, s := range picked {
				result := verb
				if slices.Contains(changed[app.accountOf(s)], s.JobID) {
					app.say("%s %s (%s)", verb, s.Label(), s.JobID)
				} else {
					result = "already " + verb
					app.say("already %s: %s (%s)", verb, s.Label(), s.JobID)
				}
				app.record(func(rep *report) {
					e := render.EntryOf(s)
					e.Result = result
					rep.Results = append(rep.Results, e)
				})
			}
			return nil
		},
	}
	app.addForce(cmd, forceUsage)
	return cmd
}
