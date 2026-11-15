package cli

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/compat"
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
			ids := make([]string, 0, len(picked))
			for _, s := range picked {
				if s.JobID == "" {
					return refused(fmt.Errorf("%s is an interactive session: only background sessions can be pinned", s.Label()))
				}
				ids = append(ids, s.JobID)
			}
			current, err := app.Jobs.Pins()
			if err != nil {
				return err
			}
			if err := app.refuse("pins.json", compat.CheckPins(current)); err != nil {
				return err
			}
			changed, err := app.Jobs.SetPinned(ids, pin)
			if err != nil {
				return err
			}
			for _, s := range picked {
				result := verb
				if slices.Contains(changed, s.JobID) {
					app.say("%s %s (%s)", verb, s.Label(), s.JobID)
				} else {
					result = "already " + verb
					app.say("already %s: %s (%s)", verb, s.Label(), s.JobID)
				}
				app.record(func(rep *report) {
					e := entryOf(s)
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
