package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/exit"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/target"
)

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
		return exit.Refusal(fmt.Errorf("%s is an interactive session: claude %s works on background sessions. claudes logs -f follows any session", s.Label(), name))
	}
	return app.Exec("claude", name, s.JobID)
}

// resumePast resumes p in its own directory: in the background with bg, or in
// this terminal otherwise, which ends claudes.
func (app *App) resumePast(ctx context.Context, p pastSession, prompt string, bg bool) error {
	if !claude.FolderExists(p.Cwd) {
		return fmt.Errorf("%s ran in %s, which is gone. Move it with claudes session mv first", p.Label(), p.Cwd)
	}
	if bg {
		job, found, err := app.findJob(ctx, p.SessionID)
		if err != nil {
			return err
		}
		if dir := app.Jobs.Folder(job, p.SessionID); dir != "" && !sameDir(dir, p.Cwd) {
			return fmt.Errorf("%s has a background job in %s, and claude --bg --resume would wake it there, not in %s. "+
				"Resume it in this terminal without --bg, or remove the job first with claudes gc %s", p.Label(), dir, p.Cwd, job)
		}
		var fresh claude.NewJob
		if !found {
			fresh = claude.NewJob{Name: p.Name, Options: p.JobOptions()}
		}
		if err := app.resumeAs(ctx, p.SessionID, p.Cwd, prompt, fresh); errors.Is(err, errNotYetRunning) {
			app.warn("claudes: %s: %v.", p.Label(), err)
		} else if err != nil {
			return err
		}
		app.say("resumed %s (%s) in the background", shell.Printable(p.Label()), p.ShortID)
		return nil
	}
	if err := app.Chdir(p.Cwd); err != nil {
		return err
	}
	args := []string{"--resume", p.SessionID}
	if prompt != "" {
		args = append(args, prompt)
	}
	app.unlock()
	return app.Exec("claude", args...)
}

// findJob is the id of a session's background job, and whether it has one,
// from claude agents and then a state.json under the session's short id.
// Passing options to a job that exists starts a copy, so either one counts.
func (app *App) findJob(ctx context.Context, sessionID string) (string, bool, error) {
	s, found, err := app.listedJob(ctx, sessionID)
	if err != nil || found {
		return s.JobID, found, err
	}
	job := claude.ShortID(sessionID)
	return job, app.Jobs.Has(job, sessionID), nil
}

// listedJob is the session claude agents lists with sessionID and a job id,
// and whether there is one. It lists through app.listed, so a list claudes
// does not recognize stops the command before it starts or removes a job.
func (app *App) listedJob(ctx context.Context, sessionID string) (claude.Session, bool, error) {
	sessions, err := app.listed(ctx, true)
	if err != nil {
		return claude.Session{}, false, err
	}
	i := slices.IndexFunc(sessions, func(s claude.Session) bool { return s.SessionID == sessionID && s.JobID != "" })
	if i < 0 {
		return claude.Session{}, false, nil
	}
	return sessions[i], true, nil
}
