package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/shell"
)

func (app *App) sessionResumeCommand() *cobra.Command {
	var bg bool
	cmd := &cobra.Command{
		Use:   "resume TARGET [PROMPT]",
		Short: "Resume a past session in its own directory, from any directory",
		Long: `Resume a past session in this terminal, in the directory it ran in, with an
optional prompt. --bg resumes it in the background instead. A session that is
still running is refused: resuming it would start a copy. When session mv
removed the session's background job, --bg starts a new job with the old
job's options, such as --model.`,
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: app.completePast(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := ""
			if len(args) == 2 {
				prompt = args[1]
			}
			if err := app.Agents.CheckPrompt(prompt); err != nil {
				return err
			}
			p, err := app.onePast(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if p.Running {
				return refused(fmt.Errorf("%s is still running, and resuming it would start a copy. Use claudes attach, or stop it first", p.Label()))
			}
			if !app.runningKnown {
				return refused(fmt.Errorf("cannot tell whether %s is running, and resuming a running session starts a copy: try again", p.Label()))
			}
			return app.resumePast(cmd.Context(), p, prompt, bg)
		},
	}
	cmd.Flags().BoolVarP(&bg, "bg", "b", false, "resume in the background, as claude --bg does")
	app.addForce(cmd, "with --bg, resume although claude agents lists something claudes does not recognize")
	return cmd
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
		if dir := app.Jobs.Folder(job, p.SessionID); dir != "" && !sameFolder(dir, p.Cwd) {
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

// sameFolder reports whether a and b name the same directory, which on Windows
// can differ in case.
func sameFolder(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ai, bi)
}
