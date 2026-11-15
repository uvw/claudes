package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/shell"
)

func (app *App) sessionResumeCommand() *cobra.Command {
	var bg bool
	cmd := &cobra.Command{
		Use:   "resume TARGET [PROMPT]",
		Short: "Resume a past session in its own folder, from any folder",
		Long: `Resume a past session in this terminal, in the folder it ran in, with an
optional prompt. --bg resumes it in the background instead. A session that is
still running is refused: resuming it would start a copy.`,
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
	cmd.Flags().BoolVar(&bg, "bg", false, "resume in the background, as claude --bg does")
	return cmd
}

// resumePast resumes p in its own folder: in the background with bg, or in
// this terminal otherwise, which ends claudes.
func (app *App) resumePast(ctx context.Context, p pastSession, prompt string, bg bool) error {
	if !claude.FolderExists(p.Cwd) {
		return fmt.Errorf("%s ran in %s, which is gone. Move it with claudes session mv first", p.Label(), p.Cwd)
	}
	if bg {
		job := claude.ShortID(p.SessionID)
		if dir := app.Jobs.Folder(job, p.SessionID); dir != "" && !sameFolder(dir, p.Cwd) {
			return fmt.Errorf("%s has a background job in %s, and claude --bg --resume would wake it there, not in %s. "+
				"Resume it in this terminal without --bg, or remove the job first with claudes gc %s", p.Label(), dir, p.Cwd, job)
		}
		name := ""
		if !app.Jobs.Has(job, p.SessionID) {
			name = p.Name
		}
		if err := app.resumeNamed(ctx, p.SessionID, p.Cwd, prompt, name); errors.Is(err, errNotYetRunning) {
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

// sameFolder reports whether a and b name the same folder, which on Windows
// can differ in case.
func sameFolder(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ai, bi)
}
