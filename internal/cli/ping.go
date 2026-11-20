package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/target"
)

// pingShort is what ping sends without a message or a ping.text setting. It is
// shorter than defaultPing, since the session was not restarted.
const pingShort = "Pick up where you left off."

func (app *App) pingCommand() *cobra.Command {
	var message string
	cmd := &cobra.Command{
		Use:   "ping TARGET...",
		Short: "Send background sessions a message, or a prompt to carry on",
		Long: `Send background sessions a message with -m, or without one a prompt to carry
on (ping.text in the config). A running session is stopped and resumed with it,
because the claude command line has no way to send a message to a running
session, so a turn in progress is cut off. A stopped session is resumed with it.

Every argument is a target, so a message goes in -m:

  claudes ping api docs -m "run the tests"

A target that names no session stops the command before anything is pinged. A
message that starts with - is refused, since claude would read it as an option.`,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: app.completeSessions(backgroundSession, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			text := app.pingPrompt(pingShort)
			if message != "" {
				text = message
			}
			if err := app.Agents.CheckPrompt(text); err != nil {
				return err
			}
			sessions, err := app.listed(ctx, true)
			if err != nil {
				return err
			}
			picked, err := target.Resolve(args, sessions)
			if err != nil {
				return messageHint(err)
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
	cmd.Flags().StringVarP(&message, "message", "m", "", "the message to send, rather than ping.text")
	app.addForce(cmd, forceUsage)
	return cmd
}

// messageHint adds a hint when a target with a space named no session, since
// it is most likely a message typed without -m.
func messageHint(err error) error {
	var miss *target.MatchError
	if errors.As(err, &miss) && miss.NoMatch && strings.ContainsFunc(miss.Missed, unicode.IsSpace) {
		return fmt.Errorf("%w. To send a message, use -m", err)
	}
	return err
}
