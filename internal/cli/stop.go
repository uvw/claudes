package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/exit"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/target"
)

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
			var failures []render.Failure
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
					failures = append(failures, render.Failure{Label: s.Label(), Reason: err.Error(), Of: s})
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
					return exit.ErrSomeFailed
				}
			}
			return result
		},
	}
	app.addForce(cmd, forceUsage)
	return cmd
}
