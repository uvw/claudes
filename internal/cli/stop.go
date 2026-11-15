package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/target"
)

// pingShort is what ping sends without a message or a ping.text setting. It is
// shorter than defaultPing, since the session was not restarted.
const pingShort = "Pick up where you left off."

func (app *App) pingCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ping TARGET [MESSAGE]",
		Short: "Send a background session a message, or a prompt to carry on",
		Long: `Send a background session MESSAGE, or without one a prompt to carry on (ping.text
in the config). A running session is stopped and resumed with it, because the
claude command line has no way to send a message to a running session, so a turn
in progress is cut off. A stopped session is resumed with it.

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
					return fmt.Errorf("%s is an interactive session: only background sessions can be pinged", s.Label())
				case app.isHost(s):
					return refused(fmt.Errorf("%s is the session running this command: it cannot ping itself", s.Label()))
				}
			}
			app.warnTeams(picked)
			hand := context.WithoutCancel(ctx)
			var failures []failure
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
					err = fmt.Errorf("not resumed (%v). Run: %s", err, s.ManualCommand())
				}
				if err != nil {
					failures = append(failures, failure{s.Label(), err.Error()})
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

func (app *App) stopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop TARGET...",
		Short: "Stop sessions; an interactive one is ended with a signal",
		Long: `Stop sessions. A background session is stopped with claude stop and its
conversation is kept: claudes resume brings it back. An interactive session is
ended, with every process it started: they get SIGTERM, then SIGKILL after ten
seconds, and on Windows they are killed at once. claude --resume brings it back.`,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: app.completeSessions(runningSession, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			sessions, err := app.listed(ctx, true)
			if err != nil {
				return err
			}
			picked, err := target.Resolve(args, sessions)
			if err != nil {
				return err
			}
			if err := requireRunning(picked); err != nil {
				return err
			}
			app.warnTeams(picked)
			others, host := app.splitHost(picked)
			hand := context.WithoutCancel(ctx)
			var failures []failure
			var manual []claude.Session
			for i, s := range others {
				if ctx.Err() != nil {
					notReached(app, others[i:])
					break
				}
				// An interactive session is parked, so claudes resume can print
				// its command later.
				stop := app.stopSession
				if s.Interactive() {
					stop = app.parkAndStop
				}
				if err := stop(hand, s); err != nil {
					failures = append(failures, failure{s.Label(), err.Error()})
					app.progressFailed(s)
					continue
				}
				if s.Interactive() {
					manual = append(manual, s)
				}
				app.progress("stopped", s, "")
			}
			printManual(app, "Interactive sessions are parked. Resume each in its own terminal, or run claudes resume:", manual)
			result := app.reportFailures(failures)
			if host != nil && ctx.Err() == nil {
				if err := app.stopHost(ctx, *host, "Stopping", false); err != nil {
					app.warn("claudes: this session (%s): %v", host.Label(), err)
					return errSomeFailed
				}
			}
			return result
		},
	}
	app.addForce(cmd, forceUsage)
	return cmd
}

// handOffCommand runs a claude subcommand that takes a job id, after matching
// the target by name or id.
func (app *App) handOffCommand(name, short string, finished bool) *cobra.Command {
	return &cobra.Command{
		Use:               name + " TARGET",
		Short:             short,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: app.completeSessions(backgroundSession, true),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.handOff(cmd.Context(), name, args[0], finished)
		},
	}
}

// handOff runs claude NAME on the background session token names, such as
// claude logs or claude attach. finished includes finished sessions.
func (app *App) handOff(ctx context.Context, name, token string, finished bool) error {
	sessions, err := app.Agents.Sessions(ctx, finished)
	if err != nil {
		return err
	}
	picked, err := target.Resolve([]string{token}, sessions)
	if err != nil {
		return err
	}
	s := picked[0]
	if s.JobID == "" {
		return fmt.Errorf("%s is an interactive session: claude %s works on background sessions. claudes logs -f follows any session", s.Label(), name)
	}
	return app.Exec("claude", name, s.JobID)
}
