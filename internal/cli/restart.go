package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/shell"
)

func (app *App) restartCommand() *cobra.Command {
	var f lifecycleFlags
	cmd := &cobra.Command{
		Use:   "restart [TARGET...]",
		Short: "Restart running sessions: the named ones, or all but pinned ones",
		Long: `Restart running sessions. With targets, restarts exactly those. Without, restarts
every running background session except pinned ones.

Background sessions are restarted with claude respawn. A session that is busy,
waiting on a question, or running a shell task comes back idle, so --ping sends
it a prompt to carry on: it is stopped and resumed with the prompt instead.
Interactive sessions are left alone unless asked.

The session running claudes goes last, after everything else is printed,
because stopping it ends this command.`,
		ValidArgsFunction: app.completeSessions(runningSession, false),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.restart(cmd.Context(), args, &f)
		},
	}
	f.addCommon(cmd, app.completeSkip(runningSession))
	f.addWait(cmd, app.waitTimeout())
	f.addPing(cmd, app.pingPrompt(defaultPing))
	f.addInteractive(cmd, "also stop interactive sessions and print their resume commands",
		"also stop interactive sessions and resume them in the background")
	app.addForce(cmd, forceUsage)
	cmd.Flags().BoolVar(&f.outdated, "outdated", false, "only sessions behind the installed Claude Code")
	return cmd
}

func (app *App) restart(ctx context.Context, targets []string, f *lifecycleFlags) error {
	text := f.text()
	if err := app.Agents.CheckPrompt(text); err != nil {
		return err
	}
	sel, err := app.acquire(ctx, targets, f)
	if err != nil {
		return err
	}
	if len(sel.chosen) == 0 {
		if f.outdated {
			app.say("No running session is on an older Claude Code than the one installed.")
		} else {
			app.say("No running sessions to restart.")
		}
		app.printLeftOut(sel, "restarted")
		return nil
	}
	promptFor := func(s claude.Session) string {
		if app.isHost(s) || (s.Interactive() && !f.interactiveBG) || !app.midTurn(s) {
			return ""
		}
		return text
	}
	rows := make([]planRow, 0, len(sel.chosen))
	for _, s := range sel.chosen {
		note := ""
		switch {
		case app.isHost(s):
			note = "this session: restarted last, not pinged"
		case promptFor(s) != "":
			note = "will be pinged"
		case app.midTurn(s) && !s.Interactive():
			note = dash(s.Activity()) + ": its turn ends, and --ping would carry it on"
		}
		note = joinNotes(note, app.teamNote(s))
		rows = append(rows, planRow{s.ID(), s.Label(), dash(s.Activity()), s.Cwd, note})
	}
	app.printPlan("Restarting", rows)
	if proceed, err := app.gate(f, "Restart these sessions?"); !proceed {
		if err == nil {
			app.printLeftOut(sel, "restarted")
		}
		return err
	}

	others, host := app.splitHost(sel.chosen)
	// The first Ctrl-C ends the loop between sessions; the session in hand
	// finishes under hand, which the Ctrl-C does not cancel.
	hand := context.WithoutCancel(ctx)
	var failures []failure
	var manual, respawned []claude.Session
	for i, s := range others {
		if ctx.Err() != nil {
			notReached(app, others[i:])
			break
		}
		s, running, err := app.current(hand, s)
		switch {
		case err != nil:
			failures = append(failures, failure{s.Label(), err.Error()})
			app.progressFailed(s)
			continue
		case !running:
			app.progress("skipped", s, ", which stopped meanwhile")
			continue
		}
		prompt := promptFor(s)
		how, err := app.restartOne(hand, s, prompt, f.interactiveBG)
		switch {
		case err != nil:
			reason := err.Error()
			if how == restartRespawned {
				reason += ". If it is down, claudes resume " + shell.Arg(s.ID()) + " brings it back"
			}
			failures = append(failures, failure{s.Label(), reason})
			app.progressFailed(s)
		case how == restartByHand:
			manual = append(manual, s)
			app.progress("stopped", s, "")
		case prompt != "":
			app.progress("restarted", s, " and pinged")
		default:
			if how == restartRespawned {
				respawned = append(respawned, s)
			}
			app.progress("restarted", s, "")
		}
	}
	failures = append(failures, app.checkBack(hand, respawned)...)
	printManual(app, "Resume each interactive session in its own terminal, or run claudes resume:", manual)
	app.printLeftOut(sel, "restarted")
	result := app.reportFailures(failures)
	if host != nil && ctx.Err() == nil {
		if err := app.restartHost(ctx, *host); err != nil {
			app.warn("claudes: this session (%s): %v", host.Label(), err)
			return errSomeFailed
		}
	}
	return result
}

// restartHow is how restartOne brought a session back.
type restartHow int

const (
	restartRespawned restartHow = iota
	restartRelaunched
	restartByHand
)

// restartOne restarts one session that is not the host. An interactive one is
// resumed in the background with interactiveBG, and otherwise parked and
// stopped for the user to resume.
func (app *App) restartOne(ctx context.Context, s claude.Session, prompt string, interactiveBG bool) (restartHow, error) {
	switch {
	case s.Interactive() && !interactiveBG:
		return restartByHand, app.parkAndStop(ctx, s)
	case s.Interactive() || prompt != "":
		return restartRelaunched, app.relaunch(ctx, s, prompt)
	case s.JobID == "":
		return restartRespawned, errors.New("claude agents gave no job id for this session")
	default:
		return restartRespawned, app.Agents.Respawn(ctx, s.JobID)
	}
}

// restartHost restarts the session running claudes, which ends this command.
func (app *App) restartHost(ctx context.Context, host claude.Session) error {
	if host.Interactive() {
		return app.stopHost(ctx, host, "Stopping", false)
	}
	if host.JobID == "" {
		return errors.New("claude agents gave no job id for this session")
	}
	app.say("")
	app.say("Restarting this session (%s) last. Output ends here.", host.Label())
	app.lastWords(host, "restarting")
	return app.Agents.Respawn(ctx, host.JobID)
}
