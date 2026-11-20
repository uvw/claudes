package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/target"
)

// pingShort is what ping sends without a message or a ping.text setting. It is
// shorter than defaultPing, since the session was not restarted.
const pingShort = "Pick up where you left off."

func (app *App) pingCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ping TARGET [MESSAGE]",
		Short: "Send a background session a message, or a prompt to carry on",
		Long: `Send a background session MESSAGE, or without one a prompt to carry on
(ping.text in the config). A running session is stopped and resumed with it,
because the claude command line has no way to send a message to a running
session, so a turn in progress is cut off. A stopped session is resumed with it.

restart --ping prompts every session that was mid-turn, for more than one.`,
		Args: cobra.RangeArgs(1, 2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return app.completeSessions(backgroundSession, false)(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			text := app.pingPrompt(pingShort)
			if len(args) == 2 {
				text = args[1]
			}
			if err := app.Agents.CheckPrompt(text); err != nil {
				return err
			}
			sessions, err := app.listed(ctx, true)
			if err != nil {
				return err
			}
			picked, err := target.Resolve(args[:1], sessions)
			if err != nil {
				return err
			}
			for _, s := range picked {
				switch {
				case s.Interactive():
					return refusal(fmt.Errorf("%s is an interactive session: only background sessions can be pinged", s.Label()))
				case app.host().Is(s):
					return refusal(fmt.Errorf("%s is the session running this command: it cannot ping itself", s.Label()))
				}
			}
			app.warnTeams(picked)
			hand := context.WithoutCancel(ctx)
			var failures []render.Failure
			for i, s := range picked {
				if ctx.Err() != nil {
					notReached(app, picked[i:])
					break
				}
				var err error
				if s.Running() {
					err = app.relaunch(hand, s, text)
				} else if err = app.resumeSession(hand, s.SessionID, s.Cwd, text); errors.Is(err, errNotYetRunning) {
					app.warn("claudes: %s: %v.", s.Label(), err)
					err = nil
				} else if err != nil {
					err = fmt.Errorf("not resumed (%w). Run: %s", err, s.ManualCommand())
				}
				if err != nil {
					failures = append(failures, render.Failure{Label: s.Label(), Reason: err.Error(), Of: s})
					app.progressFailed(s)
					continue
				}
				app.progress("pinged", s, "")
			}
			return app.reportFailures(failures)
		},
	}
	app.addForce(cmd, forceUsage)
	return cmd
}
